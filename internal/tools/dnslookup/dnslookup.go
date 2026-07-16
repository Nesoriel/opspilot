package dnslookup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

type Tool struct {
	resolver Resolver
}

type input struct {
	Host string `json:"host"`
}

type output struct {
	Host      string   `json:"host"`
	Addresses []string `json:"addresses"`
}

func New(resolver Resolver) *Tool {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Tool{resolver: resolver}
}

func (t *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "dns_lookup",
		Description: "Resolve a DNS hostname to its IP addresses. This is a read-only diagnostic tool.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"host":{"type":"string","description":"DNS hostname to resolve"}},"required":["host"],"additionalProperties":false}`),
	}
}

func (t *Tool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var request input
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}
	if request.Host == "" {
		return nil, errors.New("host is required")
	}

	addresses, err := t.resolver.LookupHost(ctx, request.Host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", request.Host, err)
	}
	sort.Strings(addresses)

	payload, err := json.Marshal(output{Host: request.Host, Addresses: addresses})
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return payload, nil
}
