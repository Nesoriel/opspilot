package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/observability"
)

const (
	eventModeNone  = "none"
	eventModeJSONL = "jsonl"
)

type agentRunOptions struct {
	prompt    string
	eventMode string
}

type runtimeObservability struct {
	observer agent.Observer
	shutdown func()
	warning  string
}

func parseAgentRunOptions(args []string, lookup func(string) string) (agentRunOptions, error) {
	eventMode := strings.ToLower(strings.TrimSpace(lookup("OPSPILOT_EVENTS")))
	if eventMode == "" {
		eventMode = eventModeNone
	}

	promptParts := make([]string, 0, len(args))
	for _, argument := range args {
		if strings.HasPrefix(argument, "--events=") {
			eventMode = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(argument, "--events=")))
			continue
		}
		promptParts = append(promptParts, argument)
	}
	if eventMode != eventModeNone && eventMode != eventModeJSONL {
		return agentRunOptions{}, fmt.Errorf("unsupported event mode %q", eventMode)
	}

	prompt := strings.TrimSpace(strings.Join(promptParts, " "))
	if prompt == "" {
		return agentRunOptions{}, fmt.Errorf("agent prompt is required")
	}
	return agentRunOptions{prompt: prompt, eventMode: eventMode}, nil
}

func setupRuntimeObservability(ctx context.Context, eventMode string, eventWriter io.Writer, lookup func(string) string) runtimeObservability {
	observers := make([]agent.Observer, 0, 2)
	if eventMode == eventModeJSONL {
		observers = append(observers, observability.NewJSONLObserver(eventWriter))
	}

	enabled, err := strconv.ParseBool(strings.TrimSpace(lookup("OPSPILOT_OTEL_ENABLED")))
	if err != nil && strings.TrimSpace(lookup("OPSPILOT_OTEL_ENABLED")) != "" {
		return runtimeObservability{
			observer: observability.Combine(observers...),
			shutdown: func() {},
			warning:  "invalid_otlp_enabled_value",
		}
	}
	if !enabled {
		return runtimeObservability{
			observer: observability.Combine(observers...),
			shutdown: func() {},
		}
	}

	serviceName := strings.TrimSpace(lookup("OTEL_SERVICE_NAME"))
	telemetry, err := observability.NewOTLPTelemetry(ctx, serviceName)
	if err != nil {
		return runtimeObservability{
			observer: observability.Combine(observers...),
			shutdown: func() {},
			warning:  "otlp_initialization_failed",
		}
	}
	observers = append(observers, telemetry.Observer())
	return runtimeObservability{
		observer: observability.Combine(observers...),
		shutdown: func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = telemetry.Shutdown(shutdownCtx)
		},
	}
}

func emitObservabilityWarning(writer io.Writer, eventMode, code string) {
	if code == "" {
		return
	}
	if eventMode == eventModeJSONL {
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"type":       "telemetry.warning",
			"status":     "error",
			"error_code": code,
		})
		return
	}
	fmt.Fprintf(writer, "warning: observability disabled (%s)\n", code)
}

func environmentLookup(key string) string {
	return os.Getenv(key)
}
