package promapi

import (
	"math"
	"strings"
	"testing"
)

func TestFormatSampleTimestampRejectsInvalidValues(t *testing.T) {
	for _, value := range []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		-62135596801,
		253402300800,
	} {
		if _, err := formatSampleTimestamp(value); err == nil || !strings.Contains(err.Error(), "prometheus_invalid_response") {
			t.Fatalf("timestamp %v was accepted: %v", value, err)
		}
	}
}

func TestFormatSampleTimestampAcceptsFractionalSeconds(t *testing.T) {
	value, err := formatSampleTimestamp(1784203201.5)
	if err != nil {
		t.Fatalf("format timestamp: %v", err)
	}
	if value != "2026-07-16T12:00:01.5Z" {
		t.Fatalf("timestamp = %q", value)
	}
}
