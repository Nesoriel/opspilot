package observability

import (
	"context"
	"errors"

	"github.com/Nesoriel/opspilot/internal/agent"
)

type multiObserver struct {
	observers []agent.Observer
}

func Combine(observers ...agent.Observer) agent.Observer {
	filtered := make([]agent.Observer, 0, len(observers))
	for _, observer := range observers {
		if observer != nil {
			filtered = append(filtered, observer)
		}
	}
	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		return &multiObserver{observers: filtered}
	}
}

func (m *multiObserver) Observe(ctx context.Context, event agent.Event) {
	for _, observer := range m.observers {
		safelyObserve(observer, ctx, event)
	}
}

func safelyObserve(observer agent.Observer, ctx context.Context, event agent.Event) {
	defer func() {
		_ = recover()
	}()
	observer.Observe(ctx, event)
}

func errorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, agent.ErrToolNotFound):
		return "tool_not_found"
	case errors.Is(err, agent.ErrMaxStepsExceeded):
		return "max_steps_exceeded"
	default:
		return "operation_failed"
	}
}
