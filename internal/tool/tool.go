// Package tool defines the Tool interface that the agent uses to act on the
// world, plus a Registry for looking them up and exposing them to the LLM.
//
// The design mirrors charmbracelet/crush: every tool is one Go type whose
// human-readable description lives in a sibling .md file embedded at build
// time. Keeping descriptions out of Go source keeps prompt text reviewable and
// lets us iterate on tool guidance without touching code.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
)

// Schema is a minimal JSON Schema (draft 2020-12) description of a tool's
// input object. Properties is the raw "properties" map; Required lists the
// required property names.
type Schema struct {
	Properties map[string]any
	Required   []string
}

// Result is the outcome of running a tool. Content is fed back to the model as
// a tool_result block; IsError marks the result as an error so the model can
// react accordingly without the whole turn failing.
type Result struct {
	Content string
	IsError bool
}

// Text returns a successful result with the given content.
func Text(content string) Result { return Result{Content: content} }

// Errorf returns an error result with a formatted message.
func Errorf(format string, args ...any) Result {
	return Result{Content: fmt.Sprintf(format, args...), IsError: true}
}

// Tool is a single capability the agent can invoke.
type Tool interface {
	// Name is the identifier the model uses to call the tool. It must match
	// ^[a-zA-Z0-9_-]+$.
	Name() string
	// Description is the guidance shown to the model, typically loaded from an
	// embedded .md file.
	Description() string
	// Schema describes the tool's input object.
	Schema() Schema
	// ReadOnly reports whether the tool only observes state. Read-only tools
	// bypass the permission gate; everything else must be approved.
	ReadOnly() bool
	// Run executes the tool with the model-supplied JSON input.
	Run(ctx context.Context, input json.RawMessage) (Result, error)
}

// Registry holds the set of tools available to an agent.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool, replacing any existing tool with the same name.
func (r *Registry) Register(tools ...Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
}

// Get returns the named tool, if registered.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All returns the registered tools sorted by name for stable output.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

// ToAnthropic converts the registry into the tool parameters the Anthropic
// Messages API expects.
func (r *Registry) ToAnthropic() []anthropic.ToolUnionParam {
	return r.ToAnthropicFiltered(nil)
}

// ToAnthropicFiltered is like ToAnthropic but only includes tools for which the
// predicate returns true. A nil predicate includes every tool. The governed
// loop uses this to expose only the tools permitted in the current phase, so
// the model never even sees a write/destructive tool during investigation.
func (r *Registry) ToAnthropicFiltered(include func(Tool) bool) []anthropic.ToolUnionParam {
	tools := r.All()
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		if include != nil && !include(t) {
			continue
		}
		s := t.Schema()
		props := s.Properties
		if props == nil {
			props = map[string]any{}
		}
		out = append(out, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name(),
				Description: anthropic.String(t.Description()),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: props,
					Required:   s.Required,
				},
			},
		})
	}
	return out
}
