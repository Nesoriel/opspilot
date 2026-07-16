package ark

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Nesoriel/opspilot/internal/agent"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeBackend struct {
	response *schema.AgenticMessage
	err      error
	input    []*schema.AgenticMessage
	options  []einomodel.Option
}

func (f *fakeBackend) Generate(_ context.Context, input []*schema.AgenticMessage, options ...einomodel.Option) (*schema.AgenticMessage, error) {
	f.input = input
	f.options = options
	return f.response, f.err
}

func TestModelGeneratePassesMessagesAndTools(t *testing.T) {
	backend := &fakeBackend{response: &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: "done"}),
		},
	}}
	model := newWithBackend(backend)

	response, err := model.Generate(context.Background(), []agent.Message{
		{Role: agent.RoleSystem, Content: "be careful"},
		{Role: agent.RoleUser, Content: "inspect example.com"},
	}, []agent.ToolDefinition{
		{
			Name:        "dns_lookup",
			Description: "resolve a hostname",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"host":{"type":"string"}},"required":["host"]}`),
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if response.Content != "done" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if len(backend.input) != 2 || backend.input[0].Role != schema.AgenticRoleTypeSystem {
		t.Fatalf("unexpected converted input: %#v", backend.input)
	}
	options := einomodel.GetCommonOptions(nil, backend.options...)
	if len(options.Tools) != 1 || options.Tools[0].Name != "dns_lookup" {
		t.Fatalf("tools were not passed to backend: %#v", options.Tools)
	}
}

func TestModelGenerateRedactsProviderSecrets(t *testing.T) {
	backend := &fakeBackend{err: errors.New("request rejected for key super-secret")}
	model := newWithBackend(backend, "super-secret")

	_, err := model.Generate(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hello"}}, nil)
	if err == nil {
		t.Fatal("expected provider error")
	}
	if strings.Contains(err.Error(), "super-secret") || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("provider error was not redacted: %v", err)
	}
}

func TestModelRejectsNilBackend(t *testing.T) {
	var model *Model
	_, err := model.Generate(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected uninitialized model error")
	}
}
