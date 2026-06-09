// Package mcp is a thin client over the Model Context Protocol Go SDK. It lets
// vala connect to remote MCP servers (e.g. Scanner's security data lake) over
// the streamable-HTTP transport, discover the tools they expose, and call them.
//
// The rest of vala depends only on the small Session interface defined here, so
// the concrete SDK client stays isolated and tests can inject a fake session
// for deterministic, offline runs.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolDesc describes one tool exposed by a remote MCP server.
type ToolDesc struct {
	// Name is the server-side tool name (e.g. "execute_query").
	Name string
	// Description is the guidance the server provides for the model.
	Description string
	// InputSchema is the tool's JSON Schema "properties" map and required list,
	// as published by the server.
	Properties map[string]any
	Required   []string
	// ReadOnly reflects the server's readOnlyHint annotation. Tools without the
	// hint are treated as not read-only so they fail closed through the gate.
	ReadOnly bool
}

// CallResult is the flattened outcome of a tools/call. Text concatenates the
// textual content blocks the server returned; IsError marks a tool-level error.
type CallResult struct {
	Text    string
	IsError bool
}

// Session is the minimal surface the rest of vala needs from an MCP server. It
// is satisfied by the real SDK-backed session and by test fakes.
type Session interface {
	// Name is the configured server name (used to namespace tools).
	Name() string
	// ListTools enumerates the tools the server exposes.
	ListTools(ctx context.Context) ([]ToolDesc, error)
	// CallTool invokes a tool by its server-side name with raw JSON arguments.
	CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error)
	// Close tears down the session.
	Close() error
}

// ServerConfig describes how to reach one MCP server.
type ServerConfig struct {
	// Name namespaces the server's tools inside vala (e.g. "scanner").
	Name string
	// URL is the streamable-HTTP endpoint.
	URL string
	// APIKey, if set, is sent as an "Authorization: Bearer <key>" header.
	APIKey string
}

// Connect dials an MCP server over streamable HTTP and returns a live session.
func Connect(ctx context.Context, cfg ServerConfig) (Session, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("mcp server %q: no URL configured", cfg.Name)
	}
	httpClient := http.DefaultClient
	if cfg.APIKey != "" {
		httpClient = &http.Client{Transport: &bearerTransport{key: cfg.APIKey, base: http.DefaultTransport}}
	}
	transport := &sdk.StreamableClientTransport{Endpoint: cfg.URL, HTTPClient: httpClient}
	client := sdk.NewClient(&sdk.Implementation{Name: "vala", Version: "0.1.0"}, nil)
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to mcp server %q: %w", cfg.Name, err)
	}
	return &session{cs: cs, name: cfg.Name}, nil
}

// bearerTransport injects a Bearer token on every request. Scanner (and most
// hosted MCP servers) authenticate with "Authorization: Bearer <api key>".
type bearerTransport struct {
	key  string
	base http.RoundTripper
}

func (b *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+b.key)
	return b.base.RoundTrip(clone)
}

// session adapts an SDK client session to the Session interface.
type session struct {
	cs   *sdk.ClientSession
	name string
}

func (s *session) Name() string { return s.name }

func (s *session) ListTools(ctx context.Context) ([]ToolDesc, error) {
	var out []ToolDesc
	var cursor string
	for {
		res, err := s.cs.ListTools(ctx, &sdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("list tools on %q: %w", s.name, err)
		}
		for _, t := range res.Tools {
			props, required := splitSchema(t.InputSchema)
			out = append(out, ToolDesc{
				Name:        t.Name,
				Description: t.Description,
				Properties:  props,
				Required:    required,
				ReadOnly:    t.Annotations != nil && t.Annotations.ReadOnlyHint,
			})
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return out, nil
}

func (s *session) CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error) {
	res, err := s.cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: json.RawMessage(args)})
	if err != nil {
		return CallResult{}, fmt.Errorf("call %q on %q: %w", name, s.name, err)
	}
	return CallResult{Text: flatten(res), IsError: res.IsError}, nil
}

func (s *session) Close() error { return s.cs.Close() }

// flatten reduces a tool result to a single string: the concatenated text
// content blocks, falling back to the JSON-encoded structured content.
func flatten(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(tc.Text)
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			return string(raw)
		}
	}
	return b.String()
}

// splitSchema extracts the "properties" map and "required" list from a server's
// published input schema (delivered to the client as a map[string]any).
func splitSchema(schema any) (map[string]any, []string) {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil, nil
	}
	props, _ := m["properties"].(map[string]any)
	var required []string
	if raw, ok := m["required"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				required = append(required, s)
			}
		}
	}
	return props, required
}
