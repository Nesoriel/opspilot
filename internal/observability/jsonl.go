package observability

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type JSONLObserver struct {
	mu      sync.Mutex
	encoder *json.Encoder
}

type eventRecord struct {
	Type       agent.EventType `json:"type"`
	RunID      string          `json:"run_id"`
	Timestamp  time.Time       `json:"timestamp"`
	DurationMS int64           `json:"duration_ms,omitempty"`
	Step       int             `json:"step,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Status     string          `json:"status"`
	ErrorCode  string          `json:"error_code,omitempty"`
}

func NewJSONLObserver(writer io.Writer) *JSONLObserver {
	return &JSONLObserver{encoder: json.NewEncoder(writer)}
}

func (o *JSONLObserver) Observe(_ context.Context, event agent.Event) {
	if o == nil || o.encoder == nil {
		return
	}

	status := "started"
	if isFinishedEvent(event.Type) {
		status = "ok"
		if event.Err != nil {
			status = "error"
		}
	}
	record := eventRecord{
		Type:       event.Type,
		RunID:      event.RunID,
		Timestamp:  event.Timestamp,
		DurationMS: event.Duration.Milliseconds(),
		Step:       event.Step,
		ToolName:   event.ToolName,
		ToolCallID: event.ToolCallID,
		Status:     status,
		ErrorCode:  errorCode(event.Err),
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.encoder.Encode(record)
}

func isFinishedEvent(eventType agent.EventType) bool {
	switch eventType {
	case agent.EventRunFinished, agent.EventModelFinished, agent.EventToolFinished:
		return true
	default:
		return false
	}
}
