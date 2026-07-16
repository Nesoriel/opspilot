package tlsinspect

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/netguard"
)

const (
	defaultPort       = 443
	defaultTimeout    = 10 * time.Second
	defaultMaxTimeout = 30 * time.Second
	maxChainLength    = 10
)

type Config struct {
	AllowPrivateNetworks bool
	Timeout              time.Duration
	MaxTimeout           time.Duration
	Resolver             *net.Resolver
	RootCAs              *x509.CertPool
	Now                  func() time.Time
}

type Tool struct {
	dialer     *netguard.Dialer
	timeout    time.Duration
	maxTimeout time.Duration
	rootCAs    *x509.CertPool
	now        func() time.Time
}

type input struct {
	Host       string `json:"host"`
	Port       int    `json:"port,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	TimeoutMS  int    `json:"timeout_ms,omitempty"`
}

type output struct {
	Host                string             `json:"host"`
	Port                int                `json:"port"`
	ServerName          string             `json:"server_name"`
	RemoteAddress       string             `json:"remote_address"`
	TCPConnectLatencyMS int64              `json:"tcp_connect_latency_ms"`
	HandshakeLatencyMS  int64              `json:"handshake_latency_ms"`
	TLSVersion          string             `json:"tls_version"`
	CipherSuite         string             `json:"cipher_suite"`
	ALPN                string             `json:"alpn,omitempty"`
	DidResume           bool               `json:"did_resume"`
	Verified            bool               `json:"verified"`
	VerificationError   *verificationError `json:"verification_error,omitempty"`
	Certificates        []certificate      `json:"certificates"`
	ChainTruncated      bool               `json:"chain_truncated,omitempty"`
}

type verificationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type certificate struct {
	Subject            string   `json:"subject"`
	Issuer             string   `json:"issuer"`
	SerialNumber       string   `json:"serial_number"`
	DNSNames           []string `json:"dns_names,omitempty"`
	IPAddresses        []string `json:"ip_addresses,omitempty"`
	NotBefore          string   `json:"not_before"`
	NotAfter           string   `json:"not_after"`
	DaysRemaining      int64    `json:"days_remaining"`
	SignatureAlgorithm string   `json:"signature_algorithm"`
	PublicKeyAlgorithm string   `json:"public_key_algorithm"`
	IsCA               bool     `json:"is_ca"`
}

type diagnosticError struct {
	code string
	err  error
}

func (e *diagnosticError) Error() string {
	return fmt.Sprintf("%s: %v", e.code, e.err)
}

func (e *diagnosticError) Unwrap() error {
	return e.err
}

func New(config Config) *Tool {
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if config.MaxTimeout <= 0 {
		config.MaxTimeout = defaultMaxTimeout
	}
	if config.Timeout > config.MaxTimeout {
		config.Timeout = config.MaxTimeout
	}
	if config.Resolver == nil {
		config.Resolver = net.DefaultResolver
	}
	if config.Now == nil {
		config.Now = time.Now
	}

	return &Tool{
		dialer: &netguard.Dialer{
			Resolver:     config.Resolver,
			AllowPrivate: config.AllowPrivateNetworks,
			Dialer:       net.Dialer{Timeout: config.Timeout},
		},
		timeout:    config.Timeout,
		maxTimeout: config.MaxTimeout,
		rootCAs:    config.RootCAs,
		now:        config.Now,
	}
}

func (t *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "tls_inspect",
		Description: "Perform a read-only TLS handshake and report certificate validity, hostname verification, protocol, cipher suite, ALPN, and certificate-chain metadata.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"host":{"type":"string","description":"DNS hostname or IP address without a port"},"port":{"type":"integer","minimum":1,"maximum":65535,"default":443},"server_name":{"type":"string","description":"Optional TLS SNI and certificate verification name; defaults to host"},"timeout_ms":{"type":"integer","minimum":100,"maximum":30000,"description":"Optional per-call timeout in milliseconds"}},"required":["host"],"additionalProperties":false}`),
	}
}

