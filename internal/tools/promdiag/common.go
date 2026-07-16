package promdiag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Nesoriel/opspilot/internal/promapi"
)

type Client interface {
	ServerInfo(ctx context.Context) (promapi.ServerInfo, error)
	TargetList(ctx context.Context, limit int) (promapi.TargetList, error)
	MetricSnapshot(ctx context.Context, request promapi.MetricSnapshotRequest) (promapi.MetricSnapshot, error)
}

func decodeStrict(arguments json.RawMessage, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode arguments: multiple JSON values are not allowed")
		}
		return fmt.Errorf("decode arguments: %w", err)
	}
	return nil
}

func encodeResult(value any) (json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return payload, nil
}
