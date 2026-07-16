package lokiapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout          = 8 * time.Second
	defaultMaxResponseBytes = 4 << 20
	maxTokenBytes           = 64 << 10
	maxTenantLength         = 128
)

type Config struct {
	BaseURL          string
	AllowHTTP        bool
	BearerTokenFile  string
	TenantID         string
	Timeout          time.Duration
	MaxResponseBytes int64
	Now              func() time.Time
}

type Client struct {
	config Config
	once   sync.Once

	baseURL    *url.URL
	httpClient *http.Client
	tokenFile  string
	tenantID   string
	initErr    error
}

func New(config Config) *Client {
	return &Client{config: config}
}

func (c *Client) initialize() {
	c.once.Do(func() {
		config := c.config
		if config.Timeout <= 0 {
			config.Timeout = defaultTimeout
		}
		if config.MaxResponseBytes <= 0 {
			config.MaxResponseBytes = defaultMaxResponseBytes
		}
		if config.Now == nil {
			config.Now = time.Now
		}

		baseURL, err := validateBaseURL(config.BaseURL, config.AllowHTTP)
		if err != nil {
			c.initErr = err
			return
		}
		tokenFile, err := validateTokenFile(config.BearerTokenFile)
		if err != nil {
			c.initErr = err
			return
		}
		tenantID, err := validateTenantID(config.TenantID)
		if err != nil {
			c.initErr = err
			return
		}

		transport := &http.Transport{
			Proxy:                 nil,
			DisableCompression:    true,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          8,
			IdleConnTimeout:       30 * time.Second,
			ResponseHeaderTimeout: config.Timeout,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		}
		c.baseURL = baseURL
		c.tokenFile = tokenFile
		c.tenantID = tenantID
		c.config = config
		c.httpClient = &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("loki_redirect_blocked: redirects are not allowed")
			},
		}
	})
}

func (c *Client) ready() error {
	c.initialize()
	return c.initErr
}

func validateBaseURL(value string, allowHTTP bool) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("loki_config_not_found: OPSPILOT_LOKI_URL is not configured")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, errors.New("loki_config_invalid: Loki URL could not be parsed")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("loki_config_unsafe: Loki URL must use HTTPS or explicitly allowed HTTP")
	}
	if parsed.Scheme == "http" && !allowHTTP {
		return nil, errors.New("loki_config_unsafe: HTTP requires OPSPILOT_LOKI_ALLOW_HTTP=true")
	}
	if parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("loki_config_unsafe: Loki URL must have a host and no user information")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("loki_config_unsafe: Loki URL must not contain a query or fragment")
	}
	cleanPath := path.Clean("/" + strings.TrimSpace(parsed.Path))
	if cleanPath == "/." || cleanPath == "/" {
		cleanPath = ""
	}
	parsed.Path = strings.TrimSuffix(cleanPath, "/")
	parsed.RawPath = ""
	return parsed, nil
}

func validateTokenFile(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("loki_config_invalid: bearer-token file path must be absolute")
	}
	return filepath.Clean(value), nil
}

func validateTenantID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > maxTenantLength {
		return "", errors.New("loki_config_invalid: tenant ID is too long")
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-') {
			return "", errors.New("loki_config_invalid: tenant ID contains unsupported characters")
		}
	}
	return value, nil
}

func (c *Client) endpoint(apiPath string, query url.Values) string {
	result := *c.baseURL
	result.Path = strings.TrimSuffix(c.baseURL.Path, "/") + apiPath
	result.RawQuery = query.Encode()
	return result.String()
}

func (c *Client) getJSON(ctx context.Context, apiPath string, output any) error {
	return c.requestJSON(ctx, http.MethodGet, apiPath, nil, output)
}

func (c *Client) postFormJSON(ctx context.Context, apiPath string, form url.Values, output any) error {
	return c.requestJSON(ctx, http.MethodPost, apiPath, form, output)
}

func (c *Client) requestJSON(ctx context.Context, method, apiPath string, form url.Values, output any) error {
	if err := c.ready(); err != nil {
		return err
	}

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, method, c.endpoint(apiPath, nil), body)
	if err != nil {
		return errors.New("loki_request_invalid: request could not be created")
	}
	if err := c.applyHeaders(request); err != nil {
		return err
	}
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return classifyTransportError(err)
	}
	defer response.Body.Close()

	payload, err := readBounded(response.Body, c.config.MaxResponseBytes)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return classifyHTTPStatus(response.StatusCode)
	}
	if err := json.Unmarshal(payload, output); err != nil {
		return errors.New("loki_invalid_response: response JSON could not be decoded")
	}
	return nil
}

func (c *Client) readiness(ctx context.Context) (bool, int, error) {
	if err := c.ready(); err != nil {
		return false, 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/ready", nil), nil)
	if err != nil {
		return false, 0, errors.New("loki_request_invalid: readiness request could not be created")
	}
	if err := c.applyHeaders(request); err != nil {
		return false, 0, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return false, 0, classifyTransportError(err)
	}
	defer response.Body.Close()
	if _, err := readBounded(response.Body, 64<<10); err != nil {
		return false, response.StatusCode, err
	}
	if response.StatusCode == http.StatusOK {
		return true, response.StatusCode, nil
	}
	if response.StatusCode == http.StatusServiceUnavailable {
		return false, response.StatusCode, nil
	}
	return false, response.StatusCode, classifyHTTPStatus(response.StatusCode)
}

func (c *Client) applyHeaders(request *http.Request) error {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "opspilot/loki-readonly")
	if c.tenantID != "" {
		request.Header.Set("X-Scope-OrgID", c.tenantID)
	}
	if c.tokenFile != "" {
		token, err := readBearerToken(c.tokenFile)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return nil
}

func readBearerToken(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("loki_token_not_found: bearer-token file was not found")
		}
		if os.IsPermission(err) {
			return "", errors.New("loki_token_permission_denied: bearer-token file could not be read")
		}
		return "", errors.New("loki_token_read_failed: bearer-token file could not be opened")
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxTokenBytes+1))
	if err != nil {
		return "", errors.New("loki_token_read_failed: bearer-token file could not be read")
	}
	if len(payload) > maxTokenBytes {
		return "", errors.New("loki_token_invalid: bearer token is too large")
	}
	token := strings.TrimSpace(string(payload))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("loki_token_invalid: bearer token must be a single non-empty line")
	}
	return token, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, errors.New("loki_response_read_failed: response could not be read")
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("loki_response_too_large: response exceeds %d bytes", limit)
	}
	return payload, nil
}

func classifyTransportError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return errors.New("loki_canceled: request was canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("loki_timeout: request timed out")
	}
	var networkError interface{ Timeout() bool }
	if errors.As(err, &networkError) && networkError.Timeout() {
		return errors.New("loki_timeout: request timed out")
	}
	if strings.Contains(err.Error(), "loki_redirect_blocked") {
		return errors.New("loki_redirect_blocked: redirects are not allowed")
	}
	return errors.New("loki_unreachable: Loki server could not be reached")
}

func classifyHTTPStatus(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return errors.New("loki_unauthorized: credentials were rejected")
	case http.StatusForbidden:
		return errors.New("loki_forbidden: access was denied")
	case http.StatusTooManyRequests:
		return errors.New("loki_rate_limited: request rate was limited")
	case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return errors.New("loki_timeout: service was unavailable or timed out")
	default:
		return fmt.Errorf("loki_http_error: server returned HTTP %d", statusCode)
	}
}
