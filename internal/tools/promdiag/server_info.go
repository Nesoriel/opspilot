package promdiag

import (
	"context"
	"encoding/json"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type ServerInfoTool struct {
	client Client
}

func NewServerInfo(client Client) *ServerInfoTool {
	return &ServerInfoTool{client: client}
}

func (t *ServerInfoTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "prometheus_server_info",
		Description: "Inspect a configured Prometheus server through redacted build and runtime status fields without returning configuration, flags, hostnames, filesystem paths, or warning text.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (t *ServerInfoTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input struct{}
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	result, err := t.client.ServerInfo(ctx)
	if err != nil {
		return nil, err
	}
	return encodeResult(result)
}
