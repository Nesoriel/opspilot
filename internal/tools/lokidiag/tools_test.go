package lokidiag

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Nesoriel/opspilot/internal/lokiapi"
)

type fakeClient struct {
	serverResult lokiapi.ServerInfo
	streamResult lokiapi.StreamSummary
	serverErr    error
	streamErr    error
	lastRequest  lokiapi.StreamSummaryRequest
}

func (f *fakeClient) ServerInfo(context.Context) (lokiapi.ServerInfo, error) {
	return f.serverResult, f.serverErr
}

func (f *fakeClient) StreamSummary(_ context.Context, request lokiapi.StreamSummaryRequest) (lokiapi.StreamSummary, error) {
	f.lastRequest = request
	return f.streamResult, f.streamErr
}

func TestLokiToolsExposeUniqueStrictSchemas(t *testing.T) {
	client := &fakeClient{}
	tools := []interface{ Definition() agent.ToolDefinition }{
		NewServerInfo(client),
		NewStreamSummary(client),
	}
	seen := map[string]struct{}{}
	for _, tool := range tools {
		definition := tool.Definition()
		if definition.Name == "" || definition.Description == "" || !json.Valid(definition.InputSchema) {
			t.Fatalf("invalid definition: %#v", definition)
		}
		if _, duplicate := seen[definition.Name]; duplicate {
			t.Fatalf("duplicate tool name: %s", definition.Name)
		}
		seen[definition.Name] = struct{}{}
	}
}

func TestServerInfoTool(t *testing.T) {
	client := &fakeClient{serverResult: lokiapi.ServerInfo{Ready: true, ReadyStatusCode: 200}}
	tool := NewServerInfo(client)
	payload, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !json.Valid(payload) {
		t.Fatalf("invalid result: %s", payload)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"extra":true}`)); err == nil {
		t.Fatal("unknown argument was accepted")
	}
}

func TestStreamSummaryToolDefaultsAndExplicitBounds(t *testing.T) {
	client := &fakeClient{streamResult: lokiapi.StreamSummary{Count: 1}}
	tool := NewStreamSummary(client)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"matchers":{"namespace":"operations"}}`))
	if err != nil {
		t.Fatalf("execute defaults: %v", err)
	}
	if client.lastRequest.Lookback != 0 || client.lastRequest.Limit != 0 || client.lastRequest.Matchers["namespace"] != "operations" {
		t.Fatalf("unexpected request: %#v", client.lastRequest)
	}

	_, err = tool.Execute(context.Background(), json.RawMessage(`{"matchers":{"job":"loki"},"lookback_minutes":30,"limit":20}`))
	if err != nil {
		t.Fatalf("execute explicit: %v", err)
	}
	if client.lastRequest.Lookback != 30 || client.lastRequest.Limit != 20 {
		t.Fatalf("unexpected explicit request: %#v", client.lastRequest)
	}

	for _, arguments := range []string{
		`{"matchers":{}}`,
		`{"matchers":{"job":"loki"},"lookback_minutes":0}`,
		`{"matchers":{"job":"loki"},"limit":0}`,
		`{"matchers":{"job":"loki"},"extra":true}`,
	} {
		if _, err := tool.Execute(context.Background(), json.RawMessage(arguments)); err == nil {
			t.Fatalf("invalid arguments accepted: %s", arguments)
		}
	}
}

func TestLokiToolsPropagateClientErrors(t *testing.T) {
	client := &fakeClient{
		serverErr: errors.New("loki_unreachable"),
		streamErr: errors.New("loki_query_failed"),
	}
	if _, err := NewServerInfo(client).Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("server error was not propagated")
	}
	if _, err := NewStreamSummary(client).Execute(context.Background(), json.RawMessage(`{"matchers":{"job":"loki"}}`)); err == nil {
		t.Fatal("stream error was not propagated")
	}
}
