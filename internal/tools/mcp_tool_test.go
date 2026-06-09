package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/mcp"
	"github.com/brittonhayes/vala/internal/tool"
)

// byName indexes adapted tools by their namespaced name.
func byName(tools []tool.Tool) map[string]tool.Tool {
	m := make(map[string]tool.Tool, len(tools))
	for _, tl := range tools {
		m[tl.Name()] = tl
	}
	return m
}

// fakeScanner returns a FakeSession exposing one read-only query tool and one
// non-read-only tool, so the adapter's namespacing and read-only handling can be
// asserted without a live server.
func fakeScanner() *mcp.FakeSession {
	return &mcp.FakeSession{
		ServerName: "scanner",
		Tools: []mcp.ToolDesc{
			{
				Name:        "execute_query",
				Description: "Run an ad-hoc query.",
				Properties:  map[string]any{"query": map[string]any{"type": "string"}},
				Required:    []string{"query"},
				ReadOnly:    true,
			},
			{Name: "write_thing", Description: "mutates", ReadOnly: false},
		},
		OnCall: func(name string, args json.RawMessage) (mcp.CallResult, error) {
			return mcp.CallResult{Text: name + ":" + string(args)}, nil
		},
	}
}

func TestMCPToolsFromNamespacesAndClassifies(t *testing.T) {
	tools, readOnly, err := MCPToolsFrom(context.Background(), fakeScanner())
	if err != nil {
		t.Fatalf("MCPToolsFrom: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 adapted tools, got %d", len(tools))
	}
	// Tools are namespaced under the server name.
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name()] = true
	}
	if !names["scanner_execute_query"] || !names["scanner_write_thing"] {
		t.Fatalf("tools not namespaced under server: %v", names)
	}
	// Only the read-only tool is reported for read classification.
	if len(readOnly) != 1 || readOnly[0] != "scanner_execute_query" {
		t.Fatalf("read-only names = %v, want [scanner_execute_query]", readOnly)
	}
}

func TestMCPToolRunForwardsCall(t *testing.T) {
	tools, _, err := MCPToolsFrom(context.Background(), fakeScanner())
	if err != nil {
		t.Fatalf("MCPToolsFrom: %v", err)
	}
	query, ok := byName(tools)["scanner_execute_query"]
	if !ok {
		t.Fatal("scanner_execute_query adapter not found")
	}
	if !query.ReadOnly() {
		t.Fatal("scanner_execute_query should be read-only")
	}
	if _, ok := query.Schema().Properties["query"]; !ok {
		t.Fatalf("schema did not carry through: %+v", query.Schema().Properties)
	}
	res := run(t, query, map[string]any{"query": "StopLogging"})
	if res.IsError {
		t.Fatalf("call errored: %s", res.Content)
	}
	// The adapter forwards the server-side name and raw args to the session.
	if !strings.HasPrefix(res.Content, "execute_query:") || !strings.Contains(res.Content, "StopLogging") {
		t.Fatalf("unexpected forwarded result: %q", res.Content)
	}
}
