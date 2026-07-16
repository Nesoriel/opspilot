package lokidiag

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
		Name:        "loki_server_info",
		Description: "Inspect configured Loki readiness and a redacted build-information projection without reading configuration, metrics, services, rings, rules, or logs.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
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
