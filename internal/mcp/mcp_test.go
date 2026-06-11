package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMain doubles the test binary as a tiny MCP server: when VALA_MCP_TEST_SERVER
// is set it serves one read-only "ping" tool over stdio and exits, so the stdio
// Connect path can be exercised end-to-end without an external binary.
func TestMain(m *testing.M) {
	if os.Getenv("VALA_MCP_TEST_SERVER") == "1" {
		runTestServer()
		return
	}
	os.Exit(m.Run())
}

func runTestServer() {
	s := sdk.NewServer(&sdk.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	sdk.AddTool(s, &sdk.Tool{
		Name:        "ping",
		Description: "returns pong",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *sdk.CallToolRequest, in struct{}) (*sdk.CallToolResult, any, error) {
		echo := os.Getenv("VALA_MCP_ECHO")
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "pong:" + echo}}}, nil, nil
	})
	_ = s.Run(context.Background(), &sdk.StdioTransport{})
}

func TestConnectStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := Connect(ctx, ServerConfig{
		Name:      "test",
		Transport: TransportStdio,
		Command:   os.Args[0],
		Env: map[string]string{
			"VALA_MCP_TEST_SERVER": "1",
			"VALA_MCP_ECHO":        "hello",
		},
	})
	if err != nil {
		t.Fatalf("Connect over stdio: %v", err)
	}
	defer sess.Close()

	tools, err := sess.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "ping" {
		t.Fatalf("expected one ping tool, got %+v", tools)
	}
	if !tools[0].ReadOnly {
		t.Errorf("ping should reflect the server's readOnlyHint")
	}

	res, err := sess.CallTool(ctx, "ping", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("ping returned a tool error: %q", res.Text)
	}
	if res.Text != "pong:hello" {
		t.Errorf("expected the subprocess env to reach the tool, got %q", res.Text)
	}
}

func TestConnectStdioRequiresCommand(t *testing.T) {
	_, err := Connect(context.Background(), ServerConfig{Name: "x", Transport: TransportStdio})
	if err == nil {
		t.Fatal("expected an error when no command is configured for stdio")
	}
}

func TestConnectHTTPRequiresURL(t *testing.T) {
	_, err := Connect(context.Background(), ServerConfig{Name: "x"})
	if err == nil {
		t.Fatal("expected an error when no URL is configured for http")
	}
}

func TestConnectUnknownTransport(t *testing.T) {
	_, err := Connect(context.Background(), ServerConfig{Name: "x", Transport: "carrier-pigeon", URL: "http://x"})
	if err == nil {
		t.Fatal("expected an error for an unknown transport")
	}
}
