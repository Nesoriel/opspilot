package dockerapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEngineInfoReportsWarningCountWithoutWarningText(t *testing.T) {
	info := mapEngineInfo(Version{APIVersion: "1.55"}, rawInfo{
		Warnings: []string{
			"mount path /host/private should not be returned",
			"proxy credential user:password should not be returned",
		},
	})
	if info.WarningCount != 2 {
		t.Fatalf("warning count = %d, want 2", info.WarningCount)
	}
	payload, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	for _, secret := range []string{"/host/private", "user:password"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("warning text leaked into output: %s", payload)
		}
	}
}

func TestContainerInspectReportsErrorPresenceWithoutErrorText(t *testing.T) {
	raw := rawContainerInspect{}
	raw.State = &struct {
		Status     string
		Running    bool
		Paused     bool
		Restarting bool
		OOMKilled  bool
		Dead       bool
		Pid        int
		ExitCode   int
		Error      string
		StartedAt  string
		FinishedAt string
		Health     *rawHealthSummary
	}{
		Status: "dead",
		Error:  "OCI runtime failed while mounting /host/private/data with token=secret",
	}
	result := mapContainerInspect(raw)
	if !result.State.ErrorPresent {
		t.Fatal("expected error_present to be true")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	for _, secret := range []string{"/host/private/data", "token=secret", "OCI runtime failed"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("state error text leaked into output: %s", payload)
		}
	}
}
