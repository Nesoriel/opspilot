package agent

import (
	"context"
	"testing"
)

type recordingObserver struct {
	events []Event
}

func (o *recordingObserver) Observe(_ context.Context, event Event) {
	o.events = append(o.events, event)
}

func TestRuntimeEmitsOrderedLifecycleEvents(t *testing.T) {
	observer := &recordingObserver{}
	model := &scriptedModel{responses: []ModelResponse{{Content: "healthy"}}}
	runtime, err := NewRuntime(model, NewRegistry(), WithObserver(observer))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if _, err := runtime.Run(context.Background(), []Message{{Role: RoleUser, Content: "status"}}); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := []EventType{EventRunStarted, EventModelStarted, EventModelFinished, EventRunFinished}
	if len(observer.events) != len(want) {
		t.Fatalf("unexpected event count: %#v", observer.events)
	}
	runID := observer.events[0].RunID
	if runID == "" {
		t.Fatal("run ID is empty")
	}
	for index, event := range observer.events {
		if event.Type != want[index] {
			t.Fatalf("event %d type = %s, want %s", index, event.Type, want[index])
		}
		if event.RunID != runID {
			t.Fatalf("event %d has mismatched run ID: %q", index, event.RunID)
		}
		if event.Timestamp.IsZero() {
			t.Fatalf("event %d has zero timestamp", index)
		}
	}
	if observer.events[2].Duration < 0 || observer.events[3].Duration < 0 {
		t.Fatal("finished event has a negative duration")
	}
}

func TestRuntimeRunFinishedIncludesFailure(t *testing.T) {
	observer := &recordingObserver{}
	model := &scriptedModel{}
	runtime, err := NewRuntime(model, NewRegistry(), WithObserver(observer))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if _, err := runtime.Run(context.Background(), nil); err == nil {
		t.Fatal("expected model failure")
	}
	last := observer.events[len(observer.events)-1]
	if last.Type != EventRunFinished || last.Err == nil {
		t.Fatalf("run failure was not emitted: %#v", last)
	}
}
