package kubediag

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type ClusterInfoTool struct {
	client Client
}

type clusterInfoInput struct {
	NodeLimit *int `json:"node_limit,omitempty"`
}

func NewClusterInfo(client Client) *ClusterInfoTool {
	return &ClusterInfoTool{client: client}
}

func (t *ClusterInfoTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "kubernetes_cluster_info",
		Description: "Inspect the configured Kubernetes server version and a bounded read-only summary of node readiness, capacity, runtime, and conditions.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"node_limit":{"type":"integer","minimum":1,"maximum":200,"default":100}},"additionalProperties":false}`),
	}
}

func (t *ClusterInfoTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input clusterInfoInput
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	nodeLimit := defaultNodeLimit
	if input.NodeLimit != nil {
		nodeLimit = *input.NodeLimit
	}
	if nodeLimit < 1 || nodeLimit > maxNodeLimit {
		return nil, errors.New("node_limit must be between 1 and 200")
	}
	result, err := t.client.ClusterInfo(ctx, int64(nodeLimit))
	if err != nil {
		return nil, err
	}
	if len(result.Nodes) > nodeLimit {
		result.Nodes = result.Nodes[:nodeLimit]
		result.NodeCount = len(result.Nodes)
		result.Truncated = true
	}
	return encodeResult(result)
}
