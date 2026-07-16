package httpprobe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeAllowsPrivateWhenConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	tool := New(Config{AllowPrivateNetworks: true, Timeout: time.Second})
	arguments, _ := json.Marshal(map[string]string{"url": server.URL})
	result, err := tool.Execute(context.Background(), arguments)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(string(result), `"status_code":204`) {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestProbeBlocksPrivateByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	tool := New(Config{Timeout: time.Second})
	arguments, _ := json.Marshal(map[string]string{"url": server.URL})
	if _, err := tool.Execute(context.Background(), arguments); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked address error, got %v", err)
	}
}

func TestProbeRejectsUnsupportedScheme(t *testing.T) {
	tool := New(Config{})
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"file:///etc/passwd"}`)); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
}
