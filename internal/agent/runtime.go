package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrMaxStepsExceeded = errors.New("agent exceeded maximum steps")

type EventType string

const (
	EventModelStarted  EventType = "model.started"
	EventModelFinished EventType = "model.finished"
	EventToolStarted   EventType = "tool.started"
	EventToolFinished  EventType = "tool.finished"
	EventRunFinished   EventType = "run.finished"
)

type Event struct {
	Type       EventType `json:"type"`
	Step       int       `json:"step,omitempty"`
	ToolName   string    `json:"tool_name,omitempty"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
	Err        error     `json:"-"`
}

type Observer interface {
	Observe(ctx context.Context, event Event)
}

type Runtime struct {
	model       Model
	registry    *Registry
	maxSteps    int
	toolTimeout time.Duration
	observer    Observer
}

type Option func(*Runtime)

func WithMaxSteps(maxSteps int) Option {
	return func(runtime *Runtime) {
		if maxSteps > 0 {
			runtime.maxSteps = maxSteps
		}
	}
}

func WithToolTimeout(timeout time.Duration) Option {
	return func(runtime *Runtime) {
		if timeout > 0 {
			runtime.toolTimeout = timeout
		}
	}
}

func WithObserver(observer Observer) Option {
	return func(runtime *Runtime) {
		runtime.observer = observer
	}
}

func NewRuntime(model Model, registry *Registry, options ...Option) (*Runtime, error) {
	if model == nil {
		return nil, errors.New("model is nil")
	}
	if registry == nil {
		return nil, errors.New("tool registry is nil")
	}

	runtime := &Runtime{
		model:       model,
		registry:    registry,
		maxSteps:    8,
		toolTimeout: 15 * time.Second,
	}
	for _, option := range options {
		option(runtime)
	}
	return runtime, nil
}

func (r *Runtime) Run(ctx context.Context, initial []Message) (RunResult, error) {
	messages := append([]Message(nil), initial...)
	definitions := r.registry.Definitions()

	for step := 1; step <= r.maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return RunResult{Messages: messages, Steps: step - 1}, err
		}

		r.observe(ctx, Event{Type: EventModelStarted, Step: step})
		response, err := r.model.Generate(ctx, append([]Message(nil), messages...), definitions)
		r.observe(ctx, Event{Type: EventModelFinished, Step: step, Err: err})
		if err != nil {
			return RunResult{Messages: messages, Steps: step}, fmt.Errorf("generate model response: %w", err)
		}

		messages = append(messages, Message{
			Role:      RoleAssistant,
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		if len(response.ToolCalls) == 0 {
			r.observe(ctx, Event{Type: EventRunFinished, Step: step})
			return RunResult{Final: response.Content, Messages: messages, Steps: step}, nil
		}

		for index, call := range response.ToolCalls {
			if call.ID == "" {
				call.ID = fmt.Sprintf("step-%d-call-%d", step, index+1)
			}
			messages = append(messages, r.executeTool(ctx, step, call))
		}
	}

	return RunResult{Messages: messages, Steps: r.maxSteps}, ErrMaxStepsExceeded
}

type toolEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error *toolError      `json:"error,omitempty"`
}

type toolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (r *Runtime) executeTool(ctx context.Context, step int, call ToolCall) Message {
	r.observe(ctx, Event{Type: EventToolStarted, Step: step, ToolName: call.Name, ToolCallID: call.ID})

	tool, found := r.registry.Get(call.Name)
	if !found {
		payload := marshalEnvelope(toolEnvelope{
			OK: false,
			Error: &toolError{
				Code:    "tool_not_found",
				Message: fmt.Sprintf("tool %q is not registered", call.Name),
			},
		})
		r.observe(ctx, Event{Type: EventToolFinished, Step: step, ToolName: call.Name, ToolCallID: call.ID, Err: ErrToolNotFound})
		return Message{Role: RoleTool, ToolCallID: call.ID, ToolName: call.Name, Content: payload}
	}

	toolCtx, cancel := context.WithTimeout(ctx, r.toolTimeout)
	defer cancel()

	data, err := tool.Execute(toolCtx, call.Arguments)
	if err != nil {
		code := "tool_execution_failed"
		if errors.Is(err, context.DeadlineExceeded) {
			code = "tool_timeout"
		}
		payload := marshalEnvelope(toolEnvelope{
			OK:    false,
			Error: &toolError{Code: code, Message: err.Error()},
		})
		r.observe(ctx, Event{Type: EventToolFinished, Step: step, ToolName: call.Name, ToolCallID: call.ID, Err: err})
		return Message{Role: RoleTool, ToolCallID: call.ID, ToolName: call.Name, Content: payload}
	}

	if len(data) == 0 {
		data = json.RawMessage(`null`)
	}
	payload := marshalEnvelope(toolEnvelope{OK: true, Data: data})
	r.observe(ctx, Event{Type: EventToolFinished, Step: step, ToolName: call.Name, ToolCallID: call.ID})
	return Message{Role: RoleTool, ToolCallID: call.ID, ToolName: call.Name, Content: payload}
}

func marshalEnvelope(envelope toolEnvelope) string {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return `{"ok":false,"error":{"code":"serialization_failed","message":"failed to serialize tool result"}}`
	}
	return string(payload)
}

func (r *Runtime) observe(ctx context.Context, event Event) {
	if r.observer != nil {
		r.observer.Observe(ctx, event)
	}
}
