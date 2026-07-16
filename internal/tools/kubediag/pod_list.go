package kubediag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type PodListTool struct {
	client Client
}

type podListInput struct {
	Namespace string `json:"namespace,omitempty"`
	Limit     *int   `json:"limit,omitempty"`
}

func NewPodList(client Client) *PodListTool {
	return &PodListTool{client: client}
}

func (t *PodListTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "kubernetes_pod_list",
		Description: "List a bounded read-only summary of Kubernetes Pods, readiness, phases, restart counts, node placement, and container totals in one namespace or all namespaces.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string","description":"Namespace name; omit to use the configured default, or use * for all namespaces"},"limit":{"type":"integer","minimum":1,"maximum":200,"default":100}},"additionalProperties":false}`),
	}
}

func (t *PodListTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input podListInput
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	input.Namespace = strings.TrimSpace(input.Namespace)
	if input.Namespace == "" {
		input.Namespace = t.client.DefaultNamespace()
	}
	if err := validateNamespace(input.Namespace, true); err != nil {
		return nil, err
	}
	limit := defaultPodLimit
	if input.Limit != nil {
		limit = *input.Limit
	}
	if limit < 1 || limit > maxPodLimit {
		return nil, errors.New("limit must be between 1 and 200")
	}
	result, err := t.client.PodList(ctx, input.Namespace, int64(limit))
	if err != nil {
		return nil, err
	}
	if len(result.Pods) > limit {
		result.Pods = result.Pods[:limit]
		result.Count = len(result.Pods)
		result.Truncated = true
	}
	return encodeResult(result)
}
