package dockerdiag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type ContainerInspectTool struct {
	client Client
}

type containerInspectInput struct {
	Container string `json:"container"`
}

func NewContainerInspect(client Client) *ContainerInspectTool {
	return &ContainerInspectTool{client: client}
}

func (t *ContainerInspectTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "docker_container_inspect",
		Description: "Inspect one local Docker container through a redacted read-only view that omits commands, environment values, raw labels, health-check output, and host mount source paths.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"container":{"type":"string","minLength":1,"maxLength":128,"description":"Container name or ID"}},"required":["container"],"additionalProperties":false}`),
	}
}

func (t *ContainerInspectTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input containerInspectInput
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	input.Container = strings.TrimSpace(input.Container)
	if input.Container == "" {
		return nil, errors.New("container is required")
	}
	result, err := t.client.ContainerInspect(ctx, input.Container)
	if err != nil {
		return nil, err
	}
	return encodeResult(result)
}
