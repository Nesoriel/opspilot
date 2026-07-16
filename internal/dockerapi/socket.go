package dockerapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	defaultSocketPath      = "/var/run/docker.sock"
	defaultTimeout         = 5 * time.Second
	defaultMaxResponseSize = 4 << 20
)

type Config struct {
	SocketPath       string
	Timeout          time.Duration
	MaxResponseBytes int64
}

type Client struct {
	httpClient       *http.Client
	socketPath       string
	maxResponseBytes int64
	versionCache     versionCache
}

func New(config Config) (*Client, error) {
	socketPath, err := normalizeSocketPath(config.SocketPath)
	if err != nil {
		return nil, err
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = defaultMaxResponseSize
	}

	dialer := net.Dialer{Timeout: config.Timeout}
	transport := &http.Transport{
		Proxy:                  nil,
		DisableCompression:     true,
		MaxIdleConns:           4,
		IdleConnTimeout:        30 * time.Second,
		ResponseHeaderTimeout:  config.Timeout,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			connection, err := dialer.DialContext(ctx, "unix", socketPath)
			if err != nil {
				return nil, classifyTransportError("connect Docker Engine", err)
			}
			return connection, nil
		},
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
		},
		socketPath:       socketPath,
		maxResponseBytes: config.MaxResponseBytes,
	}, nil
}

func normalizeSocketPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultSocketPath
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("docker_socket_invalid: parse socket URI: %w", err)
		}
		if parsed.Scheme != "unix" {
			return "", errors.New("docker_socket_unsupported: only local unix sockets are supported")
		}
		if parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return "", errors.New("docker_socket_invalid: unix socket URI must contain only an absolute path")
		}
		value = parsed.Path
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("docker_socket_invalid: socket path must be absolute")
	}
	value = filepath.Clean(value)
	if value == string(filepath.Separator) {
		return "", errors.New("docker_socket_invalid: socket path cannot be the filesystem root")
	}
	return value, nil
}

func classifyTransportError(operation string, err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("docker_canceled: %s: %w", operation, err)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("docker_timeout: %s: %w", operation, err)
	case errors.Is(err, os.ErrNotExist), errors.Is(err, syscall.ENOENT):
		return fmt.Errorf("docker_socket_not_found: %s: %w", operation, err)
	case errors.Is(err, os.ErrPermission), errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return fmt.Errorf("docker_permission_denied: %s: %w", operation, err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return fmt.Errorf("docker_timeout: %s: %w", operation, err)
	}
	return fmt.Errorf("docker_daemon_unreachable: %s: %w", operation, err)
}
