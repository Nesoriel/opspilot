package dockerapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestValidAPIVersionAcceptsOnlyTwoNumericComponents(t *testing.T) {
	for _, value := range []string{"1.24", "1.55", "12.345678"} {
		if !validAPIVersion(value) {
			t.Fatalf("valid API version %q rejected", value)
		}
	}
	for _, value := range []string{"", "latest", "1", "1.2.3", "-1.55", "1.-1", "1.a", "1.1234567"} {
		if validAPIVersion(value) {
			t.Fatalf("invalid API version %q accepted", value)
		}
	}
}

func TestTruncatePreservesUTF8(t *testing.T) {
	result := truncate("容器诊断错误", 4)
	if result != "容器诊断..." {
		t.Fatalf("truncate result = %q", result)
	}
	if !utf8.ValidString(result) {
		t.Fatalf("truncate produced invalid UTF-8: %q", result)
	}
}

func TestMissingSocketErrorIsClassifiedOnce(t *testing.T) {
	client, err := New(Config{
		SocketPath: filepath.Join(t.TempDir(), "missing.sock"),
		Timeout:    100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Version(context.Background())
	if err == nil {
		t.Fatal("expected missing socket error")
	}
	if count := strings.Count(err.Error(), "docker_socket_not_found"); count != 1 {
		t.Fatalf("socket error classified %d times: %v", count, err)
	}
}
