package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type staticTool struct {
	name string
}

func (t staticTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        t.name,
		Description: "test tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (t staticTool) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"value":1}`), nil
}

func TestRegistryRejectsDuplicateTool(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(staticTool{name: "echo"}); err != nil {
		t.Fatalf("register first tool: %v", err)
	}
	if err := registry.Register(staticTool{name: "echo"}); !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool, got %v", err)
	}
}

func TestRegistrySortsDefinitions(t *testing.T) {
	registry := NewRegistry()
	for _, name := range []string{"zeta", "alpha"} {
		if err := registry.Register(staticTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	definitions := registry.Definitions()
	if definitions[0].Name != "alpha" || definitions[1].Name != "zeta" {
		t.Fatalf("definitions are not sorted: %#v", definitions)
	}
}

func TestRegistryRejectsInvalidName(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(staticTool{name: "Invalid-Name"}); err == nil {
		t.Fatal("expected invalid name error")
	}
}
