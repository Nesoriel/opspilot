package ark

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ThinkingMode string

const (
	ThinkingAuto     ThinkingMode = "auto"
	ThinkingEnabled  ThinkingMode = "enabled"
	ThinkingDisabled ThinkingMode = "disabled"
)

type Config struct {
	APIKey            string
	AccessKey         string
	SecretKey         string
	Model             string
	BaseURL           string
	Region            string
	Timeout           time.Duration
	RetryTimes        int
	Thinking          ThinkingMode
	MaxToolCalls      int64
	ParallelToolCalls *bool
}

func ConfigFromEnv() (Config, error) {
	return configFromLookup(os.Getenv)
}

func configFromLookup(lookup func(string) string) (Config, error) {
	config := Config{
		APIKey:     strings.TrimSpace(lookup("ARK_API_KEY")),
		AccessKey:  strings.TrimSpace(lookup("ARK_ACCESS_KEY")),
		SecretKey:  strings.TrimSpace(lookup("ARK_SECRET_KEY")),
		Model:      strings.TrimSpace(lookup("ARK_MODEL_ID")),
		BaseURL:    strings.TrimSpace(lookup("ARK_BASE_URL")),
		Region:     strings.TrimSpace(lookup("ARK_REGION")),
		Timeout:    60 * time.Second,
		RetryTimes: 2,
		Thinking:   ThinkingAuto,
	}

	if raw := strings.TrimSpace(lookup("ARK_TIMEOUT")); raw != "" {
		duration, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse ARK_TIMEOUT: %w", err)
		}
		config.Timeout = duration
	}
	if raw := strings.TrimSpace(lookup("ARK_RETRY_TIMES")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse ARK_RETRY_TIMES: %w", err)
		}
		config.RetryTimes = value
	}
	if raw := strings.TrimSpace(lookup("ARK_THINKING")); raw != "" {
		config.Thinking = ThinkingMode(strings.ToLower(raw))
	}
	if raw := strings.TrimSpace(lookup("ARK_MAX_TOOL_CALLS")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse ARK_MAX_TOOL_CALLS: %w", err)
		}
		config.MaxToolCalls = value
	}
	if raw := strings.TrimSpace(lookup("ARK_PARALLEL_TOOL_CALLS")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse ARK_PARALLEL_TOOL_CALLS: %w", err)
		}
		config.ParallelToolCalls = &value
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if c.Model == "" {
		return errors.New("ARK_MODEL_ID is required")
	}
	if c.APIKey == "" {
		if c.AccessKey == "" && c.SecretKey == "" {
			return errors.New("ARK_API_KEY or both ARK_ACCESS_KEY and ARK_SECRET_KEY are required")
		}
		if c.AccessKey == "" || c.SecretKey == "" {
			return errors.New("ARK_ACCESS_KEY and ARK_SECRET_KEY must be configured together")
		}
	}
	if c.Timeout <= 0 {
		return errors.New("Ark timeout must be greater than zero")
	}
	if c.RetryTimes < 0 {
		return errors.New("Ark retry count cannot be negative")
	}
	if c.MaxToolCalls < 0 {
		return errors.New("Ark maximum tool calls cannot be negative")
	}
	switch c.Thinking {
	case "", ThinkingAuto, ThinkingEnabled, ThinkingDisabled:
	default:
		return fmt.Errorf("unsupported ARK_THINKING value %q", c.Thinking)
	}
	return nil
}
