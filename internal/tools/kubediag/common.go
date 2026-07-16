package kubediag

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Nesoriel/opspilot/internal/kubeapi"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	defaultNodeLimit  = 100
	maxNodeLimit      = 200
	defaultPodLimit   = 100
	maxPodLimit       = 200
	defaultEventLimit = 50
	maxEventLimit     = 100
)

type Client interface {
	kubeapi.Reader
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

func validateNamespace(value string, allowAll bool) error {
	value = strings.TrimSpace(value)
	if allowAll && value == "*" {
		return nil
	}
	if value == "" {
		return errors.New("namespace is required")
	}
	if problems := validation.IsDNS1123Label(value); len(problems) > 0 {
		return errors.New("namespace must be a valid DNS-1123 label")
	}
	return nil
}

func validatePodName(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("pod is required")
	}
	if problems := validation.IsDNS1123Subdomain(value); len(problems) > 0 {
		return errors.New("pod must be a valid DNS-1123 subdomain")
	}
	return nil
}
