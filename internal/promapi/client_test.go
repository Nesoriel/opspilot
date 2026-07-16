package promapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientMapsRedactedServerTargetsAndMetricSnapshot(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("rotating-test-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer rotating-test-token" {
			t.Errorf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("User-Agent") != "opspilot/prometheus-readonly" {
			t.Errorf("unexpected user agent: %q", request.Header.Get("User-Agent"))
		}
		switch request.URL.Path {
		case "/prom/api/v1/status/buildinfo":
			writeEnvelope(t, writer, map[string]any{
				"version":   "3.10.0",
				"revision":  "abc123",
				"branch":    "HEAD",
				"buildDate": "20260716-12:00:00",
				"goVersion": "go1.26.5",
				"buildUser": "secret-user@secret-host",
			}, []string{"warning with /secret/path"}, []string{"info with token=secret"})
		case "/prom/api/v1/status/runtimeinfo":
			writeEnvelope(t, writer, map[string]any{
				"startTime":           "2026-07-16T10:00:00Z",
				"serverTime":          "2026-07-16T12:00:00Z",
				"lastConfigTime":      "2026-07-16T11:00:00Z",
				"reloadConfigSuccess": true,
				"timeSeriesCount":     12345,
				"corruptionCount":     0,
				"goroutineCount":      42,
				"GOMAXPROCS":          8,
				"storageRetention":    "15d",
				"CWD":                 "/srv/prometheus/private",
				"hostname":            "secret-monitoring-host",
			}, nil, nil)
		case "/prom/api/v1/targets":
			if request.Method != http.MethodGet {
				t.Errorf("unexpected targets method: %s", request.Method)
			}
			if request.URL.Query().Get("state") != "active" {
				t.Errorf("unexpected target state: %s", request.URL.RawQuery)
			}
			writeEnvelope(t, writer, map[string]any{
				"activeTargets": []map[string]any{
					{
						"discoveredLabels":   map[string]string{"__address__": "secret.internal:9100", "token": "secret-label"},
						"labels":             map[string]string{"job": "node", "instance": "node-b:9100", "secret": "do-not-return"},
						"scrapePool":         "node",
						"scrapeUrl":          "http://user:password@secret.internal:9100/metrics?token=secret",
						"globalUrl":          "https://prometheus.example/graph?g0.expr=secret",
						"lastError":          "dial /private/path with password=secret",
						"lastScrape":         "2026-07-16T11:59:30Z",
						"lastScrapeDuration": 0.25,
						"health":             "down",
						"scrapeInterval":     "15s",
						"scrapeTimeout":      "10s",
					},
					{
						"labels":             map[string]string{"job": "node", "instance": "node-a:9100"},
						"scrapePool":         "node",
						"lastError":          "",
						"lastScrape":         "2026-07-16T11:59:45Z",
						"lastScrapeDuration": 0.1,
						"health":             "up",
						"scrapeInterval":     "15s",
						"scrapeTimeout":      "10s",
					},
				},
				"droppedTargets": []map[string]any{{"discoveredLabels": map[string]string{"secret": "dropped-secret"}}},
			}, []string{"target warning secret"}, nil)
		case "/prom/api/v1/query":
			if request.Method != http.MethodPost {
				t.Errorf("unexpected query method: %s", request.Method)
			}
			if request.URL.RawQuery != "" {
				t.Errorf("query parameters leaked into URL: %s", request.URL.RawQuery)
			}
			if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("unexpected content type: %q", request.Header.Get("Content-Type"))
			}
			if err := request.ParseForm(); err != nil {
				t.Errorf("parse query form: %v", err)
			}
			form := request.PostForm
			if form.Get("query") != `sum by (instance,job) (up{instance="node-a:9100",job="node"})` {
				t.Errorf("unexpected generated query: %q", form.Get("query"))
			}
			if form.Get("limit") != "1" || form.Get("timeout") != "2s" {
				t.Errorf("unexpected query bounds: %s", form.Encode())
			}
			writeEnvelope(t, writer, map[string]any{
				"resultType": "vector",
				"result": []map[string]any{
					{
						"metric": map[string]string{"job": "node", "instance": "node-b:9100", "secret_label": "secret-value"},
						"value":  []any{float64(1784203200), "0"},
					},
					{
						"metric": map[string]string{"job": "node", "instance": "node-a:9100", "namespace": "operations"},
						"value":  []any{float64(1784203201.5), "1"},
					},
				},
			}, []string{"query warning /secret"}, []string{"query info secret"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:          server.URL + "/prom/",
		AllowHTTP:        true,
		BearerTokenFile:  tokenFile,
		Timeout:          3 * time.Second,
		QueryTimeout:     2 * time.Second,
		MaxResponseBytes: 1 << 20,
	})

	serverInfo, err := client.ServerInfo(context.Background())
	if err != nil {
		t.Fatalf("server info: %v", err)
	}
	if serverInfo.Build.Version != "3.10.0" || serverInfo.Runtime.TimeSeriesCount != 12345 || serverInfo.WarningCount != 1 || serverInfo.InfoCount != 1 {
		t.Fatalf("unexpected server info: %#v", serverInfo)
	}

	targets, err := client.TargetList(context.Background(), 10)
	if err != nil {
		t.Fatalf("target list: %v", err)
	}
	if targets.Count != 2 || targets.Targets[0].Instance != "node-a:9100" || !targets.Targets[1].ErrorPresent {
		t.Fatalf("unexpected targets: %#v", targets)
	}

	snapshot, err := client.MetricSnapshot(context.Background(), MetricSnapshotRequest{
		Metric:      " up ",
		Matchers:    map[string]string{"job": "node", "instance": "node-a:9100"},
		Aggregation: "sum",
		GroupBy:     []string{"job", "instance"},
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("metric snapshot: %v", err)
	}
	if snapshot.Metric != "up" || snapshot.Count != 1 || !snapshot.Truncated || snapshot.Series[0].Labels["instance"] != "node-a:9100" {
		t.Fatalf("unexpected metric snapshot: %#v", snapshot)
	}

	payload, err := json.Marshal(map[string]any{"server": serverInfo, "targets": targets, "snapshot": snapshot})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	for _, secret := range []string{
		"secret-user",
		"secret-monitoring-host",
		"/srv/prometheus/private",
		"secret.internal",
		"user:password",
		"secret-label",
		"do-not-return",
		"password=secret",
		"dropped-secret",
		"secret_label",
		"secret-value",
		"warning with",
		"info with",
		"query warning",
		"query info",
	} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("sensitive value %q leaked into output: %s", secret, payload)
		}
	}
}

func TestClientConfigurationAndTransportFailures(t *testing.T) {
	t.Run("missing URL", func(t *testing.T) {
		_, err := New(Config{}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "prometheus_config_not_found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("HTTP requires opt-in", func(t *testing.T) {
		_, err := New(Config{BaseURL: "http://127.0.0.1:9090"}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "PROMETHEUS_ALLOW_HTTP") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	for name, baseURL := range map[string]string{
		"userinfo": "https://user:password@prometheus.example",
		"query":    "https://prometheus.example?token=secret",
		"fragment": "https://prometheus.example/#secret",
		"scheme":   "ftp://prometheus.example",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(Config{BaseURL: baseURL}).ServerInfo(context.Background())
			if err == nil || !strings.Contains(err.Error(), "prometheus_config_") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	t.Run("relative token file", func(t *testing.T) {
		_, err := New(Config{BaseURL: "https://prometheus.example", BearerTokenFile: "token"}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "bearer-token file path must be absolute") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "/elsewhere", http.StatusFound)
		}))
		defer server.Close()
		_, err := New(Config{BaseURL: server.URL, AllowHTTP: true}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "prometheus_redirect_blocked") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		defer server.Close()
		_, err := New(Config{BaseURL: server.URL, AllowHTTP: true, Timeout: 30 * time.Millisecond}).ServerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), "prometheus_timeout") {
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
		if err == nil || !strings.Contains(err.Error(), "prometheus_canceled") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestClientResponseFailures(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		maxBytes  int64
		errorCode string
	}{
		{
			name: "malformed JSON",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(`{"status":`))
			},
			errorCode: "prometheus_invalid_response",
		},
		{
			name: "oversized response",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(strings.Repeat("x", 65)))
			},
			maxBytes:  64,
			errorCode: "prometheus_response_too_large",
		},
		{
			name: "API execution error",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(writer).Encode(map[string]any{"status": "error", "errorType": "execution", "error": "secret query detail"})
			},
			errorCode: "prometheus_query_failed",
		},
		{
			name: "unauthorized",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte("secret auth detail"))
			},
			errorCode: "prometheus_unauthorized",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			client := New(Config{BaseURL: server.URL, AllowHTTP: true, MaxResponseBytes: test.maxBytes})
			_, err := client.ServerInfo(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.errorCode) {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("raw server error leaked: %v", err)
			}
		})
	}
}

func writeEnvelope(t *testing.T, writer http.ResponseWriter, data any, warnings, infos []string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"status":   "success",
		"data":     data,
		"warnings": warnings,
		"infos":    infos,
	}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func TestValidateBaseURLPreservesSafePrefix(t *testing.T) {
	parsed, err := validateBaseURL("https://prometheus.example/root/prometheus/", false)
	if err != nil {
		t.Fatalf("validate URL: %v", err)
	}
	if parsed.Path != "/root/prometheus" {
		t.Fatalf("unexpected path: %q", parsed.Path)
	}
	client := New(Config{BaseURL: parsed.String()})
	client.initialize()
	query := url.Values{"state": []string{"active"}}
	if endpoint := client.endpoint("/api/v1/targets", query); endpoint != "https://prometheus.example/root/prometheus/api/v1/targets?state=active" {
		t.Fatalf("unexpected endpoint: %q", endpoint)
	}
}
