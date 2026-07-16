package agent

import (
	"context"
	"testing"
)

type panickingObserver struct{}

func (panickingObserver) Observe(context.Context, Event) {
	panic("observer failure")
}

func TestRuntimeContainsObserverPanics(t *testing.T) {
	model := &scriptedModel{responses: []ModelResponse{{Content: "healthy"}}}
	runtime, err := NewRuntime(model, NewRegistry(), WithObserver(panickingObserver{}))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Run(context.Background(), []Message{{Role: RoleUser, Content: "status"}})
	if err != nil {
		t.Fatalf("observer panic interrupted run: %v", err)
	}
	if result.Final != "healthy" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
