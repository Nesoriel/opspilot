package kubediag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type PodInspectTool struct {
	client Client
}

type podInspectInput struct {
	Namespace  string `json:"namespace,omitempty"`
	Pod        string `json:"pod"`
	EventLimit *int   `json:"event_limit,omitempty"`
}

func NewPodInspect(client Client) *PodInspectTool {
	return &PodInspectTool{client: client}
}

func (t *PodInspectTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "kubernetes_pod_inspect",
		Description: "Inspect one Kubernetes Pod through a redacted read-only projection of conditions, container resources and status, ownership, and aggregated event reasons without logs or free-text messages.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string","description":"Namespace name; omit to use the configured default"},"pod":{"type":"string","minLength":1,"maxLength":253,"description":"Pod name"},"event_limit":{"type":"integer","minimum":1,"maximum":100,"default":50}},"required":["pod"],"additionalProperties":false}`),
	}
}

func (t *PodInspectTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input podInspectInput
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	input.Namespace = strings.TrimSpace(input.Namespace)
	input.Pod = strings.TrimSpace(input.Pod)
	if input.Namespace == "" {
		input.Namespace = t.client.DefaultNamespace()
	}
	if err := validateNamespace(input.Namespace, false); err != nil {
		return nil, err
	}
	if err := validatePodName(input.Pod); err != nil {
		return nil, err
	}
	eventLimit := defaultEventLimit
	if input.EventLimit != nil {
		eventLimit = *input.EventLimit
	}
	if eventLimit < 1 || eventLimit > maxEventLimit {
		return nil, errors.New("event_limit must be between 1 and 100")
	}
	result, err := t.client.PodInspect(ctx, input.Namespace, input.Pod, int64(eventLimit))
	if err != nil {
		return nil, err
	}
	return encodeResult(result)
}
