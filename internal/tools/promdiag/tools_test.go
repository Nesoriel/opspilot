package promdiag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/promapi"
)

type fakeClient struct {
	serverInfo     promapi.ServerInfo
	targetList     promapi.TargetList
	metricSnapshot promapi.MetricSnapshot
	err            error
	targetLimit    int
	metricRequest  promapi.MetricSnapshotRequest
}

func (c *fakeClient) ServerInfo(context.Context) (promapi.ServerInfo, error) {
	return c.serverInfo, c.err
}

func (c *fakeClient) TargetList(_ context.Context, limit int) (promapi.TargetList, error) {
	c.targetLimit = limit
	return c.targetList, c.err
}

func (c *fakeClient) MetricSnapshot(_ context.Context, request promapi.MetricSnapshotRequest) (promapi.MetricSnapshot, error) {
	c.metricRequest = request
	return c.metricSnapshot, c.err
}

func TestServerInfoToolStrictArguments(t *testing.T) {
	client := &fakeClient{serverInfo: promapi.ServerInfo{Build: promapi.BuildInfo{Version: "3.10.0"}}}
	tool := NewServerInfo(client)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(string(result), `"version":"3.10.0"`) {
		t.Fatalf("unexpected result: %s", result)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"unexpected":true}`)); err == nil {
		t.Fatal("expected strict argument error")
	}
}

func TestTargetListToolDefaultsAndBounds(t *testing.T) {
	client := &fakeClient{}
	tool := NewTargetList(client)
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("execute defaults: %v", err)
	}
	if client.targetLimit != defaultTargetLimit {
		t.Fatalf("limit = %d", client.targetLimit)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"limit":5}`)); err != nil {
		t.Fatalf("execute explicit: %v", err)
	}
	if client.targetLimit != 5 {
		t.Fatalf("explicit limit = %d", client.targetLimit)
	}
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"limit":0}`),
		json.RawMessage(`{"limit":501}`),
		json.RawMessage(`{"unexpected":true}`),
		json.RawMessage(`{} {}`),
	} {
		if _, err := tool.Execute(context.Background(), arguments); err == nil {
			t.Fatalf("expected validation error for %s", arguments)
		}
	}
}

func TestMetricSnapshotToolNormalizesAndForwardsRequest(t *testing.T) {
	client := &fakeClient{metricSnapshot: promapi.MetricSnapshot{Metric: "up"}}
	tool := NewMetricSnapshot(client)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"metric":" up ",
		"matchers":{"job":"node"},
		"aggregation":"sum",
		"group_by":["job"],
		"limit":10
	}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.metricRequest.Metric != "up" || client.metricRequest.Limit != 10 || client.metricRequest.Aggregation != "sum" {
		t.Fatalf("unexpected request: %#v", client.metricRequest)
	}
	if !strings.Contains(string(result), `"metric":"up"`) {
		t.Fatalf("unexpected result: %s", result)
	}
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"metric":""}`),
		json.RawMessage(`{"metric":"up","limit":0}`),
		json.RawMessage(`{"metric":"up","limit":501}`),
		json.RawMessage(`{"metric":"up","unexpected":true}`),
	} {
		if _, err := tool.Execute(context.Background(), arguments); err == nil {
			t.Fatalf("expected validation error for %s", arguments)
		}
	}

	client.err = errors.New("prometheus_timeout: request timed out")
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"metric":"up"}`)); err == nil || !strings.Contains(err.Error(), "prometheus_timeout") {
		t.Fatalf("expected propagated error, got %v", err)
	}
}

func TestPrometheusToolDefinitionsAreValidAndDistinct(t *testing.T) {
	client := &fakeClient{}
	tools := []agent.Tool{
		NewServerInfo(client),
		NewTargetList(client),
		NewMetricSnapshot(client),
	}
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		definition := tool.Definition()
		if definition.Name == "" || definition.Description == "" || !json.Valid(definition.InputSchema) {
			t.Fatalf("invalid definition: %#v", definition)
		}
		if _, duplicate := seen[definition.Name]; duplicate {
			t.Fatalf("duplicate tool name %q", definition.Name)
		}
		seen[definition.Name] = struct{}{}
	}
}
