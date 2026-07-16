package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
)

func TestJSONLObserverEmitsSafeStructuredEvent(t *testing.T) {
	var output bytes.Buffer
	observer := NewJSONLObserver(&output)
	observer.Observe(context.Background(), agent.Event{
		Type:       agent.EventToolFinished,
		RunID:      "run-1",
		Timestamp:  time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC),
		Duration:   25 * time.Millisecond,
		Step:       2,
		ToolName:   "http_probe",
		ToolCallID: "call-1",
		Err:        errors.New("super-secret prompt and arguments"),
	})

	if strings.Contains(output.String(), "super-secret") || strings.Contains(output.String(), "prompt") {
		t.Fatalf("sensitive error content leaked: %s", output.String())
	}
	var record eventRecord
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode JSONL event: %v", err)
	}
	if record.Status != "error" || record.ErrorCode != "operation_failed" {
		t.Fatalf("unexpected status: %#v", record)
	}
	if record.DurationMS != 25 || record.ToolName != "http_probe" {
		t.Fatalf("unexpected event fields: %#v", record)
	}
}

func TestJSONLObserverMarksStartEvents(t *testing.T) {
	var output bytes.Buffer
	observer := NewJSONLObserver(&output)
	observer.Observe(context.Background(), agent.Event{
		Type:      agent.EventRunStarted,
		RunID:     "run-1",
		Timestamp: time.Now().UTC(),
	})

	var record eventRecord
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode JSONL event: %v", err)
	}
	if record.Status != "started" || record.ErrorCode != "" {
		t.Fatalf("unexpected start record: %#v", record)
	}
}
