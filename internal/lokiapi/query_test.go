package lokiapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildStreamSelectorGeneratesDeterministicExactSelector(t *testing.T) {
	selector, lookback, limit, err := buildStreamSelector(StreamSummaryRequest{
		Matchers: map[string]string{"pod": `web"one`, "namespace": "operations"},
		Lookback: 30,
		Limit:    25,
	})
	if err != nil {
		t.Fatalf("build selector: %v", err)
	}
	want := `{namespace="operations",pod="web\"one"}`
	if selector != want || lookback != 30*time.Minute || limit != 25 {
		t.Fatalf("selector=%q lookback=%v limit=%d", selector, lookback, limit)
	}

	selector, lookback, limit, err = buildStreamSelector(StreamSummaryRequest{Matchers: map[string]string{"job": "loki"}})
	if err != nil {
		t.Fatalf("build defaults: %v", err)
	}
	if selector != `{job="loki"}` || lookback != time.Hour || limit != defaultStreamLimit {
		t.Fatalf("unexpected defaults: %q %v %d", selector, lookback, limit)
	}
}

func TestBuildStreamSelectorRejectsUnsafeInputs(t *testing.T) {
	tooMany := map[string]string{
		"job": "x", "app": "x", "application": "x", "service": "x", "service_name": "x",
		"namespace": "x", "pod": "x", "container": "x", "cluster": "x",
	}
	for _, request := range []StreamSummaryRequest{
		{},
		{Matchers: map[string]string{}},
		{Matchers: map[string]string{"custom": "value"}},
		{Matchers: map[string]string{"job": ""}},
		{Matchers: map[string]string{"job": "line\nbreak"}},
		{Matchers: tooMany},
		{Matchers: map[string]string{"job": "loki"}, Lookback: 361},
		{Matchers: map[string]string{"job": "loki"}, Lookback: -1},
		{Matchers: map[string]string{"job": "loki"}, Limit: 501},
		{Matchers: map[string]string{"job": "loki"}, Limit: -1},
	} {
		if _, _, _, err := buildStreamSelector(request); err == nil {
			t.Fatalf("unsafe request accepted: %#v", request)
		}
	}
}

func TestMapStreamsFiltersSortsAndTruncates(t *testing.T) {
	streams, truncated := mapStreams([]map[string]string{
		{"namespace": "operations", "pod": "b", "filename": "/private/b", "custom": "hidden-b"},
		{"namespace": "operations", "pod": "a", "container": "web", "internal": "hidden-a"},
	}, diagnosticLabels, 1)
	if len(streams) != 1 || !truncated || streams[0].Labels["pod"] != "a" {
		t.Fatalf("unexpected streams: %#v truncated=%v", streams, truncated)
	}
	payload, err := json.Marshal(streams)
	if err != nil {
		t.Fatalf("marshal streams: %v", err)
	}
	for _, forbidden := range []string{"filename", "/private", "custom", "internal", "hidden-a", "hidden-b"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("forbidden value %q leaked: %s", forbidden, payload)
		}
	}
}

func TestClassifyAPIEnvelopeDoesNotExposeDetails(t *testing.T) {
	for errorType, code := range map[string]string{
		"timeout":           "loki_timeout",
		"bad_data":          "loki_query_invalid",
		"execution":         "loki_query_failed",
		"unexpected/detail": "loki_api_error",
	} {
		err := classifyAPIEnvelope(errorType)
		if !strings.Contains(err.Error(), code) || strings.Contains(err.Error(), "unexpected") {
			t.Fatalf("error type %q classified as %v", errorType, err)
		}
	}
}
