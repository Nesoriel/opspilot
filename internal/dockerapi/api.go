package dockerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

const maxErrorMessageLength = 512

type versionCache struct {
	mu      sync.RWMutex
	version *Version
}

func (c *Client) Version(ctx context.Context) (Version, error) {
	c.versionCache.mu.RLock()
	if c.versionCache.version != nil {
		cached := *c.versionCache.version
		c.versionCache.mu.RUnlock()
		return cached, nil
	}
	c.versionCache.mu.RUnlock()

	var raw rawVersion
	if err := c.getJSON(ctx, false, "", "/version", nil, &raw); err != nil {
		return Version{}, err
	}
	version := Version{
		EngineVersion: raw.Version,
		APIVersion:    raw.APIVersion,
		MinAPIVersion: raw.MinAPIVersion,
		GitCommit:     raw.GitCommit,
		GoVersion:     raw.GoVersion,
		OS:            raw.OS,
		Arch:          raw.Arch,
		KernelVersion: raw.KernelVersion,
		BuildTime:     raw.BuildTime,
	}
	if !validAPIVersion(version.APIVersion) {
		return Version{}, fmt.Errorf("docker_invalid_response: daemon returned invalid API version %q", version.APIVersion)
	}

	c.versionCache.mu.Lock()
	if c.versionCache.version == nil {
		cached := version
		c.versionCache.version = &cached
	}
	version = *c.versionCache.version
	c.versionCache.mu.Unlock()
	return version, nil
}

func (c *Client) EngineInfo(ctx context.Context) (EngineInfo, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return EngineInfo{}, err
	}
	var raw rawInfo
	if err := c.getJSON(ctx, true, version.APIVersion, "/info", nil, &raw); err != nil {
		return EngineInfo{}, err
	}
	return mapEngineInfo(version, raw), nil
}

func (c *Client) ContainerList(ctx context.Context, options ContainerListOptions) ([]ContainerSummary, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	if options.All {
		query.Set("all", "1")
	}
	if options.Limit > 0 {
		query.Set("limit", strconv.Itoa(options.Limit))
	}

	var raw []rawContainerSummary
	if err := c.getJSON(ctx, true, version.APIVersion, "/containers/json", query, &raw); err != nil {
		return nil, err
	}
	return mapContainerSummaries(raw), nil
}

func (c *Client) ContainerInspect(ctx context.Context, identifier string) (ContainerInspect, error) {
	if err := validateContainerIdentifier(identifier); err != nil {
		return ContainerInspect{}, err
	}
	version, err := c.Version(ctx)
	if err != nil {
		return ContainerInspect{}, err
	}
	var raw rawContainerInspect
	path := "/containers/" + url.PathEscape(identifier) + "/json"
	if err := c.getJSON(ctx, true, version.APIVersion, path, nil, &raw); err != nil {
		return ContainerInspect{}, err
	}
	return mapContainerInspect(raw), nil
}

func (c *Client) getJSON(ctx context.Context, versioned bool, apiVersion, path string, query url.Values, output any) error {
	requestPath := path
	if versioned {
		requestPath = "/v" + apiVersion + path
	}
	requestURL := "http://docker" + requestPath
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("docker_request_invalid: create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "opspilot/docker-readonly")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return classifyTransportError("request Docker Engine", err)
	}
	defer response.Body.Close()

	payload, err := readBounded(response.Body, c.maxResponseBytes)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return dockerAPIError(response.StatusCode, payload)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return errors.New("docker_invalid_response: Docker Engine returned an empty response")
	}
	if err := json.Unmarshal(payload, output); err != nil {
		return fmt.Errorf("docker_invalid_response: decode Docker Engine response: %w", err)
	}
	return nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("docker_response_read_failed: %w", err)
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("docker_response_too_large: response exceeds %d bytes", limit)
	}
	return payload, nil
}

func dockerAPIError(statusCode int, payload []byte) error {
	message := strings.TrimSpace(string(payload))
	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(payload, &body) == nil && strings.TrimSpace(body.Message) != "" {
		message = strings.TrimSpace(body.Message)
	}
	message = truncate(message, maxErrorMessageLength)
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return fmt.Errorf("docker_api_error: HTTP %d: %s", statusCode, message)
}

func validAPIVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 6 {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func validateContainerIdentifier(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("docker_container_invalid: container identifier is required")
	}
	if len(value) > 128 {
		return errors.New("docker_container_invalid: container identifier is too long")
	}
	for index, character := range value {
		valid := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			(index > 0 && (character == '_' || character == '.' || character == '-'))
		if !valid {
			return errors.New("docker_container_invalid: identifier contains unsupported characters")
		}
	}
	return nil
}

func truncate(value string, limit int) string {
	characters := []rune(value)
	if len(characters) <= limit {
		return value
	}
	return string(characters[:limit]) + "..."
}
