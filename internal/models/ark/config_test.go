package ark

import (
	"strings"
	"testing"
	"time"
)

func TestConfigFromLookupAPIKey(t *testing.T) {
	values := map[string]string{
		"ARK_API_KEY":             "test-key",
		"ARK_MODEL_ID":            "ep-test",
		"ARK_TIMEOUT":             "30s",
		"ARK_RETRY_TIMES":         "3",
		"ARK_THINKING":            "disabled",
		"ARK_MAX_TOOL_CALLS":      "6",
		"ARK_PARALLEL_TOOL_CALLS": "true",
	}
	config, err := configFromLookup(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("config from lookup: %v", err)
	}
	if config.Timeout != 30*time.Second || config.RetryTimes != 3 {
		t.Fatalf("unexpected retry or timeout: %#v", config)
	}
	if config.Thinking != ThinkingDisabled || config.MaxToolCalls != 6 {
		t.Fatalf("unexpected model options: %#v", config)
	}
	if config.ParallelToolCalls == nil || !*config.ParallelToolCalls {
		t.Fatal("parallel tool calls were not parsed")
	}
}

func TestConfigRequiresModel(t *testing.T) {
	_, err := configFromLookup(func(key string) string {
		if key == "ARK_API_KEY" {
			return "test-key"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "ARK_MODEL_ID") {
		t.Fatalf("expected missing model error, got %v", err)
	}
}

func TestConfigRejectsPartialAKSK(t *testing.T) {
	values := map[string]string{
		"ARK_MODEL_ID":  "ep-test",
		"ARK_ACCESS_KEY": "access",
	}
	_, err := configFromLookup(func(key string) string { return values[key] })
	if err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("expected partial credential error, got %v", err)
	}
}

func TestConfigRejectsInvalidThinkingMode(t *testing.T) {
	config := Config{
		APIKey:    "test-key",
		Model:     "ep-test",
		Timeout:   time.Second,
		Thinking:  "sometimes",
	}
	if err := config.Validate(); err == nil {
		t.Fatal("expected invalid thinking mode error")
	}
}
