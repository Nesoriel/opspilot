package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type scriptedModel struct {
	responses []ModelResponse
	calls     int
}

func (m *scriptedModel) Generate(context.Context, []Message, []ToolDefinition) (ModelResponse, error) {
	if m.calls >= len(m.responses) {
		return ModelResponse{}, errors.New("no scripted response")
	}
	response := m.responses[m.calls]
	m.calls++
	return response, nil
}

type errorTool struct{}

func (errorTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "fail", Description: "always fails", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (errorTool) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("boom")
}

type slowTool struct{}

func (slowTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "slow", Description: "waits for cancellation", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (slowTool) Execute(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRuntimeReturnsDirectResponse(t *testing.T) {
	model := &scriptedModel{responses: []ModelResponse{{Content: "healthy"}}}
	runtime, err := NewRuntime(model, NewRegistry())
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Run(context.Background(), []Message{{Role: RoleUser, Content: "status"}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Final != "healthy" || result.Steps != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRuntimeExecutesToolThenContinues(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(staticTool{name: "echo"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	model := &scriptedModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{ID: "call-1", Name: "echo", Arguments: json.RawMessage(`{}`)}}},
		{Content: "done"},
	}}
	runtime, err := NewRuntime(model, registry)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Run(context.Background(), []Message{{Role: RoleUser, Content: "run"}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Final != "done" || result.Steps != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Messages) != 4 || result.Messages[2].Role != RoleTool {
		t.Fatalf("tool result was not appended: %#v", result.Messages)
	}
	if !strings.Contains(result.Messages[2].Content, `"ok":true`) {
		t.Fatalf("unexpected tool envelope: %s", result.Messages[2].Content)
	}
}

func TestRuntimeReturnsUnknownToolToModel(t *testing.T) {
	model := &scriptedModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{ID: "call-1", Name: "missing", Arguments: json.RawMessage(`{}`)}}},
		{Content: "recovered"},
	}}
	runtime, err := NewRuntime(model, NewRegistry())
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(result.Messages[1].Content, "tool_not_found") {
		t.Fatalf("missing structured tool error: %s", result.Messages[1].Content)
	}
}

func TestRuntimeReturnsToolFailureToModel(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(errorTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	model := &scriptedModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{ID: "call-1", Name: "fail", Arguments: json.RawMessage(`{}`)}}},
		{Content: "handled"},
	}}
	runtime, err := NewRuntime(model, registry)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(result.Messages[1].Content, "tool_execution_failed") {
		t.Fatalf("missing structured execution error: %s", result.Messages[1].Content)
	}
}

func TestRuntimeStopsAtMaxSteps(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(staticTool{name: "echo"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	model := &scriptedModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{Name: "echo", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []ToolCall{{Name: "echo", Arguments: json.RawMessage(`{}`)}}},
	}}
	runtime, err := NewRuntime(model, registry, WithMaxSteps(2))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Run(context.Background(), nil)
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("expected max steps error, got %v", err)
	}
	if result.Steps != 2 {
		t.Fatalf("unexpected steps: %d", result.Steps)
	}
}

func TestRuntimeTimesOutTool(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(slowTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	model := &scriptedModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{ID: "call-1", Name: "slow", Arguments: json.RawMessage(`{}`)}}},
		{Content: "timed out safely"},
	}}
	runtime, err := NewRuntime(model, registry, WithToolTimeout(5*time.Millisecond))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(result.Messages[1].Content, "tool_timeout") {
		t.Fatalf("missing timeout result: %s", result.Messages[1].Content)
	}
}
