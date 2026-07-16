package observability

import (
	"context"
	"sync"

	"github.com/Nesoriel/opspilot/internal/agent"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type TraceObserver struct {
	mu         sync.Mutex
	tracer     trace.Tracer
	runs       map[string]spanState
	models     map[modelSpanKey]trace.Span
	tools      map[toolSpanKey]trace.Span
}

type spanState struct {
	ctx  context.Context
	span trace.Span
}

type modelSpanKey struct {
	runID string
	step  int
}

type toolSpanKey struct {
	runID string
	step  int
	callID string
}

func NewTraceObserver(tracer trace.Tracer) *TraceObserver {
	return &TraceObserver{
		tracer: tracer,
		runs:   make(map[string]spanState),
		models: make(map[modelSpanKey]trace.Span),
		tools:  make(map[toolSpanKey]trace.Span),
	}
}

func (o *TraceObserver) Observe(ctx context.Context, event agent.Event) {
	if o == nil || o.tracer == nil || event.RunID == "" {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	switch event.Type {
	case agent.EventRunStarted:
		runCtx, span := o.tracer.Start(
			ctx,
			"opspilot.agent.run",
			trace.WithTimestamp(event.Timestamp),
			trace.WithAttributes(attribute.String("opspilot.run.id", event.RunID)),
		)
		o.runs[event.RunID] = spanState{ctx: runCtx, span: span}
	case agent.EventModelStarted:
		parent := o.parentContext(ctx, event.RunID)
		_, span := o.tracer.Start(
			parent,
			"opspilot.model.generate",
			trace.WithTimestamp(event.Timestamp),
			trace.WithAttributes(
				attribute.String("opspilot.run.id", event.RunID),
				attribute.Int("opspilot.step", event.Step),
			),
		)
		o.models[modelSpanKey{runID: event.RunID, step: event.Step}] = span
	case agent.EventModelFinished:
		key := modelSpanKey{runID: event.RunID, step: event.Step}
		if span, exists := o.models[key]; exists {
			finishSpan(span, event)
			delete(o.models, key)
		}
	case agent.EventToolStarted:
		parent := o.parentContext(ctx, event.RunID)
		_, span := o.tracer.Start(
			parent,
			"opspilot.tool.execute",
			trace.WithTimestamp(event.Timestamp),
			trace.WithAttributes(
				attribute.String("opspilot.run.id", event.RunID),
				attribute.Int("opspilot.step", event.Step),
				attribute.String("opspilot.tool.name", event.ToolName),
				attribute.String("opspilot.tool.call_id", event.ToolCallID),
			),
		)
		o.tools[toolSpanKey{runID: event.RunID, step: event.Step, callID: event.ToolCallID}] = span
	case agent.EventToolFinished:
		key := toolSpanKey{runID: event.RunID, step: event.Step, callID: event.ToolCallID}
		if span, exists := o.tools[key]; exists {
			finishSpan(span, event)
			delete(o.tools, key)
		}
	case agent.EventRunFinished:
		if state, exists := o.runs[event.RunID]; exists {
			finishSpan(state.span, event)
			delete(o.runs, event.RunID)
		}
		o.cleanupRun(event.RunID, event.Timestamp)
	}
}

func (o *TraceObserver) parentContext(fallback context.Context, runID string) context.Context {
	if state, exists := o.runs[runID]; exists {
		return state.ctx
	}
	return fallback
}

func (o *TraceObserver) cleanupRun(runID string, timestamp time.Time) {
	for key, span := range o.models {
		if key.runID == runID {
			span.End(trace.WithTimestamp(timestamp))
			delete(o.models, key)
		}
	}
	for key, span := range o.tools {
		if key.runID == runID {
			span.End(trace.WithTimestamp(timestamp))
			delete(o.tools, key)
		}
	}
}

func finishSpan(span trace.Span, event agent.Event) {
	span.SetAttributes(attribute.Int64("opspilot.duration_ms", event.Duration.Milliseconds()))
	if code := errorCode(event.Err); code != "" {
		span.SetAttributes(attribute.String("error.type", code))
		span.SetStatus(codes.Error, code)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End(trace.WithTimestamp(event.Timestamp))
}
