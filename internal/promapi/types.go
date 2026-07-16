package promapi

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ServerInfo struct {
	Build        BuildInfo   `json:"build"`
	Runtime      RuntimeInfo `json:"runtime"`
	WarningCount int         `json:"warning_count"`
	InfoCount    int         `json:"info_count"`
}

type BuildInfo struct {
	Version   string `json:"version"`
	Revision  string `json:"revision,omitempty"`
	Branch    string `json:"branch,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
}

type RuntimeInfo struct {
	StartTime           string `json:"start_time,omitempty"`
	ServerTime          string `json:"server_time,omitempty"`
	LastConfigTime      string `json:"last_config_time,omitempty"`
	ReloadConfigSuccess bool   `json:"reload_config_success"`
	TimeSeriesCount     int64  `json:"time_series_count"`
	CorruptionCount     int64  `json:"corruption_count"`
	GoroutineCount      int64  `json:"goroutine_count"`
	GOMAXPROCS          int64  `json:"gomaxprocs"`
	StorageRetention    string `json:"storage_retention,omitempty"`
}

type TargetList struct {
	Count        int             `json:"count"`
	Truncated    bool            `json:"truncated"`
	Targets      []TargetSummary `json:"targets"`
	WarningCount int             `json:"warning_count"`
	InfoCount    int             `json:"info_count"`
}

type TargetSummary struct {
	ScrapePool         string  `json:"scrape_pool"`
	Job                string  `json:"job,omitempty"`
	Instance           string  `json:"instance,omitempty"`
	Health             string  `json:"health"`
	LastScrape         string  `json:"last_scrape,omitempty"`
	LastScrapeDuration float64 `json:"last_scrape_duration_seconds"`
	ScrapeInterval     string  `json:"scrape_interval,omitempty"`
	ScrapeTimeout      string  `json:"scrape_timeout,omitempty"`
	ErrorPresent       bool    `json:"error_present"`
}

type MetricSnapshotRequest struct {
	Metric      string
	Matchers    map[string]string
	Aggregation string
	GroupBy     []string
	Limit       int
}

type MetricSnapshot struct {
	Metric       string         `json:"metric"`
	Aggregation  string         `json:"aggregation"`
	Count        int            `json:"count"`
	Truncated    bool           `json:"truncated"`
	WarningCount int            `json:"warning_count"`
	InfoCount    int            `json:"info_count"`
	Series       []MetricSeries `json:"series"`
}

type MetricSeries struct {
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp string            `json:"timestamp"`
	Value     string            `json:"value"`
}

type rawBuildInfo struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

type rawRuntimeInfo struct {
	StartTime           string `json:"startTime"`
	ServerTime          string `json:"serverTime"`
	LastConfigTime      string `json:"lastConfigTime"`
	ReloadConfigSuccess bool   `json:"reloadConfigSuccess"`
	TimeSeriesCount     int64  `json:"timeSeriesCount"`
	CorruptionCount     int64  `json:"corruptionCount"`
	GoroutineCount      int64  `json:"goroutineCount"`
	GOMAXPROCS          int64  `json:"GOMAXPROCS"`
	StorageRetention    string `json:"storageRetention"`
}

type rawTargets struct {
	ActiveTargets []rawTarget `json:"activeTargets"`
}

type rawTarget struct {
	Labels             map[string]string `json:"labels"`
	ScrapePool         string            `json:"scrapePool"`
	LastError          string            `json:"lastError"`
	LastScrape         string            `json:"lastScrape"`
	LastScrapeDuration float64           `json:"lastScrapeDuration"`
	Health             string            `json:"health"`
	ScrapeInterval     string            `json:"scrapeInterval"`
	ScrapeTimeout      string            `json:"scrapeTimeout"`
}

type rawQueryData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

type rawVectorSample struct {
	Metric map[string]string `json:"metric"`
	Value  []json.RawMessage `json:"value"`
}

func mapBuildInfo(raw rawBuildInfo) BuildInfo {
	return BuildInfo{
		Version:   sanitizeDisplay(raw.Version, 128),
		Revision:  sanitizeDisplay(raw.Revision, 128),
		Branch:    sanitizeDisplay(raw.Branch, 128),
		BuildDate: sanitizeDisplay(raw.BuildDate, 128),
		GoVersion: sanitizeDisplay(raw.GoVersion, 128),
	}
}

func mapRuntimeInfo(raw rawRuntimeInfo) RuntimeInfo {
	return RuntimeInfo{
		StartTime:           normalizeTimestamp(raw.StartTime),
		ServerTime:          normalizeTimestamp(raw.ServerTime),
		LastConfigTime:      normalizeTimestamp(raw.LastConfigTime),
		ReloadConfigSuccess: raw.ReloadConfigSuccess,
		TimeSeriesCount:     raw.TimeSeriesCount,
		CorruptionCount:     raw.CorruptionCount,
		GoroutineCount:      raw.GoroutineCount,
		GOMAXPROCS:          raw.GOMAXPROCS,
		StorageRetention:    sanitizeDisplay(raw.StorageRetention, 64),
	}
}

func mapTargets(raw rawTargets, limit int) ([]TargetSummary, bool) {
	result := make([]TargetSummary, 0, len(raw.ActiveTargets))
	for _, target := range raw.ActiveTargets {
		result = append(result, TargetSummary{
			ScrapePool:         sanitizeDisplay(target.ScrapePool, 256),
			Job:                sanitizeDisplay(target.Labels["job"], 256),
			Instance:           sanitizeDisplay(target.Labels["instance"], 512),
			Health:             sanitizeToken(target.Health),
			LastScrape:         normalizeTimestamp(target.LastScrape),
			LastScrapeDuration: target.LastScrapeDuration,
			ScrapeInterval:     sanitizeDisplay(target.ScrapeInterval, 64),
			ScrapeTimeout:      sanitizeDisplay(target.ScrapeTimeout, 64),
			ErrorPresent:       strings.TrimSpace(target.LastError) != "",
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ScrapePool != result[j].ScrapePool {
			return result[i].ScrapePool < result[j].ScrapePool
		}
		if result[i].Job != result[j].Job {
			return result[i].Job < result[j].Job
		}
		return result[i].Instance < result[j].Instance
	})
	truncated := len(result) > limit
	if truncated {
		result = result[:limit]
	}
	return result, truncated
}

func mapVector(data rawQueryData, labelAllowlist map[string]struct{}, limit int) ([]MetricSeries, bool, error) {
	if data.ResultType != "vector" {
		return nil, false, fmt.Errorf("prometheus_invalid_response: generated metric selector returned %q instead of vector", data.ResultType)
	}
	var raw []rawVectorSample
	if err := json.Unmarshal(data.Result, &raw); err != nil {
		return nil, false, fmt.Errorf("prometheus_invalid_response: vector result could not be decoded")
	}
	series := make([]MetricSeries, 0, len(raw))
	for _, item := range raw {
		if len(item.Value) != 2 {
			return nil, false, fmt.Errorf("prometheus_invalid_response: vector sample has an invalid value")
		}
		var timestamp float64
		if err := json.Unmarshal(item.Value[0], &timestamp); err != nil {
			return nil, false, fmt.Errorf("prometheus_invalid_response: vector timestamp is invalid")
		}
		formattedTimestamp, err := formatSampleTimestamp(timestamp)
		if err != nil {
			return nil, false, err
		}
		var value string
		if err := json.Unmarshal(item.Value[1], &value); err != nil {
			return nil, false, fmt.Errorf("prometheus_invalid_response: vector sample value is invalid")
		}
		labels := make(map[string]string)
		for name, labelValue := range item.Metric {
			if _, allowed := labelAllowlist[name]; allowed {
				labels[name] = sanitizeDisplay(labelValue, 512)
			}
		}
		series = append(series, MetricSeries{
			Labels:    labels,
			Timestamp: formattedTimestamp,
			Value:     sanitizeSampleValue(value),
		})
	}
	sort.Slice(series, func(i, j int) bool {
		left := canonicalLabels(series[i].Labels)
		right := canonicalLabels(series[j].Labels)
		if left != right {
			return left < right
		}
		return series[i].Value < series[j].Value
	})
	truncated := len(series) > limit
	if truncated {
		series = series[:limit]
	}
	return series, truncated, nil
}

func formatSampleTimestamp(timestamp float64) (string, error) {
	if math.IsNaN(timestamp) || math.IsInf(timestamp, 0) {
		return "", fmt.Errorf("prometheus_invalid_response: vector timestamp is not finite")
	}
	seconds, fraction := math.Modf(timestamp)
	if seconds < -62135596800 || seconds > 253402300799 {
		return "", fmt.Errorf("prometheus_invalid_response: vector timestamp is outside the supported range")
	}
	parsed := time.Unix(int64(seconds), int64(math.Round(fraction*float64(time.Second)))).UTC()
	if parsed.Year() < 1 || parsed.Year() > 9999 {
		return "", fmt.Errorf("prometheus_invalid_response: vector timestamp is outside the supported range")
	}
	return parsed.Format(time.RFC3339Nano), nil
}

func canonicalLabels(labels map[string]string) string {
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, name := range names {
		builder.WriteString(name)
		builder.WriteByte('=')
		builder.WriteString(labels[name])
		builder.WriteByte('\x00')
	}
	return builder.String()
}

func normalizeTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

func sanitizeDisplay(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	count := 0
	for _, character := range value {
		if count == limit {
			break
		}
		if character < 0x20 || character == 0x7f {
			continue
		}
		builder.WriteRune(character)
		count++
	}
	return builder.String()
}

func sanitizeSampleValue(value string) string {
	value = strings.TrimSpace(value)
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return "invalid"
	}
	return value
}
