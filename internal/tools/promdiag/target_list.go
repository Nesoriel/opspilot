package promdiag

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Nesoriel/opspilot/internal/agent"
)

const (
	defaultTargetLimit = 100
	maxTargetLimit     = 500
)

type TargetListTool struct {
	client Client
}

type targetListInput struct {
	Limit *int `json:"limit,omitempty"`
}

func NewTargetList(client Client) *TargetListTool {
	return &TargetListTool{client: client}
}

func (t *TargetListTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "prometheus_target_list",
		Description: "List bounded active Prometheus scrape targets with redacted health and timing fields, without scrape URLs, discovered labels, arbitrary labels, or last-error text.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":500,"default":100}},"additionalProperties":false}`),
	}
}

func (t *TargetListTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input targetListInput
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	limit := defaultTargetLimit
	if input.Limit != nil {
		limit = *input.Limit
	}
	if limit < 1 || limit > maxTargetLimit {
		return nil, errors.New("limit must be between 1 and 500")
	}
	result, err := t.client.TargetList(ctx, limit)
	if err != nil {
		return nil, err
	}
	return encodeResult(result)
}
