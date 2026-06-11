// Package mcp is a thin client over the Model Context Protocol Go SDK. It lets
// vala connect to MCP servers — both remote servers over streamable HTTP (e.g.
// Scanner's security data lake) and local subprocesses over stdio (e.g. the Wiz
// Security Graph server) — discover the tools they expose, and call them.
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
	"os"
	"os/exec"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Transport names the wire vala uses to reach an MCP server.
const (
	// TransportHTTP dials a remote server over streamable HTTP. It is the
	// default when a server leaves its transport unset.
	TransportHTTP = "http"
	// TransportStdio launches a local subprocess and speaks newline-delimited
	// JSON over its stdin/stdout. Security servers that ship as a CLI (e.g. Wiz)
	// use this.
	TransportStdio = "stdio"
)

// EvidenceStatus reports the outcome of connecting one configured MCP evidence
// source, so the session can show the operator what is (and is not) connected
// instead of swallowing failures on stderr behind the alt-screen.
type EvidenceStatus struct {
	// Name is the configured server name (e.g. "scanner").
	Name string
	// Transport is the wire used ("http" or "stdio").
	Transport string
	// Tools is the number of tools discovered when the connection succeeded.
	Tools int
	// Err is non-nil when the server failed to connect or serve its tools.
	Err error
}

// OK reports whether the source connected and served at least its tool list.
func (s EvidenceStatus) OK() bool { return s.Err == nil }

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

// ServerConfig describes how to reach one MCP server. Transport selects the
// wire: TransportHTTP (the default) uses URL/APIKey; TransportStdio uses
// Command/Args/Env to launch a local subprocess.
type ServerConfig struct {
	// Name namespaces the server's tools inside vala (e.g. "scanner").
	Name string
	// Transport is "http" (default) or "stdio".
	Transport string

	// URL is the streamable-HTTP endpoint (TransportHTTP).
	URL string
	// APIKey, if set, is sent as an "Authorization: Bearer <key>" header
	// (TransportHTTP).
	APIKey string
	// OAuth, when true, authorizes the server with the MCP OAuth flow (browser
	// sign-in + dynamic client registration), caching tokens out of band. Used by
	// servers like Wiz that have no static API key (TransportHTTP).
	OAuth bool

	// Command is the executable to launch for a local server (TransportStdio).
	Command string
	// Args are the command's arguments (TransportStdio).
	Args []string
	// Env holds already-resolved environment variables (name->value) to set on
	// the subprocess, layered over the operator's environment (TransportStdio).
	Env map[string]string
}

// Connect dials an MCP server over the configured transport and returns a live
// session. Streamable HTTP is the default; "stdio" launches a local subprocess.
func Connect(ctx context.Context, cfg ServerConfig) (Session, error) {
	transport, err := clientTransport(cfg)
	if err != nil {
		return nil, err
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "vala", Version: "0.1.0"}, nil)
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to mcp server %q: %w", cfg.Name, err)
	}
	return &session{cs: cs, name: cfg.Name}, nil
}

// clientTransport builds the SDK transport for a server config.
func clientTransport(cfg ServerConfig) (sdk.Transport, error) {
	switch cfg.Transport {
	case TransportStdio:
		if cfg.Command == "" {
			return nil, fmt.Errorf("mcp server %q: no command configured for stdio transport", cfg.Name)
		}
		cmd := exec.Command(cfg.Command, cfg.Args...)
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		return &sdk.CommandTransport{Command: cmd}, nil
	case TransportHTTP, "":
		if cfg.URL == "" {
			return nil, fmt.Errorf("mcp server %q: no URL configured", cfg.Name)
		}
		t := &sdk.StreamableClientTransport{Endpoint: cfg.URL, HTTPClient: http.DefaultClient}
		switch {
		case cfg.OAuth:
			// Browser sign-in with token caching; the handler sets the auth header
			// and recovers from the initial 401.
			t.OAuthHandler = newOAuthHandler(cfg.Name, defaultTokenStore())
		case cfg.APIKey != "":
			t.HTTPClient = &http.Client{Transport: &bearerTransport{key: cfg.APIKey, base: http.DefaultTransport}}
		}
		return t, nil
	default:
		return nil, fmt.Errorf("mcp server %q: unknown transport %q", cfg.Name, cfg.Transport)
	}
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
