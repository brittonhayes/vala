package mcp

import (
	"context"
	"encoding/json"
)

// FakeSession is an in-memory Session for tests and the offline harness. It
// exposes a fixed tool list and dispatches calls to OnCall, so vala's MCP
// integration can be exercised deterministically without a live server.
type FakeSession struct {
	ServerName string
	Tools      []ToolDesc
	// OnCall handles a tools/call. If nil, every call returns empty text.
	OnCall func(name string, args json.RawMessage) (CallResult, error)
}

func (f *FakeSession) Name() string { return f.ServerName }

func (f *FakeSession) ListTools(context.Context) ([]ToolDesc, error) { return f.Tools, nil }

func (f *FakeSession) CallTool(_ context.Context, name string, args json.RawMessage) (CallResult, error) {
	if f.OnCall == nil {
		return CallResult{}, nil
	}
	return f.OnCall(name, args)
}

func (f *FakeSession) Close() error { return nil }
