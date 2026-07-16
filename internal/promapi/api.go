package promapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultTargetLimit = 100
	maxTargetLimit     = 500
	defaultSeriesLimit = 100
	maxSeriesLimit     = 500
	maxMatchers        = 8
	maxGroupLabels     = 5
	maxMatcherValue    = 256
)

var diagnosticLabels = map[string]struct{}{
	"job":       {},
	"instance":  {},
	"cluster":   {},
	"namespace": {},
	"pod":       {},
	"container": {},
	"node":      {},
	"service":   {},
	"endpoint":  {},
}

var supportedAggregations = map[string]struct{}{
	"none":  {},
	"sum":   {},
	"avg":   {},
	"min":   {},
	"max":   {},
	"count": {},
}

func (c *Client) ServerInfo(ctx context.Context) (ServerInfo, error) {
	var build rawBuildInfo
	buildMeta, err := c.get(ctx, "/api/v1/status/buildinfo", nil, &build)
	if err != nil {
		return ServerInfo{}, err
	}
	var runtime rawRuntimeInfo
	runtimeMeta, err := c.get(ctx, "/api/v1/status/runtimeinfo", nil, &runtime)
	if err != nil {
		return ServerInfo{}, err
	}
	return ServerInfo{
		Build:        mapBuildInfo(build),
		Runtime:      mapRuntimeInfo(runtime),
		WarningCount: buildMeta.WarningCount + runtimeMeta.WarningCount,
		InfoCount:    buildMeta.InfoCount + runtimeMeta.InfoCount,
	}, nil
}

func (c *Client) TargetList(ctx context.Context, limit int) (TargetList, error) {
	if limit == 0 {
		limit = defaultTargetLimit
	}
	if limit < 1 || limit > maxTargetLimit {
		return TargetList{}, errors.New("prometheus_request_invalid: target limit must be between 1 and 500")
	}
	query := url.Values{"state": []string{"active"}}
	var raw rawTargets
	meta, err := c.get(ctx, "/api/v1/targets", query, &raw)
	if err != nil {
		return TargetList{}, err
	}
	targets, truncated := mapTargets(raw, limit)
	return TargetList{
		Count:        len(targets),
		Truncated:    truncated,
		Targets:      targets,
		WarningCount: meta.WarningCount,
		InfoCount:    meta.InfoCount,
	}, nil
}

func (c *Client) MetricSnapshot(ctx context.Context, request MetricSnapshotRequest) (MetricSnapshot, error) {
	queryExpression, aggregation, limit, err := buildMetricQuery(request)
	if err != nil {
		return MetricSnapshot{}, err
	}
	query := url.Values{
		"query":   []string{queryExpression},
		"timeout": []string{formatPrometheusDuration(c.queryTimeout())},
		"limit":   []string{strconv.Itoa(limit)},
	}
	var raw rawQueryData
	meta, err := c.get(ctx, "/api/v1/query", query, &raw)
	if err != nil {
		return MetricSnapshot{}, err
	}
	series, truncated, err := mapVector(raw, diagnosticLabels, limit)
	if err != nil {
		return MetricSnapshot{}, err
	}
	return MetricSnapshot{
		Metric:       request.Metric,
		Aggregation:  aggregation,
		Count:        len(series),
		Truncated:    truncated,
		WarningCount: meta.WarningCount,
		InfoCount:    meta.InfoCount,
		Series:       series,
	}, nil
}

func (c *Client) queryTimeout() time.Duration {
	c.initialize()
	if c.config.QueryTimeout <= 0 {
		return defaultQueryTimeout
	}
	return c.config.QueryTimeout
}

func buildMetricQuery(request MetricSnapshotRequest) (string, string, int, error) {
	metric := strings.TrimSpace(request.Metric)
	if !validMetricName(metric) {
		return "", "", 0, errors.New("prometheus_query_invalid: metric must use the safe ASCII Prometheus name syntax")
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultSeriesLimit
	}
	if limit < 1 || limit > maxSeriesLimit {
		return "", "", 0, errors.New("prometheus_query_invalid: series limit must be between 1 and 500")
	}
	if len(request.Matchers) > maxMatchers {
		return "", "", 0, errors.New("prometheus_query_invalid: at most 8 label matchers are allowed")
	}

	matcherNames := make([]string, 0, len(request.Matchers))
	for name, value := range request.Matchers {
		if _, allowed := diagnosticLabels[name]; !allowed {
			return "", "", 0, fmt.Errorf("prometheus_query_invalid: label %q is not in the diagnostic allowlist", name)
		}
		if len([]rune(value)) > maxMatcherValue || strings.ContainsAny(value, "\r\n\x00") {
			return "", "", 0, fmt.Errorf("prometheus_query_invalid: matcher value for %q is invalid", name)
		}
		matcherNames = append(matcherNames, name)
	}
	sort.Strings(matcherNames)

	var selector strings.Builder
	selector.WriteString(metric)
	if len(matcherNames) > 0 {
		selector.WriteByte('{')
		for index, name := range matcherNames {
			if index > 0 {
				selector.WriteByte(',')
			}
			selector.WriteString(name)
			selector.WriteByte('=')
			selector.WriteString(strconv.Quote(request.Matchers[name]))
		}
		selector.WriteByte('}')
	}

	aggregation := strings.ToLower(strings.TrimSpace(request.Aggregation))
	if aggregation == "" {
		aggregation = "none"
	}
	if _, supported := supportedAggregations[aggregation]; !supported {
		return "", "", 0, errors.New("prometheus_query_invalid: unsupported aggregation")
	}
	if len(request.GroupBy) > maxGroupLabels {
		return "", "", 0, errors.New("prometheus_query_invalid: at most 5 grouping labels are allowed")
	}
	groupBy := append([]string(nil), request.GroupBy...)
	seen := make(map[string]struct{}, len(groupBy))
	for _, name := range groupBy {
		if _, allowed := diagnosticLabels[name]; !allowed {
			return "", "", 0, fmt.Errorf("prometheus_query_invalid: grouping label %q is not in the diagnostic allowlist", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return "", "", 0, errors.New("prometheus_query_invalid: grouping labels must be unique")
		}
		seen[name] = struct{}{}
	}
	sort.Strings(groupBy)
	if aggregation == "none" && len(groupBy) > 0 {
		return "", "", 0, errors.New("prometheus_query_invalid: grouping requires an aggregation")
	}
	if aggregation == "none" {
		return selector.String(), aggregation, limit, nil
	}
	if len(groupBy) == 0 {
		return aggregation + "(" + selector.String() + ")", aggregation, limit, nil
	}
	return aggregation + " by (" + strings.Join(groupBy, ",") + ") (" + selector.String() + ")", aggregation, limit, nil
}

func validMetricName(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		valid := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character == '_' || character == ':' ||
			(index > 0 && character >= '0' && character <= '9')
		if !valid {
			return false
		}
	}
	return true
}

func formatPrometheusDuration(duration time.Duration) string {
	milliseconds := duration.Milliseconds()
	if milliseconds < 1 {
		milliseconds = 1
	}
	return strconv.FormatInt(milliseconds, 10) + "ms"
}
