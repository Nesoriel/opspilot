package dockerdiag

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/dockerapi"
)

const (
	defaultContainerLimit = 100
	maxContainerLimit     = 200
)

type ContainerListTool struct {
	client Client
}

type containerListInput struct {
	All   *bool `json:"all,omitempty"`
	Limit int   `json:"limit,omitempty"`
}

type containerListOutput struct {
	Count      int                          `json:"count"`
	Containers []dockerapi.ContainerSummary `json:"containers"`
}

func NewContainerList(client Client) *ContainerListTool {
	return &ContainerListTool{client: client}
}

func (t *ContainerListTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "docker_container_list",
		Description: "List local Docker containers with bounded, read-only state, health, port, and network summaries. Includes stopped containers by default.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"all":{"type":"boolean","default":true,"description":"Include stopped containers"},"limit":{"type":"integer","minimum":1,"maximum":200,"default":100}},"additionalProperties":false}`),
	}
}

func (t *ContainerListTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	input := containerListInput{Limit: defaultContainerLimit}
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	all := true
	if input.All != nil {
		all = *input.All
	}
	if input.Limit == 0 {
		input.Limit = defaultContainerLimit
	}
	if input.Limit < 1 || input.Limit > maxContainerLimit {
		return nil, errors.New("limit must be between 1 and 200")
	}

	containers, err := t.client.ContainerList(ctx, dockerapi.ContainerListOptions{All: all, Limit: input.Limit})
	if err != nil {
		return nil, err
	}
	if len(containers) > input.Limit {
		containers = containers[:input.Limit]
	}
	return encodeResult(containerListOutput{Count: len(containers), Containers: containers})
}
