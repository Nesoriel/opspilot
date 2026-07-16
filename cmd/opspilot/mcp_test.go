package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunMCPRequiresStdioSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	err := runMCP(context.Background(), nil, &stderr)
	if err == nil || !strings.Contains(err.Error(), "stdio") {
		t.Fatalf("expected stdio command error, got %v", err)
	}
	if !strings.Contains(stderr.String(), "opspilot mcp stdio") {
		t.Fatalf("missing MCP usage: %s", stderr.String())
	}
}

func TestMCPCommandErrorsNeverWriteStdout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportCommandError([]string{"mcp", "stdio"}, &stdout, &stderr, errors.New("transport failed"))

	if stdout.Len() != 0 {
		t.Fatalf("MCP error polluted stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "transport failed") {
		t.Fatalf("MCP error missing from stderr: %q", stderr.String())
	}
}

func TestNonMCPCommandErrorsRemainStructuredJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportCommandError([]string{"tool"}, &stdout, &stderr, errors.New("failed"))

	if !strings.Contains(stdout.String(), `"ok": false`) || !strings.Contains(stdout.String(), `"error": "failed"`) {
		t.Fatalf("unexpected structured error: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}
