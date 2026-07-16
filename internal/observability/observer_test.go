package observability

import (
	"context"
	"testing"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type countObserver struct {
	count int
}

func (o *countObserver) Observe(context.Context, agent.Event) {
	o.count++
}

type panicObserver struct{}

func (panicObserver) Observe(context.Context, agent.Event) {
	panic("observer failure")
}

func TestCombineFansOutAndContainsPanics(t *testing.T) {
	first := &countObserver{}
	second := &countObserver{}
	observer := Combine(first, panicObserver{}, second)
	observer.Observe(context.Background(), agent.Event{Type: agent.EventRunStarted, RunID: "run-1"})

	if first.count != 1 || second.count != 1 {
		t.Fatalf("observers did not receive event: first=%d second=%d", first.count, second.count)
	}
}

func TestCombineDropsNilObservers(t *testing.T) {
	observer := Combine(nil, nil)
	if observer != nil {
		t.Fatalf("expected nil observer, got %T", observer)
	}
}
