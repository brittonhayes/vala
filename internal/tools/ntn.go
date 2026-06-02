package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed ntn.md
var ntnDescription string

// NTN wraps the official Notion `ntn` CLI. Not read-only: Notion writes are
// possible, so calls are permission-gated.
type NTN struct {
	// Bin is the ntn executable name or path (default "ntn").
	Bin string
	Dir string
}

func (n *NTN) Name() string        { return "ntn" }
func (n *NTN) Description() string { return ntnDescription }
func (n *NTN) ReadOnly() bool      { return false }

func (n *NTN) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"args": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Arguments passed to the ntn CLI, e.g. [\"pages\", \"list\"].",
			},
		},
		Required: []string{"args"},
	}
}

func (n *NTN) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if len(in.Args) == 0 {
		return tool.Errorf("args is required (e.g. [\"pages\", \"--help\"])"), nil
	}

	bin := n.Bin
	if bin == "" {
		bin = "ntn"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return tool.Errorf("the ntn CLI is not installed or not on PATH (%v)", err), nil
	}

	cmd := exec.CommandContext(ctx, bin, in.Args...)
	cmd.Dir = n.Dir
	out, err := cmd.CombinedOutput()
	body := truncate(string(out))
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("ntn %s failed: %v\n%s",
			strings.Join(in.Args, " "), err, body), IsError: true}, nil
	}
	if body == "" {
		body = "(no output)"
	}
	return tool.Text(body), nil
}
