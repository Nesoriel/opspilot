package kubediag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/kubeapi"
)

type fakeClient struct {
	defaultNamespace string
	cluster          kubeapi.ClusterInfo
	podList          kubeapi.PodList
	podInspect       kubeapi.PodInspect
	err              error
	nodeLimit        int64
	namespace        string
	podLimit         int64
	podName          string
	eventLimit       int64
}

func (c *fakeClient) DefaultNamespace() string {
	if c.defaultNamespace == "" {
		return "default"
	}
	return c.defaultNamespace
}

func (c *fakeClient) ClusterInfo(_ context.Context, limit int64) (kubeapi.ClusterInfo, error) {
	c.nodeLimit = limit
	return c.cluster, c.err
}

func (c *fakeClient) PodList(_ context.Context, namespace string, limit int64) (kubeapi.PodList, error) {
	c.namespace = namespace
	c.podLimit = limit
	return c.podList, c.err
}

func (c *fakeClient) PodInspect(_ context.Context, namespace, name string, eventLimit int64) (kubeapi.PodInspect, error) {
	c.namespace = namespace
	c.podName = name
	c.eventLimit = eventLimit
	return c.podInspect, c.err
}

func TestClusterInfoToolDefaultsAndBounds(t *testing.T) {
	nodes := make([]kubeapi.NodeSummary, 0, 105)
	for index := 0; index < 105; index++ {
		nodes = append(nodes, kubeapi.NodeSummary{Name: "node"})
	}
	client := &fakeClient{cluster: kubeapi.ClusterInfo{NodeCount: len(nodes), Nodes: nodes}}
	tool := NewClusterInfo(client)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.nodeLimit != defaultNodeLimit {
		t.Fatalf("node limit = %d", client.nodeLimit)
	}
	var decoded kubeapi.ClusterInfo
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if decoded.NodeCount != defaultNodeLimit || len(decoded.Nodes) != defaultNodeLimit || !decoded.Truncated {
		t.Fatalf("result was not bounded: %#v", decoded)
	}
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"node_limit":0}`),
		json.RawMessage(`{"node_limit":201}`),
		json.RawMessage(`{"unexpected":true}`),
		json.RawMessage(`{} {}`),
	} {
		if _, err := tool.Execute(context.Background(), arguments); err == nil {
			t.Fatalf("expected validation error for %s", arguments)
		}
	}
}

func TestPodListToolUsesDefaultAndAllNamespaces(t *testing.T) {
	client := &fakeClient{defaultNamespace: "operations", podList: kubeapi.PodList{Namespace: "operations"}}
	tool := NewPodList(client)
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("execute default: %v", err)
	}
	if client.namespace != "operations" || client.podLimit != defaultPodLimit {
		t.Fatalf("unexpected default request: namespace=%q limit=%d", client.namespace, client.podLimit)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"namespace":"*","limit":5}`)); err != nil {
		t.Fatalf("execute all namespaces: %v", err)
	}
	if client.namespace != "*" || client.podLimit != 5 {
		t.Fatalf("unexpected all-namespace request: namespace=%q limit=%d", client.namespace, client.podLimit)
	}
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"namespace":"Invalid_Name"}`),
		json.RawMessage(`{"limit":201}`),
		json.RawMessage(`{"unexpected":true}`),
	} {
		if _, err := tool.Execute(context.Background(), arguments); err == nil {
			t.Fatalf("expected validation error for %s", arguments)
		}
	}
}

func TestPodInspectToolUsesDefaultsAndRejectsUnsafeNames(t *testing.T) {
	client := &fakeClient{defaultNamespace: "operations", podInspect: kubeapi.PodInspect{Summary: kubeapi.PodSummary{Name: "web-0"}}}
	tool := NewPodInspect(client)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"pod":"web-0"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.namespace != "operations" || client.podName != "web-0" || client.eventLimit != defaultEventLimit {
		t.Fatalf("unexpected inspect request: %#v", client)
	}
	if !strings.Contains(string(result), `"name":"web-0"`) {
		t.Fatalf("unexpected result: %s", result)
	}
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"namespace":"*","pod":"web-0"}`),
		json.RawMessage(`{"pod":""}`),
		json.RawMessage(`{"pod":"Web_0"}`),
		json.RawMessage(`{"pod":"web-0","event_limit":101}`),
	} {
		if _, err := tool.Execute(context.Background(), arguments); err == nil {
			t.Fatalf("expected validation error for %s", arguments)
		}
	}

	client.err = errors.New("kubernetes_forbidden: access denied")
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"pod":"web-0"}`)); err == nil || !strings.Contains(err.Error(), "kubernetes_forbidden") {
		t.Fatalf("expected propagated error, got %v", err)
	}
}

func TestKubernetesToolDefinitionsAreValidAndDistinct(t *testing.T) {
	client := &fakeClient{}
	tools := []agent.Tool{
		NewClusterInfo(client),
		NewPodList(client),
		NewPodInspect(client),
	}
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		definition := tool.Definition()
		if definition.Name == "" || definition.Description == "" || !json.Valid(definition.InputSchema) {
			t.Fatalf("invalid definition: %#v", definition)
		}
		if _, found := seen[definition.Name]; found {
			t.Fatalf("duplicate definition name %q", definition.Name)
		}
		seen[definition.Name] = struct{}{}
	}
}
