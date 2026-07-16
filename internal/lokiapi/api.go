package lokiapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLookbackMinutes = 60
	minLookbackMinutes     = 1
	maxLookbackMinutes     = 360
	defaultStreamLimit     = 100
	maxStreamLimit         = 500
	maxMatchers            = 8
	maxMatcherValue        = 256
)

var diagnosticLabels = map[string]struct{}{
	"job":          {},
	"app":          {},
	"application":  {},
	"service":      {},
	"service_name": {},
	"namespace":    {},
	"pod":          {},
	"container":    {},
	"cluster":      {},
	"node":         {},
	"level":        {},
	"environment":  {},
}

func (c *Client) ServerInfo(ctx context.Context) (ServerInfo, error) {
	ready, statusCode, err := c.readiness(ctx)
	if err != nil {
		return ServerInfo{}, err
	}
	var build rawBuildInfo
	if err := c.getJSON(ctx, "/loki/api/v1/status/buildinfo", &build); err != nil {
		return ServerInfo{}, err
	}
	return ServerInfo{
		Ready:           ready,
		ReadyStatusCode: statusCode,
		Build:           mapBuildInfo(build),
	}, nil
}

func (c *Client) StreamSummary(ctx context.Context, request StreamSummaryRequest) (StreamSummary, error) {
	selector, lookback, limit, err := buildStreamSelector(request)
	if err != nil {
		return StreamSummary{}, err
	}
	if err := c.ready(); err != nil {
		return StreamSummary{}, err
	}
	now := c.config.Now().UTC()
	from := now.Add(-lookback)
	form := url.Values{
		"match[]": []string{selector},
		"start":   []string{strconv.FormatInt(from.UnixNano(), 10)},
		"end":     []string{strconv.FormatInt(now.UnixNano(), 10)},
	}
	var envelope rawSeriesEnvelope
	if err := c.postFormJSON(ctx, "/loki/api/v1/series", form, &envelope); err != nil {
		return StreamSummary{}, err
	}
	if envelope.Status != "success" {
		return StreamSummary{}, classifyAPIEnvelope(envelope.ErrorType)
	}
	streams, truncated := mapStreams(envelope.Data, diagnosticLabels, limit)
	return StreamSummary{
		From:      from.Format(time.RFC3339Nano),
		Through:   now.Format(time.RFC3339Nano),
		Count:     len(streams),
		Truncated: truncated,
		Streams:   streams,
	}, nil
}

func buildStreamSelector(request StreamSummaryRequest) (string, time.Duration, int, error) {
	if len(request.Matchers) == 0 {
		return "", 0, 0, errors.New("loki_query_invalid: at least one exact label matcher is required")
	}
	if len(request.Matchers) > maxMatchers {
		return "", 0, 0, errors.New("loki_query_invalid: at most 8 label matchers are allowed")
	}

	names := make([]string, 0, len(request.Matchers))
	for name, value := range request.Matchers {
		if _, allowed := diagnosticLabels[name]; !allowed {
			return "", 0, 0, fmt.Errorf("loki_query_invalid: label %q is not in the diagnostic allowlist", name)
		}
		if len([]rune(value)) == 0 || len([]rune(value)) > maxMatcherValue || strings.ContainsAny(value, "\r\n\x00") {
			return "", 0, 0, fmt.Errorf("loki_query_invalid: matcher value for %q is invalid", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var selector strings.Builder
	selector.WriteByte('{')
	for index, name := range names {
		if index > 0 {
			selector.WriteByte(',')
		}
		selector.WriteString(name)
		selector.WriteByte('=')
		selector.WriteString(strconv.Quote(request.Matchers[name]))
	}
	selector.WriteByte('}')

	lookbackMinutes := request.Lookback
	if lookbackMinutes == 0 {
		lookbackMinutes = defaultLookbackMinutes
	}
	if lookbackMinutes < minLookbackMinutes || lookbackMinutes > maxLookbackMinutes {
		return "", 0, 0, errors.New("loki_query_invalid: lookback must be between 1 and 360 minutes")
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultStreamLimit
	}
	if limit < 1 || limit > maxStreamLimit {
		return "", 0, 0, errors.New("loki_query_invalid: stream limit must be between 1 and 500")
	}
	return selector.String(), time.Duration(lookbackMinutes) * time.Minute, limit, nil
}

func classifyAPIEnvelope(errorType string) error {
	switch sanitizeToken(errorType) {
	case "timeout", "canceled":
		return errors.New("loki_timeout: query timed out or was canceled")
	case "bad_data":
		return errors.New("loki_query_invalid: generated stream selector was rejected")
	case "execution":
		return errors.New("loki_query_failed: query execution failed")
	default:
		return errors.New("loki_api_error: Loki API returned an error")
	}
}

func sanitizeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return ""
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-') {
			return ""
		}
	}
	return strings.ToLower(value)
}
