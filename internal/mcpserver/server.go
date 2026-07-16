package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultToolTimeout = 15 * time.Second

type Config struct {
	Version     string
	ToolTimeout time.Duration
	Logger      *slog.Logger
}

func New(registry *agent.Registry, config Config) (*mcp.Server, error) {
	if registry == nil {
		return nil, errors.New("tool registry is nil")
	}
	if config.Version == "" {
		config.Version = "dev"
	}
	if config.ToolTimeout <= 0 {
		config.ToolTimeout = defaultToolTimeout
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "opspilot",
			Title:   "OpsPilot",
			Version: config.Version,
		},
		&mcp.ServerOptions{
			Instructions: "Use these read-only tools to collect operational evidence. Preserve uncertainty and do not invent results.",
			Logger:       config.Logger,
			Capabilities: &mcp.ServerCapabilities{},
		},
	)

	for _, definition := range registry.Definitions() {
		tool, found := registry.Get(definition.Name)
		if !found {
			return nil, fmt.Errorf("tool %q disappeared from registry", definition.Name)
		}
		inputSchema, err := decodeInputSchema(definition)
		if err != nil {
			return nil, err
		}

		server.AddTool(
			&mcp.Tool{
				Name:        definition.Name,
				Title:       definition.Name,
				Description: definition.Description,
				InputSchema: inputSchema,
				Annotations: readOnlyAnnotations(),
			},
			toolHandler(tool, config.ToolTimeout),
		)
	}
	return server, nil
}

func decodeInputSchema(definition agent.ToolDefinition) (map[string]any, error) {
	var schema map[string]any
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		return nil, fmt.Errorf("decode input schema for tool %q: %w", definition.Name, err)
	}
	if schema == nil || schema["type"] != "object" {
		return nil, fmt.Errorf("tool %q input schema must have type object", definition.Name)
	}
	return schema, nil
}

func readOnlyAnnotations() *mcp.ToolAnnotations {
	destructive := false
	openWorld := true
	return &mcp.ToolAnnotations{
		DestructiveHint: &destructive,
		IdempotentHint:  true,
		OpenWorldHint:   &openWorld,
		ReadOnlyHint:    true,
	}
}

func toolHandler(tool agent.Tool, timeout time.Duration) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := json.RawMessage(`{}`)
		if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
			arguments = append(json.RawMessage(nil), request.Params.Arguments...)
		}

		toolCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		result, err := tool.Execute(toolCtx, arguments)
		if err != nil {
			return errorResult(err), nil
		}
		if len(result) == 0 {
			result = json.RawMessage(`null`)
		}
		if !json.Valid(result) {
			return errorResult(errors.New("tool returned invalid JSON")), nil
		}

		response := &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(result)}},
		}
		var structured map[string]any
		if err := json.Unmarshal(result, &structured); err == nil && structured != nil {
			response.StructuredContent = structured
		}
		return response, nil
	}
}

func errorResult(err error) *mcp.CallToolResult {
	message := "tool execution failed"
	if err != nil {
		message = err.Error()
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}
