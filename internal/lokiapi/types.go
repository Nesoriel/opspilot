package lokiapi

import (
	"sort"
	"strings"
)

type ServerInfo struct {
	Ready           bool      `json:"ready"`
	ReadyStatusCode int       `json:"ready_status_code"`
	Build           BuildInfo `json:"build"`
}

type BuildInfo struct {
	Version   string `json:"version"`
	Revision  string `json:"revision,omitempty"`
	Branch    string `json:"branch,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
}

type StreamSummaryRequest struct {
	Matchers map[string]string
	Lookback int
	Limit    int
}

type StreamSummary struct {
	From      string              `json:"from"`
	Through   string              `json:"through"`
	Count     int                 `json:"count"`
	Truncated bool                `json:"truncated"`
	Streams   []StreamSummaryItem `json:"streams"`
}

type StreamSummaryItem struct {
	Labels map[string]string `json:"labels"`
}

type rawBuildInfo struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

type rawSeriesEnvelope struct {
	Status    string              `json:"status"`
	Data      []map[string]string `json:"data"`
	ErrorType string              `json:"errorType"`
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

func mapStreams(raw []map[string]string, allowlist map[string]struct{}, limit int) ([]StreamSummaryItem, bool) {
	unique := make(map[string]StreamSummaryItem, len(raw))
	for _, labels := range raw {
		projected := make(map[string]string)
		for name, value := range labels {
			if _, allowed := allowlist[name]; allowed {
				projected[name] = sanitizeDisplay(value, 512)
			}
		}
		if len(projected) == 0 {
			continue
		}
		item := StreamSummaryItem{Labels: projected}
		unique[canonicalLabels(projected)] = item
	}
	result := make([]StreamSummaryItem, 0, len(unique))
	for _, item := range unique {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return canonicalLabels(result[i].Labels) < canonicalLabels(result[j].Labels)
	})
	truncated := len(result) > limit
	if truncated {
		result = result[:limit]
	}
	return result, truncated
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
