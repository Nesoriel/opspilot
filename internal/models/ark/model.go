package ark

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/cloudwego/eino-ext/components/model/agenticark"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model/responses"
)

type backend interface {
	Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.AgenticMessage, error)
}

type Model struct {
	backend backend
	secrets []string
}

func New(ctx context.Context, config Config) (*Model, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	arkConfig := &agenticark.Config{
		APIKey:            config.APIKey,
		AccessKey:         config.AccessKey,
		SecretKey:         config.SecretKey,
		Model:             config.Model,
		BaseURL:           config.BaseURL,
		Region:            config.Region,
		Timeout:           &config.Timeout,
		RetryTimes:        &config.RetryTimes,
		ParallelToolCalls: config.ParallelToolCalls,
	}
	if config.MaxToolCalls > 0 {
		arkConfig.MaxToolCalls = &config.MaxToolCalls
	}

	switch config.Thinking {
	case "", ThinkingAuto:
	case ThinkingEnabled:
		arkConfig.Thinking = &responses.ResponsesThinking{Type: responses.ThinkingType_enabled.Enum()}
	case ThinkingDisabled:
		arkConfig.Thinking = &responses.ResponsesThinking{Type: responses.ThinkingType_disabled.Enum()}
	default:
		return nil, fmt.Errorf("unsupported Ark thinking mode %q", config.Thinking)
	}

	client, err := agenticark.New(ctx, arkConfig)
	if err != nil {
		return nil, redactError(err, config.APIKey, config.AccessKey, config.SecretKey)
	}
	return newWithBackend(client, config.APIKey, config.AccessKey, config.SecretKey), nil
}

func newWithBackend(client backend, secrets ...string) *Model {
	filtered := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			filtered = append(filtered, secret)
		}
	}
	return &Model{backend: client, secrets: filtered}
}

func (m *Model) Generate(ctx context.Context, messages []agent.Message, tools []agent.ToolDefinition) (agent.ModelResponse, error) {
	if m == nil || m.backend == nil {
		return agent.ModelResponse{}, errors.New("Ark model is not initialized")
	}

	input, err := toAgenticMessages(messages)
	if err != nil {
		return agent.ModelResponse{}, err
	}
	toolInfos, err := toToolInfos(tools)
	if err != nil {
		return agent.ModelResponse{}, err
	}

	options := make([]einomodel.Option, 0, 1)
	if len(toolInfos) > 0 {
		options = append(options, einomodel.WithTools(toolInfos))
	}
	response, err := m.backend.Generate(ctx, input, options...)
	if err != nil {
		return agent.ModelResponse{}, redactError(err, m.secrets...)
	}
	return fromAgenticMessage(response)
}

func redactError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return errors.New(message)
}