func (t *Tool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	request, err := t.decodeInput(arguments)
	if err != nil {
		return nil, err
	}

	timeout := t.timeout
	if request.TimeoutMS > 0 {
		timeout = time.Duration(request.TimeoutMS) * time.Millisecond
	}
	if timeout > t.maxTimeout {
		return nil, fmt.Errorf("timeout_ms exceeds maximum %d", t.maxTimeout.Milliseconds())
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tcpStarted := time.Now()
	rawConnection, err := t.dialer.DialContext(callCtx, "tcp", net.JoinHostPort(request.Host, fmt.Sprintf("%d", request.Port)))
	tcpLatency := time.Since(tcpStarted)
	if err != nil {
		return nil, classifyDialError(err)
	}
	defer rawConnection.Close()

	tlsConnection := tls.Client(rawConnection, &tls.Config{
		ServerName:         request.ServerName,
		RootCAs:            t.rootCAs,
		Time:               t.now,
		InsecureSkipVerify: true, // Verification is performed explicitly below so invalid certificates remain inspectable.
	})
	handshakeStarted := time.Now()
	if err := tlsConnection.HandshakeContext(callCtx); err != nil {
		return nil, classifyHandshakeError(err)
	}
	handshakeLatency := time.Since(handshakeStarted)

	state := tlsConnection.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, &diagnosticError{code: "certificate_missing", err: errors.New("peer provided no certificates")}
	}

	now := t.now()
	verified, verifyError := verifyPeer(state.PeerCertificates, request.ServerName, t.rootCAs, now)
	certificates, truncated := summarizeCertificates(state.PeerCertificates, now)
	result := output{
		Host:                request.Host,
		Port:                request.Port,
		ServerName:          request.ServerName,
		RemoteAddress:       rawConnection.RemoteAddr().String(),
		TCPConnectLatencyMS: tcpLatency.Milliseconds(),
		HandshakeLatencyMS:  handshakeLatency.Milliseconds(),
		TLSVersion:          tlsVersionName(state.Version),
		CipherSuite:         tls.CipherSuiteName(state.CipherSuite),
		ALPN:                state.NegotiatedProtocol,
		DidResume:           state.DidResume,
		Verified:            verified,
		VerificationError:   verifyError,
		Certificates:        certificates,
		ChainTruncated:      truncated,
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return payload, nil
}

func (t *Tool) decodeInput(arguments json.RawMessage) (input, error) {
	var request input
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return input{}, fmt.Errorf("decode arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return input{}, errors.New("decode arguments: multiple JSON values are not allowed")
		}
		return input{}, fmt.Errorf("decode arguments: %w", err)
	}

	request.Host = strings.TrimSpace(request.Host)
	request.ServerName = strings.TrimSpace(request.ServerName)
	if err := validateHost(request.Host, "host"); err != nil {
		return input{}, err
	}
	if request.Port == 0 {
		request.Port = defaultPort
	}
	if request.Port < 1 || request.Port > 65535 {
		return input{}, errors.New("port must be between 1 and 65535")
	}
	if request.ServerName == "" {
		request.ServerName = request.Host
	}
	if err := validateHost(request.ServerName, "server_name"); err != nil {
		return input{}, err
	}
	if request.TimeoutMS < 0 {
		return input{}, errors.New("timeout_ms cannot be negative")
	}
	if request.TimeoutMS > 0 && request.TimeoutMS < 100 {
		return input{}, errors.New("timeout_ms must be at least 100")
	}
	return request, nil
}

func validateHost(value, field string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.ContainsAny(value, "/\\@\x00\r\n\t ") {
		return fmt.Errorf("%s contains invalid characters", field)
	}
	if strings.Contains(value, "%") {
		return fmt.Errorf("%s IPv6 zone identifiers are not allowed", field)
	}
	if strings.Contains(value, ":") && net.ParseIP(value) == nil {
		return fmt.Errorf("%s must not include a port", field)
	}
	return nil
}

