package ark

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/cloudwego/eino/schema"
)

func TestToAgenticMessagesPreservesToolCorrelation(t *testing.T) {
	messages, err := toAgenticMessages([]agent.Message{
		{Role: agent.RoleUser, Content: "check it"},
		{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "dns_lookup", Arguments: json.RawMessage(`{"host":"example.com"}`)},
			},
		},
		{Role: agent.RoleTool, ToolCallID: "call-1", ToolName: "dns_lookup", Content: `{"ok":true}`},
	})
	if err != nil {
		t.Fatalf("convert messages: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("unexpected message count: %d", len(messages))
	}
	call := messages[1].ContentBlocks[0].FunctionToolCall
	if call == nil || call.CallID != "call-1" || call.Name != "dns_lookup" {
		t.Fatalf("tool call was not preserved: %#v", call)
	}
	result := messages[2].ContentBlocks[0].FunctionToolResult
	if messages[2].Role != schema.AgenticRoleTypeUser || result == nil || result.CallID != "call-1" {
		t.Fatalf("tool result was not preserved: %#v", messages[2])
	}
}

func TestToToolInfosRejectsInvalidSchema(t *testing.T) {
	_, err := toToolInfos([]agent.ToolDefinition{
		{Name: "broken", Description: "broken", InputSchema: json.RawMessage(`{"type":`)},
	})
	if err == nil {
		t.Fatal("expected invalid schema error")
	}
}

func TestFromAgenticMessageConvertsTextAndToolCall(t *testing.T) {
	message := &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: "I will inspect it."}),
			schema.NewContentBlock(&schema.FunctionToolCall{
				CallID:    "call-7",
				Name:      "http_probe",
				Arguments: `{"url":"https://example.com"}`,
			}),
		},
	}
	response, err := fromAgenticMessage(message)
	if err != nil {
		t.Fatalf("convert response: %v", err)
	}
	if response.Content != "I will inspect it." || len(response.ToolCalls) != 1 {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.ToolCalls[0].ID != "call-7" || !strings.Contains(string(response.ToolCalls[0].Arguments), "example.com") {
		t.Fatalf("tool call was not preserved: %#v", response.ToolCalls[0])
	}
}

func TestFromAgenticMessageRejectsInvalidToolArguments(t *testing.T) {
	_, err := fromAgenticMessage(&schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.FunctionToolCall{CallID: "call-1", Name: "broken", Arguments: "{"}),
		},
	})
	if err == nil {
		t.Fatal("expected invalid tool arguments error")
	}
}
