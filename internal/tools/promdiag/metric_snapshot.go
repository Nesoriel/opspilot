package promdiag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/promapi"
)

const (
	defaultSeriesLimit = 100
	maxSeriesLimit     = 500
)

type MetricSnapshotTool struct {
	client Client
}

type metricSnapshotInput struct {
	Metric      string            `json:"metric"`
	Matchers    map[string]string `json:"matchers,omitempty"`
	Aggregation string            `json:"aggregation,omitempty"`
	GroupBy     []string          `json:"group_by,omitempty"`
	Limit       *int              `json:"limit,omitempty"`
}

func NewMetricSnapshot(client Client) *MetricSnapshotTool {
	return &MetricSnapshotTool{client: client}
}

func (t *MetricSnapshotTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "prometheus_metric_snapshot",
		Description: "Evaluate a constrained instant Prometheus metric selector with exact allowlisted label matchers and optional safe aggregation; arbitrary PromQL is not accepted.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"metric":{"type":"string","minLength":1,"maxLength":256,"description":"Prometheus metric name using safe ASCII syntax"},"matchers":{"type":"object","maxProperties":8,"additionalProperties":{"type":"string","maxLength":256},"description":"Exact-match diagnostic labels only"},"aggregation":{"type":"string","enum":["none","sum","avg","min","max","count"],"default":"none"},"group_by":{"type":"array","maxItems":5,"uniqueItems":true,"items":{"type":"string","enum":["job","instance","cluster","namespace","pod","container","node","service","endpoint"]}},"limit":{"type":"integer","minimum":1,"maximum":500,"default":100}},"required":["metric"],"additionalProperties":false}`),
	}
}

func (t *MetricSnapshotTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	var input metricSnapshotInput
	if err := decodeStrict(arguments, &input); err != nil {
		return nil, err
	}
	input.Metric = strings.TrimSpace(input.Metric)
	if input.Metric == "" {
		return nil, errors.New("metric is required")
	}
	limit := defaultSeriesLimit
	if input.Limit != nil {
		limit = *input.Limit
	}
	if limit < 1 || limit > maxSeriesLimit {
		return nil, errors.New("limit must be between 1 and 500")
	}
	result, err := t.client.MetricSnapshot(ctx, promapi.MetricSnapshotRequest{
		Metric:      input.Metric,
		Matchers:    input.Matchers,
		Aggregation: input.Aggregation,
		GroupBy:     input.GroupBy,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	return encodeResult(result)
}
