package tlsinspect

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)

func TestInspectValidCertificate(t *testing.T) {
	certificate, roots := makeTestCertificate(t, certificateOptions{
		NotBefore:   fixedNow.Add(-time.Hour),
		NotAfter:    fixedNow.Add(30 * 24 * time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	server, host, port := startTLSServer(t, certificate)
	defer server.Close()

	tool := New(Config{
		AllowPrivateNetworks: true,
		Timeout:              time.Second,
		RootCAs:              roots,
		Now:                  func() time.Time { return fixedNow },
	})
	result, err := tool.Execute(context.Background(), mustJSON(t, input{Host: host, Port: port}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var decoded output
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !decoded.Verified || decoded.VerificationError != nil {
		t.Fatalf("expected verified certificate: %#v", decoded.VerificationError)
	}
	if decoded.Host != host || decoded.Port != port || decoded.ServerName != host {
		t.Fatalf("unexpected target metadata: %#v", decoded)
	}
	if decoded.TLSVersion == "" || decoded.CipherSuite == "" || decoded.RemoteAddress == "" {
		t.Fatalf("missing handshake metadata: %#v", decoded)
	}
	if len(decoded.Certificates) != 2 {
		t.Fatalf("unexpected certificate chain: %#v", decoded.Certificates)
	}
	if decoded.Certificates[0].DaysRemaining != 30 {
		t.Fatalf("days remaining = %d, want 30", decoded.Certificates[0].DaysRemaining)
	}
}

func TestInspectClassifiesHostnameMismatch(t *testing.T) {
	certificate, roots := makeTestCertificate(t, certificateOptions{
		NotBefore:   fixedNow.Add(-time.Hour),
		NotAfter:    fixedNow.Add(24 * time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	server, host, port := startTLSServer(t, certificate)
	defer server.Close()

	tool := New(Config{
		AllowPrivateNetworks: true,
		Timeout:              time.Second,
		RootCAs:              roots,
		Now:                  func() time.Time { return fixedNow },
	})
	result, err := tool.Execute(context.Background(), mustJSON(t, input{
		Host:       host,
		Port:       port,
		ServerName: "wrong.example",
	}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var decoded output
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if decoded.Verified || decoded.VerificationError == nil || decoded.VerificationError.Code != "hostname_mismatch" {
		t.Fatalf("unexpected verification result: %#v", decoded.VerificationError)
	}
}

func TestInspectClassifiesExpiredCertificate(t *testing.T) {
	certificate, roots := makeTestCertificate(t, certificateOptions{
		NotBefore:   fixedNow.Add(-48 * time.Hour),
		NotAfter:    fixedNow.Add(-time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	server, host, port := startTLSServer(t, certificate)
	defer server.Close()

	tool := New(Config{
		AllowPrivateNetworks: true,
		Timeout:              time.Second,
		RootCAs:              roots,
		Now:                  func() time.Time { return fixedNow },
	})
	result, err := tool.Execute(context.Background(), mustJSON(t, input{Host: host, Port: port}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var decoded output
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if decoded.VerificationError == nil || decoded.VerificationError.Code != "certificate_expired" {
		t.Fatalf("unexpected verification error: %#v", decoded.VerificationError)
	}
	if decoded.Certificates[0].DaysRemaining >= 0 {
		t.Fatalf("expected negative days remaining: %#v", decoded.Certificates[0])
	}
}

func TestInspectClassifiesNotYetValidCertificate(t *testing.T) {
	certificate, roots := makeTestCertificate(t, certificateOptions{
		NotBefore:   fixedNow.Add(time.Hour),
		NotAfter:    fixedNow.Add(48 * time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	server, host, port := startTLSServer(t, certificate)
	defer server.Close()

	tool := New(Config{
		AllowPrivateNetworks: true,
		Timeout:              time.Second,
		RootCAs:              roots,
		Now:                  func() time.Time { return fixedNow },
	})
	result, err := tool.Execute(context.Background(), mustJSON(t, input{Host: host, Port: port}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var decoded output
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if decoded.VerificationError == nil || decoded.VerificationError.Code != "certificate_not_yet_valid" {
		t.Fatalf("unexpected verification error: %#v", decoded.VerificationError)
	}
}

func TestInspectBlocksPrivateTargetsByDefault(t *testing.T) {
	certificate, roots := makeTestCertificate(t, certificateOptions{
		NotBefore:   fixedNow.Add(-time.Hour),
		NotAfter:    fixedNow.Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	server, host, port := startTLSServer(t, certificate)
	defer server.Close()

	tool := New(Config{Timeout: time.Second, RootCAs: roots})
	_, err := tool.Execute(context.Background(), mustJSON(t, input{Host: host, Port: port}))
	if err == nil || !strings.Contains(err.Error(), "blocked_target") {
		t.Fatalf("expected blocked target error, got %v", err)
	}
}

func TestInspectTimesOutDuringHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()

	address := listener.Addr().(*net.TCPAddr)
	tool := New(Config{AllowPrivateNetworks: true, Timeout: 100 * time.Millisecond})
	_, err = tool.Execute(context.Background(), mustJSON(t, input{
		Host:      "127.0.0.1",
		Port:      address.Port,
		TimeoutMS: 100,
	}))
	select {
	case connection := <-accepted:
		connection.Close()
	default:
	}
	if err == nil || !strings.Contains(err.Error(), "tls_timeout") {
		t.Fatalf("expected TLS timeout, got %v", err)
	}
}

func TestInspectHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := New(Config{AllowPrivateNetworks: true, Timeout: time.Second})
	_, err := tool.Execute(ctx, json.RawMessage(`{"host":"127.0.0.1","port":443}`))
	if err == nil || !strings.Contains(err.Error(), "tls_canceled") {
		t.Fatalf("expected cancellation classification, got %v", err)
	}
}

func TestInspectStrictInputValidation(t *testing.T) {
	tool := New(Config{})
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"host":"example.com","unexpected":true}`),
		json.RawMessage(`{"host":"example.com"} {}`),
		json.RawMessage(`{"host":"example.com:443"}`),
		json.RawMessage(`{"host":"example.com","port":70000}`),
		json.RawMessage(`{"host":"example.com","timeout_ms":50}`),
	} {
		if _, err := tool.Execute(context.Background(), arguments); err == nil {
			t.Fatalf("expected validation failure for %s", arguments)
		}
	}
}

func TestSummarizeCertificatesSortsNames(t *testing.T) {
	certificate := &x509.Certificate{
		Subject:      pkix.Name{CommonName: "leaf"},
		Issuer:       pkix.Name{CommonName: "issuer"},
		SerialNumber: big.NewInt(15),
		DNSNames:     []string{"z.example", "a.example"},
		IPAddresses:  []net.IP{net.ParseIP("192.0.2.20"), net.ParseIP("192.0.2.10")},
		NotBefore:    fixedNow.Add(-time.Hour),
		NotAfter:     fixedNow.Add(24 * time.Hour),
	}
	summary, truncated := summarizeCertificates([]*x509.Certificate{certificate}, fixedNow)
	if truncated || len(summary) != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if strings.Join(summary[0].DNSNames, ",") != "a.example,z.example" {
		t.Fatalf("DNS names were not sorted: %#v", summary[0].DNSNames)
	}
	if strings.Join(summary[0].IPAddresses, ",") != "192.0.2.10,192.0.2.20" {
		t.Fatalf("IP addresses were not sorted: %#v", summary[0].IPAddresses)
	}
	if summary[0].SerialNumber != "F" {
		t.Fatalf("unexpected serial number: %s", summary[0].SerialNumber)
	}
}

type certificateOptions struct {
	NotBefore   time.Time
	NotAfter    time.Time
	DNSNames    []string
	IPAddresses []net.IP
}

func makeTestCertificate(t *testing.T, options certificateOptions) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "OpsPilot Test CA"},
		NotBefore:             fixedNow.Add(-365 * 24 * time.Hour),
		NotAfter:              fixedNow.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "OpsPilot Test Server"},
		NotBefore:    options.NotBefore,
		NotAfter:     options.NotAfter,
		DNSNames:     append([]string(nil), options.DNSNames...),
		IPAddresses:  append([]net.IP(nil), options.IPAddresses...),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCertificate, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	leafCertificate, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(caCertificate)
	return tls.Certificate{
		Certificate: [][]byte{leafDER, caDER},
		PrivateKey:  leafKey,
		Leaf:        leafCertificate,
	}, roots
}

func startTLSServer(t *testing.T, certificate tls.Certificate) (*httptest.Server, string, int) {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	address := server.Listener.Addr().(*net.TCPAddr)
	return server, "127.0.0.1", address.Port
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return payload
}
