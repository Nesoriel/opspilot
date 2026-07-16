package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
)

func TestParseAgentRunOptionsFlagOverridesEnvironment(t *testing.T) {
	options, err := parseAgentRunOptions(
		[]string{"--events=jsonl", "inspect", "example.com"},
		func(key string) string {
			if key == "OPSPILOT_EVENTS" {
				return "none"
			}
			return ""
		},
	)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if options.eventMode != eventModeJSONL || options.prompt != "inspect example.com" {
		t.Fatalf("unexpected options: %#v", options)
	}
}

func TestParseAgentRunOptionsRejectsUnsupportedMode(t *testing.T) {
	_, err := parseAgentRunOptions([]string{"--events=xml", "inspect"}, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported mode error, got %v", err)
	}
}

func TestSetupRuntimeObservabilityWritesEventsSeparately(t *testing.T) {
	var events bytes.Buffer
	setup := setupRuntimeObservability(
		context.Background(),
		eventModeJSONL,
		&events,
		func(string) string { return "" },
	)
	defer setup.shutdown()
	if setup.observer == nil {
		t.Fatal("expected JSONL observer")
	}

	setup.observer.Observe(context.Background(), agent.Event{
		Type:      agent.EventRunStarted,
		RunID:     "run-1",
		Timestamp: time.Now().UTC(),
	})
	if !strings.Contains(events.String(), `"type":"run.started"`) {
		t.Fatalf("event stream missing run event: %s", events.String())
	}
}

func TestInvalidOTelConfigurationDoesNotFailSetup(t *testing.T) {
	setup := setupRuntimeObservability(
		context.Background(),
		eventModeNone,
		&bytes.Buffer{},
		func(key string) string {
			if key == "OPSPILOT_OTEL_ENABLED" {
				return "not-a-bool"
			}
			return ""
		},
	)
	defer setup.shutdown()
	if setup.warning != "invalid_otlp_enabled_value" {
		t.Fatalf("unexpected warning: %q", setup.warning)
	}
}

func TestEmitObservabilityWarningUsesJSONLWhenRequested(t *testing.T) {
	var output bytes.Buffer
	emitObservabilityWarning(&output, eventModeJSONL, "otlp_initialization_failed")
	if !strings.Contains(output.String(), `"type":"telemetry.warning"`) || strings.Contains(output.String(), "http") {
		t.Fatalf("unexpected warning output: %s", output.String())
	}
}
