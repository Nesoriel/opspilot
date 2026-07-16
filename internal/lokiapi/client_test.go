package lokiapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClientMapsServerAndRedactedStreams(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("rotating-loki-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	fixedNow := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer rotating-loki-token" {
			t.Errorf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("X-Scope-OrgID") != "operations" {
			t.Errorf("unexpected tenant header: %q", request.Header.Get("X-Scope-OrgID"))
		}
		if request.Header.Get("User-Agent") != "opspilot/loki-readonly" {
			t.Errorf("unexpected user agent: %q", request.Header.Get("User-Agent"))
		}
		switch request.URL.Path {
		case "/proxy/ready":
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("ready\n"))
		case "/proxy/loki/api/v1/status/buildinfo":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"version":   "3.5.0",
				"revision":  "abc123",
				"branch":    "HEAD",
				"buildDate": "2026-07-16",
				"goVersion": "go1.26.5",
				"buildUser": "secret-user@secret-host",
			})
		case "/proxy/loki/api/v1/series":
			if request.Method != http.MethodPost {
				t.Errorf("unexpected series method: %s", request.Method)
			}
			if request.URL.RawQuery != "" {
				t.Errorf("selector leaked into URL: %s", request.URL.RawQuery)
			}
			if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("unexpected content type: %q", request.Header.Get("Content-Type"))
			}
			if err := request.ParseForm(); err != nil {
				t.Errorf("parse series form: %v", err)
			}
			if request.PostForm.Get("match[]") != `{namespace="operations",service_name="api"}` {
				t.Errorf("unexpected selector: %q", request.PostForm.Get("match[]"))
			}
			if request.PostForm.Get("start") != strconv.FormatInt(fixedNow.Add(-time.Hour).UnixNano(), 10) {
				t.Errorf("unexpected start: %q", request.PostForm.Get("start"))
			}
			if request.PostForm.Get("end") != strconv.FormatInt(fixedNow.UnixNano(), 10) {
				t.Errorf("unexpected end: %q", request.PostForm.Get("end"))
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"status": "success",
				"data": []map[string]string{
					{"namespace": "operations", "service_name": "api", "pod": "api-b", "filename": "/secret/path", "token": "secret-value"},
					{"namespace": "operations", "service_name": "api", "pod": "api-a", "container": "web"},
				},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:          server.URL + "/proxy/",
		AllowHTTP:        true,
		BearerTokenFile:  tokenFile,
		TenantID:         "operations",
		Timeout:          2 * time.Second,
		MaxResponseBytes: 1 << 20,
		Now:              func() time.Time { return fixedNow },
	})

	serverInfo, err := client.ServerInfo(context.Background())
	if err != nil {
		t.Fatalf("server info: %v", err)
	}
	if !serverInfo.Ready || serverInfo.ReadyStatusCode != http.StatusOK || serverInfo.Build.Version != "3.5.0" {
		t.Fatalf("unexpected server info: %#v", serverInfo)
	}

	summary, err := client.StreamSummary(context.Background(), StreamSummaryRequest{
		Matchers: map[string]string{"service_name": "api", "namespace": "operations"},
		Lookback: 60,
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("stream summary: %v", err)
	}
	if summary.Count != 1 || !summary.Truncated || summary.Streams[0].Labels["pod"] != "api-a" {
		t.Fatalf("unexpected stream summary: %#v", summary)
	}
	payload, err := json.Marshal(map[string]any{"server": serverInfo, "summary": summary})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	for _, secret := range []string{"secret-user", "secret-host", "/secret/path", "secret-value", "filename", "token"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("sensitive value %q leaked: %s", secret, payload)
		}
	}
}

func TestClientReportsNotReadyWithoutLeakingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/ready":
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte("secret readiness detail"))
		case "/loki/api/v1/status/buildinfo":
			_ = json.NewEncoder(writer).Encode(map[string]any{"version": "3.5.0"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	result, err := New(Config{BaseURL: server.URL, AllowHTTP: true}).ServerInfo(context.Background())
	if err != nil {
		t.Fatalf("server info: %v", err)
	}
	if result.Ready || result.ReadyStatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected readiness: %#v", result)
	}
	payload, _ := json.Marshal(result)
	if strings.Contains(string(payload), "secret") {
		t.Fatalf("readiness body leaked: %s", payload)
	}
}

func TestClientConfigurationFailures(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		code   string
	}{
		{"missing URL", Config{}, "loki_config_not_found"},
		{"HTTP requires opt-in", Config{BaseURL: "http://127.0.0.1:3100"}, "LOKI_ALLOW_HTTP"},
		{"userinfo", Config{BaseURL: "https://user:password@loki.example"}, "loki_config_unsafe"},
		{"query", Config{BaseURL: "https://loki.example?token=secret"}, "loki_config_unsafe"},
		{"fragment", Config{BaseURL: "https://loki.example/#secret"}, "loki_config_unsafe"},
		{"scheme", Config{BaseURL: "ftp://loki.example"}, "loki_config_unsafe"},
		{"relative token", Config{BaseURL: "https://loki.example", BearerTokenFile: "token"}, "must be absolute"},
		{"multi tenant", Config{BaseURL: "https://loki.example", TenantID: "tenant-a|tenant-b"}, "tenant ID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.config).ServerInfo(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestClientTransportAndResponseFailures(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("request should not be sent")
		}))
		defer server.Close()
		_, err := New(Config{BaseURL: server.URL, AllowHTTP: true, BearerTokenFile: filepath.Join(t.TempDir(), "missing")}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "loki_token_not_found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "/elsewhere", http.StatusFound)
		}))
		defer server.Close()
		_, err := New(Config{BaseURL: server.URL, AllowHTTP: true}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "loki_redirect_blocked") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		defer server.Close()
		_, err := New(Config{BaseURL: server.URL, AllowHTTP: true, Timeout: 30 * time.Millisecond}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "loki_timeout") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := New(Config{BaseURL: server.URL, AllowHTTP: true}).ServerInfo(ctx)
		if err == nil || !strings.Contains(err.Error(), "loki_canceled") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("oversized readiness", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(strings.Repeat("x", (64<<10)+1)))
		}))
		defer server.Close()
		_, err := New(Config{BaseURL: server.URL, AllowHTTP: true}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "loki_response_too_large") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
