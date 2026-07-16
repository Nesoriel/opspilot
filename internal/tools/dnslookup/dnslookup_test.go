package dnslookup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeResolver struct{}

func (fakeResolver) LookupHost(context.Context, string) ([]string, error) {
	return []string{"2001:db8::1", "192.0.2.10"}, nil
}

func TestExecute(t *testing.T) {
	tool := New(fakeResolver{})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"host":"example.test"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(string(result), "192.0.2.10") {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestRejectsUnknownField(t *testing.T) {
	tool := New(fakeResolver{})
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"host":"example.test","extra":true}`)); err == nil {
		t.Fatal("expected unknown field error")
	}
}
