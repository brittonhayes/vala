package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed bash.md
var bashDescription string

const (
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second
	maxToolOutput      = 30000 // bytes returned to the model before truncation
)

// Bash runs shell commands. It is not read-only and is permission-gated.
type Bash struct {
	// Dir is the working directory for commands.
	Dir string
}

func (b *Bash) Name() string        { return "bash" }
func (b *Bash) Description() string { return bashDescription }
func (b *Bash) ReadOnly() bool      { return false }

func (b *Bash) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to run via 'sh -c'.",
			},
			"timeout_s": map[string]any{
				"type":        "integer",
				"description": "Max seconds before the command is killed (default 120, max 600).",
			},
		},
		Required: []string{"command"},
	}
}

type bashInput struct {
	Command  string `json:"command"`
	TimeoutS int    `json:"timeout_s"`
}

func (b *Bash) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if strings.TrimSpace(in.Command) == "" {
		return tool.Errorf("command is required"), nil
	}

	timeout := defaultBashTimeout
	if in.TimeoutS > 0 {
		timeout = time.Duration(in.TimeoutS) * time.Second
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", in.Command)
	cmd.Dir = b.Dir
	out, err := cmd.CombinedOutput()

	body := truncate(string(out))
	if runCtx.Err() == context.DeadlineExceeded {
		return tool.Errorf("command timed out after %s\n%s", timeout, body), nil
	}
	if err != nil {
		// Surface the failure to the model without aborting the turn.
		return tool.Result{Content: fmt.Sprintf("exit error: %v\n%s", err, body), IsError: true}, nil
	}
	if body == "" {
		body = "(no output)"
	}
	return tool.Text(body), nil
}

// truncate caps tool output so a single command can't blow the context window.
func truncate(s string) string {
	if len(s) <= maxToolOutput {
		return s
	}
	half := maxToolOutput / 2
	return s[:half] + "\n... [truncated " +
		fmt.Sprintf("%d", len(s)-maxToolOutput) + " bytes] ...\n" + s[len(s)-half:]
}
