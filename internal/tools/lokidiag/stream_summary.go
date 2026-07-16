package lokidiag

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/lokiapi"
)

type StreamSummaryTool struct {
	client Client
}

type streamSummaryInput struct {
	Matchers        map[string]string `json:"matchers"`
	LookbackMinutes *int              `json:"lookback_minutes,omitempty"`
	Limit           *int              `json:"limit,omitempty"`
}

func NewStreamSummary(client Client) *StreamSummaryTool {
	return &StreamSummaryTool{client: client}
}

func (t *StreamSummaryTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "loki_stream_summary",
		Description: "List a bounded redacted summary of Loki streams matching exact diagnostic labels over a limited lookback window without returning log lines or arbitrary labels.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"matchers":{"type":"object","minProperties":1,"maxProperties":8,"additionalProperties":{"type":"string","minLength":1,"maxLength":256}},"lookback_minutes":{"type":"integer","minimum":1,"maximum":360,"default":60},"limit":{"type":"integer","minimum":1,"maximum":500,"default":100}},"required":["matchers"],"additionalProperties":false}`),
	}
}

func (t *StreamSummaryTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input streamSummaryInput
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	if len(input.Matchers) == 0 {
		return nil, errors.New("matchers must contain at least one exact label matcher")
	}
	request := lokiapi.StreamSummaryRequest{Matchers: input.Matchers}
	if input.LookbackMinutes != nil {
		if *input.LookbackMinutes < 1 || *input.LookbackMinutes > 360 {
			return nil, errors.New("lookback_minutes must be between 1 and 360")
		}
		request.Lookback = *input.LookbackMinutes
	}
	if input.Limit != nil {
		if *input.Limit < 1 || *input.Limit > 500 {
			return nil, errors.New("limit must be between 1 and 500")
		}
		request.Limit = *input.Limit
	}
	result, err := t.client.StreamSummary(ctx, request)
	if err != nil {
		return nil, err
	}
	return encodeResult(result)
}
