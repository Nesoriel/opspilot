package dockerdiag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Nesoriel/opspilot/internal/dockerapi"
)

type fakeClient struct {
	engineInfo        dockerapi.EngineInfo
	containers        []dockerapi.ContainerSummary
	inspect           dockerapi.ContainerInspect
	err               error
	listOptions       dockerapi.ContainerListOptions
	inspectIdentifier string
}

func (c *fakeClient) EngineInfo(context.Context) (dockerapi.EngineInfo, error) {
	return c.engineInfo, c.err
}

func (c *fakeClient) ContainerList(_ context.Context, options dockerapi.ContainerListOptions) ([]dockerapi.ContainerSummary, error) {
	c.listOptions = options
	return append([]dockerapi.ContainerSummary(nil), c.containers...), c.err
}

func (c *fakeClient) ContainerInspect(_ context.Context, identifier string) (dockerapi.ContainerInspect, error) {
	c.inspectIdentifier = identifier
	return c.inspect, c.err
}

func TestEngineInfoToolStrictArgumentsAndResult(t *testing.T) {
	client := &fakeClient{engineInfo: dockerapi.EngineInfo{StorageDriver: "overlay2"}}
	tool := NewEngineInfo(client)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(string(result), `"storage_driver":"overlay2"`) {
		t.Fatalf("unexpected result: %s", result)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"unexpected":true}`)); err == nil {
		t.Fatal("expected unknown argument error")
	}
}

func TestContainerListToolDefaultsAndBounds(t *testing.T) {
	containers := make([]dockerapi.ContainerSummary, 0, 105)
	for index := 0; index < 105; index++ {
		containers = append(containers, dockerapi.ContainerSummary{ID: string(rune('a' + index%26))})
	}
	client := &fakeClient{containers: containers}
	tool := NewContainerList(client)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !client.listOptions.All || client.listOptions.Limit != defaultContainerLimit {
		t.Fatalf("unexpected defaults: %#v", client.listOptions)
	}
	var decoded containerListOutput
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if decoded.Count != defaultContainerLimit || len(decoded.Containers) != defaultContainerLimit {
		t.Fatalf("tool did not enforce result limit: %#v", decoded)
	}

	all := false
	client.containers = nil
	arguments, _ := json.Marshal(containerListInput{All: &all, Limit: 5})
	if _, err := tool.Execute(context.Background(), arguments); err != nil {
		t.Fatalf("execute custom options: %v", err)
	}
	if client.listOptions.All || client.listOptions.Limit != 5 {
		t.Fatalf("unexpected custom options: %#v", client.listOptions)
	}

	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"limit":201}`),
		json.RawMessage(`{"limit":-1}`),
		json.RawMessage(`{"unexpected":true}`),
		json.RawMessage(`{} {}`),
	} {
		if _, err := tool.Execute(context.Background(), arguments); err == nil {
			t.Fatalf("expected validation error for %s", arguments)
		}
	}
}

func TestContainerInspectToolTrimsIdentifierAndPropagatesErrors(t *testing.T) {
	client := &fakeClient{inspect: dockerapi.ContainerInspect{ID: "abc", Name: "web"}}
	tool := NewContainerInspect(client)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"container":" web "}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.inspectIdentifier != "web" || !strings.Contains(string(result), `"name":"web"`) {
		t.Fatalf("unexpected inspect call/result: %q %s", client.inspectIdentifier, result)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"container":""}`)); err == nil {
		t.Fatal("expected missing container error")
	}

	client.err = errors.New("docker_timeout: timed out")
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"container":"web"}`)); err == nil || !strings.Contains(err.Error(), "docker_timeout") {
		t.Fatalf("expected propagated client error, got %v", err)
	}
}

func TestToolDefinitionsRemainReadOnlySchemas(t *testing.T) {
	client := &fakeClient{}
	for _, tool := range []interface{ Definition() struct {
		Name        string
		Description string
		InputSchema json.RawMessage
	} }{} {
		_ = tool
	}
	definitions := []json.RawMessage{
		NewEngineInfo(client).Definition().InputSchema,
		NewContainerList(client).Definition().InputSchema,
		NewContainerInspect(client).Definition().InputSchema,
	}
	for _, schema := range definitions {
		if !json.Valid(schema) || !strings.Contains(string(schema), `"type":"object"`) {
			t.Fatalf("invalid tool schema: %s", schema)
		}
	}
}
