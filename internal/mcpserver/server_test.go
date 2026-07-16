package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoTool struct{}

type echoArguments struct {
	Message string `json:"message"`
}

func (echoTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "echo",
		Description: "echo a message as structured JSON",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"],"additionalProperties":false}`),
	}
}

func (echoTool) Execute(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var input echoArguments
	if err := decoder.Decode(&input); err != nil {
		return nil, err
	}
	if input.Message == "" {
		return nil, errors.New("message is required")
	}
	return json.Marshal(map[string]string{"message": input.Message})
}

type blockingTool struct{}

func (blockingTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "blocking",
		Description: "wait until the request is cancelled",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (blockingTool) Execute(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestServerDiscoveryAndToolExecution(t *testing.T) {
	registry := agent.NewRegistry()
	if err := registry.Register(echoTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	server, err := New(registry, Config{Version: "v0.1.0"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	clientSession, serverSession := connectTestSession(t, server)
	defer clientSession.Close()
	defer serverSession.Close()

	initialize := clientSession.InitializeResult()
	if initialize.ServerInfo.Name != "opspilot" || initialize.ServerInfo.Version != "v0.1.0" {
		t.Fatalf("unexpected server metadata: %#v", initialize.ServerInfo)
	}

	listed, err := clientSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", listed.Tools)
	}
	if listed.Tools[0].Annotations == nil || !listed.Tools[0].Annotations.ReadOnlyHint || !listed.Tools[0].Annotations.IdempotentHint {
		t.Fatalf("missing safety annotations: %#v", listed.Tools[0].Annotations)
	}

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"message": "hello"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(content.Text, `"message":"hello"`) {
		t.Fatalf("unexpected text content: %#v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["message"] != "hello" {
		t.Fatalf("unexpected structured content: %#v", result.StructuredContent)
	}
}

func TestServerReturnsToolErrorsWithoutProtocolFailure(t *testing.T) {
	registry := agent.NewRegistry()
	if err := registry.Register(echoTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	server, err := New(registry, Config{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	clientSession, serverSession := connectTestSession(t, server)
	defer clientSession.Close()
	defer serverSession.Close()

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"unexpected": true},
	})
	if err != nil {
		t.Fatalf("tool failure became protocol failure: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool error: %#v", result)
	}

	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "missing"})
	if err == nil {
		t.Fatal("expected unknown tool protocol error")
	}
}

func TestToolHandlerAppliesTimeout(t *testing.T) {
	handler := toolHandler(blockingTool{}, 5*time.Millisecond)
	result, err := handler(context.Background(), &mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected timeout tool error: %#v", result)
	}
	content := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(content.Text, "deadline exceeded") {
		t.Fatalf("unexpected timeout message: %q", content.Text)
	}
}

func TestServerRunIsCancellable(t *testing.T) {
	registry := agent.NewRegistry()
	server, err := New(registry, Config{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("server run error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}

func connectTestSession(t *testing.T, server *mcp.Server) (*mcp.ClientSession, *mcp.ServerSession) {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		serverSession.Close()
		t.Fatalf("connect client: %v", err)
	}
	return clientSession, serverSession
}
