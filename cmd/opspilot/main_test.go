package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunAgentRequiresPrompt(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(context.Background(), []string{"agent", "run"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("expected missing prompt error, got %v", err)
	}
}

func TestRunAgentRejectsMissingArkConfiguration(t *testing.T) {
	for _, key := range []string{
		"ARK_API_KEY",
		"ARK_ACCESS_KEY",
		"ARK_SECRET_KEY",
		"ARK_MODEL_ID",
		"ARK_BASE_URL",
		"ARK_REGION",
		"ARK_TIMEOUT",
		"ARK_RETRY_TIMES",
		"ARK_THINKING",
		"ARK_MAX_TOOL_CALLS",
		"ARK_PARALLEL_TOOL_CALLS",
	} {
		t.Setenv(key, "")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(context.Background(), []string{"agent", "run", "inspect", "example.com"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "ARK_MODEL_ID") {
		t.Fatalf("expected missing Ark configuration error, got %v", err)
	}
}
