package httpprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/netguard"
)

type Config struct {
	AllowPrivateNetworks bool
	Timeout              time.Duration
	MaxRedirects         int
	Resolver             *net.Resolver
}

type Tool struct {
	client *http.Client
}

type input struct {
	URL string `json:"url"`
}

type output struct {
	URL        string            `json:"url"`
	FinalURL   string            `json:"final_url"`
	Status     string            `json:"status"`
	StatusCode int               `json:"status_code"`
	LatencyMS  int64             `json:"latency_ms"`
	Headers    map[string]string `json:"headers,omitempty"`
}

func New(config Config) *Tool {
	if config.Timeout <= 0 {
		config.Timeout = 15 * time.Second
	}
	if config.MaxRedirects <= 0 {
		config.MaxRedirects = 5
	}
	if config.Resolver == nil {
		config.Resolver = net.DefaultResolver
	}

	dialer := &netguard.Dialer{
		Resolver:     config.Resolver,
		AllowPrivate: config.AllowPrivateNetworks,
		Dialer:       net.Dialer{Timeout: config.Timeout},
	}
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        32,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	return &Tool{client: &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= config.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", config.MaxRedirects)
			}
			return nil
		},
	}}
}

func (t *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "http_probe",
		Description: "Send a read-only HTTP HEAD request and report status, latency, redirects, and selected headers.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"HTTP or HTTPS URL to probe"}},"required":["url"],"additionalProperties":false}`),
	}
}

func (t *Tool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var request input
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}
	if request.URL == "" {
		return nil, errors.New("url is required")
	}

	parsed, err := url.Parse(request.URL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("only http and https URLs are supported")
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("url hostname is required")
	}
	if parsed.User != nil {
		return nil, errors.New("url user information is not allowed")
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodHead, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpRequest.Header.Set("User-Agent", "opspilot/0.1")

	started := time.Now()
	response, err := t.client.Do(httpRequest)
	latency := time.Since(started)
	if err != nil {
		return nil, fmt.Errorf("probe %q: %w", request.URL, err)
	}
	defer response.Body.Close()

	selectedHeaders := make(map[string]string)
	for _, name := range []string{"Content-Type", "Content-Length", "Location", "Server"} {
		if value := response.Header.Get(name); value != "" {
			selectedHeaders[strings.ToLower(name)] = value
		}
	}

	payload, err := json.Marshal(output{
		URL:        request.URL,
		FinalURL:   response.Request.URL.String(),
		Status:     response.Status,
		StatusCode: response.StatusCode,
		LatencyMS:  latency.Milliseconds(),
		Headers:    selectedHeaders,
	})
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return payload, nil
}
