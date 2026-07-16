package dockerapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientNegotiatesVersionAndMapsEngineInfo(t *testing.T) {
	var versionCalls atomic.Int32
	socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/version":
			versionCalls.Add(1)
			writeJSONResponse(t, writer, map[string]any{
				"Version":       "29.1.0",
				"ApiVersion":    "1.55",
				"MinAPIVersion": "1.24",
				"GitCommit":     "abc123",
				"GoVersion":     "go1.26.5",
				"Os":            "linux",
				"Arch":          "amd64",
				"KernelVersion": "6.12.0",
			})
		case "/v1.55/info":
			writeJSONResponse(t, writer, map[string]any{
				"Containers":         5,
				"ContainersRunning":  2,
				"ContainersPaused":   1,
				"ContainersStopped":  2,
				"Images":             8,
				"Driver":             "overlay2",
				"LoggingDriver":      "json-file",
				"CgroupDriver":       "systemd",
				"CgroupVersion":      "2",
				"KernelVersion":      "6.12.0",
				"OperatingSystem":    "Debian GNU/Linux 13",
				"OSVersion":          "13",
				"OSType":             "linux",
				"Architecture":       "x86_64",
				"NCPU":               8,
				"MemTotal":           int64(16 << 30),
				"DefaultRuntime":     "runc",
				"LiveRestoreEnabled": true,
				"ExperimentalBuild":  false,
				"SecurityOptions":    []string{"name=seccomp", "name=apparmor"},
				"Warnings":           []string{" warning two ", "warning one"},
				"DockerRootDir":      "/sensitive/docker/root",
				"HttpProxy":          "http://secret-proxy",
			})
		default:
			http.NotFound(writer, request)
		}
	}))

	client, err := New(Config{SocketPath: socketPath, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	info, err := client.EngineInfo(context.Background())
	if err != nil {
		t.Fatalf("engine info: %v", err)
	}
	if info.Version.APIVersion != "1.55" || info.Version.EngineVersion != "29.1.0" {
		t.Fatalf("unexpected version: %#v", info.Version)
	}
	if info.ContainersRunning != 2 || info.StorageDriver != "overlay2" || info.CgroupVersion != "2" {
		t.Fatalf("unexpected engine info: %#v", info)
	}
	if strings.Join(info.SecurityOptions, ",") != "name=apparmor,name=seccomp" {
		t.Fatalf("security options were not sorted: %#v", info.SecurityOptions)
	}
	payload, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	for _, secret := range []string{"/sensitive/docker/root", "secret-proxy"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("sensitive field leaked into output: %s", payload)
		}
	}
	if _, err := client.Version(context.Background()); err != nil {
		t.Fatalf("cached version: %v", err)
	}
	if versionCalls.Load() != 1 {
		t.Fatalf("version endpoint called %d times, want 1", versionCalls.Load())
	}
}

func TestClientClassifiesProtocolFailures(t *testing.T) {
	t.Run("unsupported socket scheme", func(t *testing.T) {
		if _, err := New(Config{SocketPath: "tcp://127.0.0.1:2375"}); err == nil || !strings.Contains(err.Error(), "docker_socket_unsupported") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("relative socket", func(t *testing.T) {
		if _, err := New(Config{SocketPath: "docker.sock"}); err == nil || !strings.Contains(err.Error(), "docker_socket_invalid") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing socket", func(t *testing.T) {
		client, err := New(Config{SocketPath: filepath.Join(t.TempDir(), "missing.sock"), Timeout: 100 * time.Millisecond})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		if _, err := client.Version(context.Background()); err == nil || !strings.Contains(err.Error(), "docker_socket_not_found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("permission denied", func(t *testing.T) {
		err := classifyTransportError("connect Docker Engine", os.ErrPermission)
		if !strings.Contains(err.Error(), "docker_permission_denied") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("API error", func(t *testing.T) {
		socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusInternalServerError)
			writeJSONResponse(t, writer, map[string]string{"message": "daemon unavailable"})
		}))
		client, err := New(Config{SocketPath: socketPath, Timeout: time.Second})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		if _, err := client.Version(context.Background()); err == nil || !strings.Contains(err.Error(), "docker_api_error: HTTP 500: daemon unavailable") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed response", func(t *testing.T) {
		socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"ApiVersion":`))
		}))
		client, err := New(Config{SocketPath: socketPath, Timeout: time.Second})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		if _, err := client.Version(context.Background()); err == nil || !strings.Contains(err.Error(), "docker_invalid_response") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid API version", func(t *testing.T) {
		socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, writer, map[string]string{"ApiVersion": "latest"})
		}))
		client, err := New(Config{SocketPath: socketPath, Timeout: time.Second})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		if _, err := client.Version(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid API version") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("oversized response", func(t *testing.T) {
		socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(strings.Repeat("x", 65)))
		}))
		client, err := New(Config{SocketPath: socketPath, Timeout: time.Second, MaxResponseBytes: 64})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		if _, err := client.Version(context.Background()); err == nil || !strings.Contains(err.Error(), "docker_response_too_large") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		client, err := New(Config{SocketPath: socketPath, Timeout: 30 * time.Millisecond})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		if _, err := client.Version(context.Background()); err == nil || !strings.Contains(err.Error(), "docker_timeout") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("canceled", func(t *testing.T) {
		socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		client, err := New(Config{SocketPath: socketPath, Timeout: time.Second})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := client.Version(ctx); err == nil || !strings.Contains(err.Error(), "docker_canceled") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestReadBounded(t *testing.T) {
	payload, err := readBounded(strings.NewReader("hello"), 5)
	if err != nil || string(payload) != "hello" {
		t.Fatalf("unexpected result %q, %v", payload, err)
	}
	if _, err := readBounded(strings.NewReader("hello!"), 5); err == nil {
		t.Fatal("expected oversized response error")
	}
}

func TestValidateContainerIdentifier(t *testing.T) {
	for _, value := range []string{"web", "web-1", "abc123", "service.name"} {
		if err := validateContainerIdentifier(value); err != nil {
			t.Fatalf("valid identifier %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "-web", "web/one", "web one", strings.Repeat("a", 129)} {
		if err := validateContainerIdentifier(value); err == nil {
			t.Fatalf("invalid identifier %q accepted", value)
		}
	}
}

func startUnixHTTPServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "docker.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on Unix socket: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		_ = listener.Close()
	})
	return socketPath
}

func writeJSONResponse(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Errorf("encode response: %v", err)
	}
}