func verifyPeer(chain []*x509.Certificate, serverName string, roots *x509.CertPool, now time.Time) (bool, *verificationError) {
	intermediates := x509.NewCertPool()
	for _, cert := range chain[1:] {
		intermediates.AddCert(cert)
	}
	_, err := chain[0].Verify(x509.VerifyOptions{
		DNSName:      serverName,
		Intermediates: intermediates,
		Roots:        roots,
		CurrentTime:  now,
	})
	if err == nil {
		return true, nil
	}
	code, message := classifyVerificationError(err, chain[0], now)
	return false, &verificationError{Code: code, Message: message}
}

func classifyVerificationError(err error, leaf *x509.Certificate, now time.Time) (string, string) {
	var hostnameError x509.HostnameError
	if errors.As(err, &hostnameError) {
		return "hostname_mismatch", "certificate does not match the requested server name"
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return "unknown_authority", "certificate chain is not trusted by the configured roots"
	}
	var invalidError x509.CertificateInvalidError
	if errors.As(err, &invalidError) && invalidError.Reason == x509.Expired {
		if now.Before(leaf.NotBefore) {
			return "certificate_not_yet_valid", "certificate validity period has not started"
		}
		return "certificate_expired", "certificate validity period has ended"
	}
	return "certificate_invalid", "certificate chain verification failed"
}

func summarizeCertificates(chain []*x509.Certificate, now time.Time) ([]certificate, bool) {
	limit := len(chain)
	truncated := false
	if limit > maxChainLength {
		limit = maxChainLength
		truncated = true
	}
	result := make([]certificate, 0, limit)
	for _, cert := range chain[:limit] {
		dnsNames := append([]string(nil), cert.DNSNames...)
		sort.Strings(dnsNames)
		ipAddresses := make([]string, 0, len(cert.IPAddresses))
		for _, address := range cert.IPAddresses {
			ipAddresses = append(ipAddresses, address.String())
		}
		sort.Strings(ipAddresses)
		result = append(result, certificate{
			Subject:            cert.Subject.String(),
			Issuer:             cert.Issuer.String(),
			SerialNumber:       strings.ToUpper(cert.SerialNumber.Text(16)),
			DNSNames:           dnsNames,
			IPAddresses:        ipAddresses,
			NotBefore:          cert.NotBefore.UTC().Format(time.RFC3339),
			NotAfter:           cert.NotAfter.UTC().Format(time.RFC3339),
			DaysRemaining:      int64(cert.NotAfter.Sub(now).Hours() / 24),
			SignatureAlgorithm: cert.SignatureAlgorithm.String(),
			PublicKeyAlgorithm: cert.PublicKeyAlgorithm.String(),
			IsCA:               cert.IsCA,
		})
	}
	return result, truncated
}

func classifyDialError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return &diagnosticError{code: "tls_canceled", err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &diagnosticError{code: "tls_timeout", err: err}
	case strings.Contains(err.Error(), "resolve target"):
		return &diagnosticError{code: "dns_failure", err: err}
	case strings.Contains(err.Error(), "blocked addresses"):
		return &diagnosticError{code: "blocked_target", err: err}
	default:
		return &diagnosticError{code: "connect_failed", err: err}
	}
}

func classifyHandshakeError(err error) error {
	if errors.Is(err, context.Canceled) {
		return &diagnosticError{code: "tls_canceled", err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &diagnosticError{code: "tls_timeout", err: err}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &diagnosticError{code: "tls_timeout", err: err}
	}
	var recordError tls.RecordHeaderError
	if errors.As(err, &recordError) {
		return &diagnosticError{code: "tls_protocol_error", err: err}
	}
	return &diagnosticError{code: "tls_handshake_failed", err: err}
}

func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("unknown (0x%04x)", version)
	}
}
