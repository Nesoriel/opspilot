package observability

import (
	"context"
	"errors"

	"github.com/Nesoriel/opspilot/internal/agent"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const instrumentationName = "github.com/Nesoriel/opspilot"

type Telemetry struct {
	provider *sdktrace.TracerProvider
}

func NewOTLPTelemetry(ctx context.Context, serviceName string) (*Telemetry, error) {
	if serviceName == "" {
		serviceName = "opspilot"
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	serviceResource := resource.NewWithAttributes(
		"",
		attribute.String("service.name", serviceName),
	)
	mergedResource, err := resource.Merge(resource.Default(), serviceResource)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(mergedResource),
	)
	return &Telemetry{provider: provider}, nil
}

func (t *Telemetry) Observer() agent.Observer {
	if t == nil || t.provider == nil {
		return nil
	}
	return NewTraceObserver(t.provider.Tracer(instrumentationName))
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || t.provider == nil {
		return nil
	}
	if err := t.provider.Shutdown(ctx); err != nil {
		return errors.New("shut down OpenTelemetry provider")
	}
	return nil
}
