package ark

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

func toAgenticMessages(messages []agent.Message) ([]*schema.AgenticMessage, error) {
	converted := make([]*schema.AgenticMessage, 0, len(messages))
	for index, message := range messages {
		var result *schema.AgenticMessage

		switch message.Role {
		case agent.RoleSystem:
			result = schema.SystemAgenticMessage(message.Content)
		case agent.RoleUser:
			result = schema.UserAgenticMessage(message.Content)
		case agent.RoleAssistant:
			blocks := make([]*schema.ContentBlock, 0, len(message.ToolCalls)+1)
			if message.Content != "" {
				blocks = append(blocks, schema.NewContentBlock(&schema.AssistantGenText{Text: message.Content}))
			}
			for callIndex, call := range message.ToolCalls {
				if call.ID == "" {
					return nil, fmt.Errorf("assistant message %d tool call %d has no ID", index, callIndex)
				}
				if call.Name == "" {
					return nil, fmt.Errorf("assistant message %d tool call %d has no name", index, callIndex)
				}
				arguments := call.Arguments
				if len(arguments) == 0 {
					arguments = json.RawMessage(`{}`)
				}
				if !json.Valid(arguments) {
					return nil, fmt.Errorf("assistant message %d tool call %d has invalid JSON arguments", index, callIndex)
				}
				blocks = append(blocks, schema.NewContentBlock(&schema.FunctionToolCall{
					CallID:    call.ID,
					Name:      call.Name,
					Arguments: string(arguments),
				}))
			}
			if len(blocks) == 0 {
				return nil, fmt.Errorf("assistant message %d has no content", index)
			}
			result = &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: blocks}
		case agent.RoleTool:
			if message.ToolCallID == "" {
				return nil, fmt.Errorf("tool message %d has no tool call ID", index)
			}
			if message.ToolName == "" {
				return nil, fmt.Errorf("tool message %d has no tool name", index)
			}
			content := message.Content
			if content == "" {
				content = "null"
			}
			result = &schema.AgenticMessage{
				Role: schema.AgenticRoleTypeUser,
				ContentBlocks: []*schema.ContentBlock{
					schema.NewContentBlock(&schema.FunctionToolResult{
						CallID: message.ToolCallID,
						Name:   message.ToolName,
						Content: []*schema.FunctionToolResultContentBlock{
							{
								Type: schema.FunctionToolResultContentBlockTypeText,
								Text: &schema.UserInputText{Text: content},
							},
						},
					}),
				},
			}
		default:
			return nil, fmt.Errorf("message %d has unsupported role %q", index, message.Role)
		}

		converted = append(converted, result)
	}
	return converted, nil
}

func toToolInfos(definitions []agent.ToolDefinition) ([]*schema.ToolInfo, error) {
	tools := make([]*schema.ToolInfo, 0, len(definitions))
	for index, definition := range definitions {
		if definition.Name == "" {
			return nil, fmt.Errorf("tool definition %d has no name", index)
		}
		if definition.Description == "" {
			return nil, fmt.Errorf("tool definition %q has no description", definition.Name)
		}
		if len(definition.InputSchema) == 0 {
			return nil, fmt.Errorf("tool definition %q has no input schema", definition.Name)
		}

		var inputSchema jsonschema.Schema
		if err := json.Unmarshal(definition.InputSchema, &inputSchema); err != nil {
			return nil, fmt.Errorf("decode tool definition %q schema: %w", definition.Name, err)
		}
		tools = append(tools, &schema.ToolInfo{
			Name:        definition.Name,
			Desc:        definition.Description,
			ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&inputSchema),
		})
	}
	return tools, nil
}

func fromAgenticMessage(message *schema.AgenticMessage) (agent.ModelResponse, error) {
	if message == nil {
		return agent.ModelResponse{}, errors.New("Ark returned a nil message")
	}

	var text strings.Builder
	toolCalls := make([]agent.ToolCall, 0)
	for index, block := range message.ContentBlocks {
		if block == nil {
			continue
		}
		switch block.Type {
		case schema.ContentBlockTypeAssistantGenText:
			if block.AssistantGenText == nil {
				return agent.ModelResponse{}, fmt.Errorf("response block %d has no generated text", index)
			}
			text.WriteString(block.AssistantGenText.Text)
		case schema.ContentBlockTypeFunctionToolCall:
			if block.FunctionToolCall == nil {
				return agent.ModelResponse{}, fmt.Errorf("response block %d has no function tool call", index)
			}
			call := block.FunctionToolCall
			if call.CallID == "" || call.Name == "" {
				return agent.ModelResponse{}, fmt.Errorf("response block %d has an incomplete function tool call", index)
			}
			arguments := call.Arguments
			if arguments == "" {
				arguments = `{}`
			}
			if !json.Valid([]byte(arguments)) {
				return agent.ModelResponse{}, fmt.Errorf("response block %d has invalid tool arguments", index)
			}
			toolCalls = append(toolCalls, agent.ToolCall{
				ID:        call.CallID,
				Name:      call.Name,
				Arguments: append(json.RawMessage(nil), []byte(arguments)...),
			})
		}
	}

	if text.Len() == 0 && len(toolCalls) == 0 {
		return agent.ModelResponse{}, errors.New("Ark response contained no usable text or function tool calls")
	}
	return agent.ModelResponse{Content: text.String(), ToolCalls: toolCalls}, nil
}
