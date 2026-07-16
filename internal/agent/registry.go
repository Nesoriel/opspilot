package agent

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

var (
	ErrToolNotFound  = errors.New("tool not found")
	ErrDuplicateTool = errors.New("tool already registered")
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return errors.New("tool is nil")
	}

	definition := tool.Definition()
	if !toolNamePattern.MatchString(definition.Name) {
		return fmt.Errorf("invalid tool name %q", definition.Name)
	}
	if definition.Description == "" {
		return fmt.Errorf("tool %q has no description", definition.Name)
	}
	if len(definition.InputSchema) == 0 || !jsonObject(definition.InputSchema) {
		return fmt.Errorf("tool %q has an invalid input schema", definition.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[definition.Name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateTool, definition.Name)
	}
	r.tools[definition.Name] = tool
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) Definitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, tool.Definition())
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})
	return definitions
}

func jsonObject(raw []byte) bool {
	for _, b := range raw {
		switch b {
		case ' ', '\n', '\r', '\t':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}
