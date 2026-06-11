package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/mcp"
	"github.com/brittonhayes/vala/internal/tool"
)

// MCPTool adapts a single tool exposed by a remote MCP server into vala's tool
// interface. The agent calls it like any native tool; the call is forwarded to
// the server over the shared session. Read-only MCP tools (e.g. Scanner's
// query/discovery tools) bypass the permission gate like other observers; any
// tool the server does not mark read-only is gated.
type MCPTool struct {
	session mcp.Session
	desc    mcp.ToolDesc
	name    string // namespaced, gate/policy-facing name
}

func (t *MCPTool) Name() string        { return t.name }
func (t *MCPTool) Description() string { return t.desc.Description }
func (t *MCPTool) ReadOnly() bool      { return t.desc.ReadOnly }
func (t *MCPTool) Schema() tool.Schema {
	props := t.desc.Properties
	if props == nil {
		props = map[string]any{}
	}
	return tool.Schema{Properties: props, Required: t.desc.Required}
}

func (t *MCPTool) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	res, err := t.session.CallTool(ctx, t.desc.Name, input)
	if err != nil {
		return tool.Errorf("%s: %v", t.name, err), nil
	}
	if res.IsError {
		return tool.Result{Content: res.Text, IsError: true}, nil
	}
	return tool.Text(res.Text), nil
}

// MCPToolsFrom discovers the tools a connected session exposes and wraps each as
// an MCPTool. It returns the adapter tools plus the namespaced names of the
// read-only ones, so the caller can classify them as read in the policy set
// (otherwise they fail closed to action_execute and never appear during
// investigation or a hunt).
func MCPToolsFrom(ctx context.Context, session mcp.Session) ([]tool.Tool, []string, error) {
	descs, err := session.ListTools(ctx)
	if err != nil {
		return nil, nil, err
	}
	var tools []tool.Tool
	var readOnly []string
	for _, d := range descs {
		name := mcpToolName(session.Name(), d.Name)
		tools = append(tools, &MCPTool{session: session, desc: d, name: name})
		if d.ReadOnly {
			readOnly = append(readOnly, name)
		}
	}
	return tools, readOnly, nil
}

// ConnectEvidence dials one configured MCP server, discovers its tools, and
// reports the outcome as an EvidenceStatus. A connect or discovery failure is
// returned in the status (not as an error) so a single bad source never blocks
// the rest — the caller surfaces it to the operator. On success the returned
// session lives for the process; on failure it is closed before returning.
func ConnectEvidence(ctx context.Context, cfg mcp.ServerConfig) ([]tool.Tool, mcp.EvidenceStatus) {
	transport := cfg.Transport
	if transport == "" {
		transport = mcp.TransportHTTP
	}
	status := mcp.EvidenceStatus{Name: cfg.Name, Transport: transport}

	sess, err := mcp.Connect(ctx, cfg)
	if err != nil {
		status.Err = err
		return nil, status
	}
	ts, _, err := MCPToolsFrom(ctx, sess)
	if err != nil {
		status.Err = err
		_ = sess.Close()
		return nil, status
	}
	status.Tools = len(ts)
	return ts, status
}

// mcpToolName namespaces a server-side tool name under its server and sanitizes
// it to the ^[a-zA-Z0-9_-]+$ shape the Anthropic API requires for tool names.
// e.g. server "scanner" + tool "execute_query" -> "scanner_execute_query".
func mcpToolName(server, tool string) string {
	raw := fmt.Sprintf("%s_%s", server, tool)
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
