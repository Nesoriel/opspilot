package dockerdiag

import (
	"context"
	"encoding/json"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type EngineInfoTool struct {
	client Client
}

func NewEngineInfo(client Client) *EngineInfoTool {
	return &EngineInfoTool{client: client}
}

func (t *EngineInfoTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "docker_engine_info",
		Description: "Inspect the local Docker Engine version, platform, storage, cgroup, capacity, and container-count metadata through a read-only Unix-socket API call.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (t *EngineInfoTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input struct{}
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	result, err := t.client.EngineInfo(ctx)
	if err != nil {
		return nil, err
	}
	return encodeResult(result)
}
