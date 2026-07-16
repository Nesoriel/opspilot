package promapi

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
	defaultQueryTimeout     = 5 * time.Second
	defaultMaxResponseBytes = 4 << 20
	maxTokenBytes           = 64 << 10
)

type Config struct {
	BaseURL          string
	AllowHTTP        bool
	BearerTokenFile  string
	Timeout          time.Duration
	QueryTimeout     time.Duration
	MaxResponseBytes int64
}

type Client struct {
	config Config
	once   sync.Once

	baseURL    *url.URL
	httpClient *http.Client
	tokenFile  string
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
		if config.QueryTimeout <= 0 {
			config.QueryTimeout = defaultQueryTimeout
		}
		if config.QueryTimeout > config.Timeout {
			config.QueryTimeout = config.Timeout
		}
		if config.MaxResponseBytes <= 0 {
			config.MaxResponseBytes = defaultMaxResponseBytes
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
		c.config = config
		c.httpClient = &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("prometheus_redirect_blocked: redirects are not allowed")
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
		return nil, errors.New("prometheus_config_not_found: OPSPILOT_PROMETHEUS_URL is not configured")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, errors.New("prometheus_config_invalid: Prometheus URL could not be parsed")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("prometheus_config_unsafe: Prometheus URL must use HTTPS or explicitly allowed HTTP")
	}
	if parsed.Scheme == "http" && !allowHTTP {
		return nil, errors.New("prometheus_config_unsafe: HTTP requires OPSPILOT_PROMETHEUS_ALLOW_HTTP=true")
	}
	if parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("prometheus_config_unsafe: Prometheus URL must have a host and no user information")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("prometheus_config_unsafe: Prometheus URL must not contain a query or fragment")
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
		return "", errors.New("prometheus_config_invalid: bearer-token file path must be absolute")
	}
	return filepath.Clean(value), nil
}

func (c *Client) endpoint(apiPath string, query url.Values) string {
	result := *c.baseURL
	result.Path = strings.TrimSuffix(c.baseURL.Path, "/") + apiPath
	result.RawQuery = query.Encode()
	return result.String()
}

func (c *Client) get(ctx context.Context, apiPath string, query url.Values, output any) (responseMeta, error) {
	if err := c.ready(); err != nil {
		return responseMeta{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(apiPath, query), nil)
	if err != nil {
		return responseMeta{}, errors.New("prometheus_request_invalid: request could not be created")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "opspilot/prometheus-readonly")
	if c.tokenFile != "" {
		token, err := readBearerToken(c.tokenFile)
		if err != nil {
			return responseMeta{}, err
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return responseMeta{}, classifyTransportError(err)
	}
	defer response.Body.Close()

	payload, err := readBounded(response.Body, c.config.MaxResponseBytes)
	if err != nil {
		return responseMeta{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseMeta{}, classifyHTTPStatus(response.StatusCode)
	}

	var envelope apiEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return responseMeta{}, errors.New("prometheus_invalid_response: response JSON could not be decoded")
	}
	if envelope.Status != "success" {
		return responseMeta{}, classifyAPIEnvelope(envelope)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return responseMeta{}, errors.New("prometheus_invalid_response: response data is empty")
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return responseMeta{}, errors.New("prometheus_invalid_response: response data has an unexpected shape")
	}
	return responseMeta{WarningCount: len(envelope.Warnings), InfoCount: len(envelope.Infos)}, nil
}

func readBearerToken(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("prometheus_token_not_found: bearer-token file was not found")
		}
		if os.IsPermission(err) {
			return "", errors.New("prometheus_token_permission_denied: bearer-token file could not be read")
		}
		return "", errors.New("prometheus_token_read_failed: bearer-token file could not be opened")
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxTokenBytes+1))
	if err != nil {
		return "", errors.New("prometheus_token_read_failed: bearer-token file could not be read")
	}
	if len(payload) > maxTokenBytes {
		return "", errors.New("prometheus_token_invalid: bearer token is too large")
	}
	token := strings.TrimSpace(string(payload))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("prometheus_token_invalid: bearer token must be a single non-empty line")
	}
	return token, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, errors.New("prometheus_response_read_failed: response could not be read")
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("prometheus_response_too_large: response exceeds %d bytes", limit)
	}
	return payload, nil
}

type apiEnvelope struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Warnings  []string        `json:"warnings"`
	Infos     []string        `json:"infos"`
}

type responseMeta struct {
	WarningCount int
	InfoCount    int
}

func classifyTransportError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return errors.New("prometheus_canceled: request was canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("prometheus_timeout: request timed out")
	}
	var networkError interface{ Timeout() bool }
	if errors.As(err, &networkError) && networkError.Timeout() {
		return errors.New("prometheus_timeout: request timed out")
	}
	if strings.Contains(err.Error(), "prometheus_redirect_blocked") {
		return errors.New("prometheus_redirect_blocked: redirects are not allowed")
	}
	return errors.New("prometheus_unreachable: Prometheus server could not be reached")
}

func classifyHTTPStatus(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return errors.New("prometheus_unauthorized: credentials were rejected")
	case http.StatusForbidden:
		return errors.New("prometheus_forbidden: access was denied")
	case http.StatusTooManyRequests:
		return errors.New("prometheus_rate_limited: request rate was limited")
	case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return errors.New("prometheus_timeout: query timed out or service was unavailable")
	default:
		return fmt.Errorf("prometheus_http_error: server returned HTTP %d", statusCode)
	}
}

func classifyAPIEnvelope(envelope apiEnvelope) error {
	switch sanitizeToken(envelope.ErrorType) {
	case "timeout", "canceled":
		return errors.New("prometheus_timeout: query timed out or was canceled")
	case "bad_data":
		return errors.New("prometheus_query_invalid: generated query was rejected")
	case "execution":
		return errors.New("prometheus_query_failed: query execution failed")
	default:
		return errors.New("prometheus_api_error: Prometheus API returned an error")
	}
}

func sanitizeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return ""
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-') {
			return ""
		}
	}
	return strings.ToLower(value)
}
