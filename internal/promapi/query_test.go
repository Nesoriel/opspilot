package promapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildMetricQueryGeneratesDeterministicSafePromQL(t *testing.T) {
	query, aggregation, limit, err := buildMetricQuery(MetricSnapshotRequest{
		Metric:      "http_requests_total",
		Matchers:    map[string]string{"pod": `web"one`, "namespace": "operations"},
		Aggregation: "SUM",
		GroupBy:     []string{"pod", "namespace"},
		Limit:       25,
	})
	if err != nil {
		t.Fatalf("build query: %v", err)
	}
	want := `sum by (namespace,pod) (http_requests_total{namespace="operations",pod="web\"one"})`
	if query != want || aggregation != "sum" || limit != 25 {
		t.Fatalf("query=%q aggregation=%q limit=%d", query, aggregation, limit)
	}

	selector, aggregation, limit, err := buildMetricQuery(MetricSnapshotRequest{Metric: "up"})
	if err != nil {
		t.Fatalf("build default selector: %v", err)
	}
	if selector != "up" || aggregation != "none" || limit != defaultSeriesLimit {
		t.Fatalf("unexpected defaults: %q %q %d", selector, aggregation, limit)
	}
}

func TestBuildMetricQueryRejectsUnsafeInputs(t *testing.T) {
	tooManyMatchers := make(map[string]string)
	for _, name := range []string{"job", "instance", "cluster", "namespace", "pod", "container", "node", "service", "endpoint"} {
		tooManyMatchers[name] = "value"
	}
	for _, request := range []MetricSnapshotRequest{
		{Metric: ""},
		{Metric: "1metric"},
		{Metric: "metric{job=\"secret\"}"},
		{Metric: "metric", Matchers: map[string]string{"secret": "value"}},
		{Metric: "metric", Matchers: map[string]string{"job": "line\nbreak"}},
		{Metric: "metric", Matchers: tooManyMatchers},
		{Metric: "metric", Aggregation: "rate"},
		{Metric: "metric", GroupBy: []string{"job"}},
		{Metric: "metric", Aggregation: "sum", GroupBy: []string{"secret"}},
		{Metric: "metric", Aggregation: "sum", GroupBy: []string{"job", "job"}},
		{Metric: "metric", Limit: 501},
	} {
		if _, _, _, err := buildMetricQuery(request); err == nil {
			t.Fatalf("unsafe request accepted: %#v", request)
		}
	}
}

func TestMapVectorFiltersLabelsSortsAndTruncates(t *testing.T) {
	rawResult, err := json.Marshal([]map[string]any{
		{
			"metric": map[string]string{"instance": "b", "job": "node", "secret": "do-not-return"},
			"value":  []any{float64(1784203201), "2"},
		},
		{
			"metric": map[string]string{"instance": "a", "job": "node", "namespace": "operations"},
			"value":  []any{float64(1784203200.5), "1"},
		},
	})
	if err != nil {
		t.Fatalf("marshal vector: %v", err)
	}
	series, truncated, err := mapVector(rawQueryData{ResultType: "vector", Result: rawResult}, diagnosticLabels, 1)
	if err != nil {
		t.Fatalf("map vector: %v", err)
	}
	if len(series) != 1 || !truncated || series[0].Labels["instance"] != "a" || series[0].Value != "1" {
		t.Fatalf("unexpected series: %#v truncated=%v", series, truncated)
	}
	payload, _ := json.Marshal(series)
	if strings.Contains(string(payload), "secret") {
		t.Fatalf("secret label leaked: %s", payload)
	}
}

func TestMapVectorRejectsUnexpectedShapes(t *testing.T) {
	for _, data := range []rawQueryData{
		{ResultType: "matrix", Result: json.RawMessage(`[]`)},
		{ResultType: "vector", Result: json.RawMessage(`not-json`)},
		{ResultType: "vector", Result: json.RawMessage(`[{"metric":{},"value":[1]}]`)},
		{ResultType: "vector", Result: json.RawMessage(`[{"metric":{},"value":["bad","1"]}]`)},
		{ResultType: "vector", Result: json.RawMessage(`[{"metric":{},"value":[1,{}]}]`)},
	} {
		if _, _, err := mapVector(data, diagnosticLabels, 10); err == nil {
			t.Fatalf("unexpected data accepted: %#v", data)
		}
	}
}

func TestPrometheusDurationFormatting(t *testing.T) {
	if value := formatPrometheusDuration(2 * time.Second); value != "2s" {
		t.Fatalf("duration = %q", value)
	}
	if value := formatPrometheusDuration(0); value != "1ms" {
		t.Fatalf("minimum duration = %q", value)
	}
}
