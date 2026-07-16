package lokidiag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/Nesoriel/opspilot/internal/lokiapi"
)

type Client interface {
	ServerInfo(ctx context.Context) (lokiapi.ServerInfo, error)
	StreamSummary(ctx context.Context, request lokiapi.StreamSummaryRequest) (lokiapi.StreamSummary, error)
}

func decodeStrict(arguments json.RawMessage, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return errors.New("loki_tool_invalid_arguments: arguments must match the tool schema")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("loki_tool_invalid_arguments: multiple JSON values are not allowed")
	}
	return nil
}

func encodeResult(value any) (json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("loki_tool_result_error: result could not be encoded")
	}
	return payload, nil
}
