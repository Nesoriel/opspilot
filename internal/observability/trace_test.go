package observability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceObserverCreatesParentedSpansWithoutSensitiveErrors(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer provider.Shutdown(context.Background())

	observer := NewTraceObserver(provider.Tracer("test"))
	started := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	observer.Observe(context.Background(), agent.Event{Type: agent.EventRunStarted, RunID: "run-1", Timestamp: started})
	observer.Observe(context.Background(), agent.Event{Type: agent.EventModelStarted, RunID: "run-1", Step: 1, Timestamp: started.Add(time.Millisecond)})
	observer.Observe(context.Background(), agent.Event{
		Type:      agent.EventModelFinished,
		RunID:     "run-1",
		Step:      1,
		Timestamp: started.Add(3 * time.Millisecond),
		Duration:  2 * time.Millisecond,
	})
	observer.Observe(context.Background(), agent.Event{
		Type:       agent.EventToolStarted,
		RunID:      "run-1",
		Step:       1,
		ToolName:   "dns_lookup",
		ToolCallID: "call-1",
		Timestamp:  started.Add(4 * time.Millisecond),
	})
	observer.Observe(context.Background(), agent.Event{
		Type:       agent.EventToolFinished,
		RunID:      "run-1",
		Step:       1,
		ToolName:   "dns_lookup",
		ToolCallID: "call-1",
		Timestamp:  started.Add(6 * time.Millisecond),
		Duration:   2 * time.Millisecond,
		Err:        errors.New("super-secret tool arguments"),
	})
	observer.Observe(context.Background(), agent.Event{
		Type:      agent.EventRunFinished,
		RunID:     "run-1",
		Step:      1,
		Timestamp: started.Add(7 * time.Millisecond),
		Duration:  7 * time.Millisecond,
		Err:       errors.New("super-secret final prompt"),
	})

	spans := recorder.Ended()
	if len(spans) != 3 {
		t.Fatalf("ended spans = %d, want 3", len(spans))
	}
	byName := make(map[string]sdktrace.ReadOnlySpan, len(spans))
	for _, span := range spans {
		byName[span.Name()] = span
		for _, attr := range span.Attributes() {
			value := fmt.Sprint(attr.Value.AsInterface())
			if strings.Contains(value, "super-secret") || strings.Contains(value, "prompt") || strings.Contains(value, "arguments") {
				t.Fatalf("sensitive value leaked into span %s: %s=%s", span.Name(), attr.Key, value)
			}
		}
	}

	runSpan := byName["opspilot.agent.run"]
	modelSpan := byName["opspilot.model.generate"]
	toolSpan := byName["opspilot.tool.execute"]
	if runSpan == nil || modelSpan == nil || toolSpan == nil {
		t.Fatalf("missing expected spans: %#v", byName)
	}
	if modelSpan.Parent().SpanID() != runSpan.SpanContext().SpanID() || toolSpan.Parent().SpanID() != runSpan.SpanContext().SpanID() {
		t.Fatal("model or tool span is not parented to the run span")
	}
	if toolSpan.Status().Code != codes.Error || runSpan.Status().Code != codes.Error {
		t.Fatal("failed events did not set error span status")
	}
	if value := spanAttribute(toolSpan, "error.type"); value != "operation_failed" {
		t.Fatalf("tool error type = %q", value)
	}
}

func spanAttribute(span sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return fmt.Sprint(attr.Value.AsInterface())
		}
	}
	return ""
}
