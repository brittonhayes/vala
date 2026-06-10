package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed remember.md
var rememberDescription string

// Remember records a durable environment fact as a shared brain memory, stamped
// with the operator who learned it and linked to the active hunt. Because it
// writes through the brain, a team pointed at the same Notion workspace shares
// memories: what one hunter learns primes everyone's next session. Not
// read-only; permission-gated.
type Remember struct{ RC *RunContext }

func (r *Remember) Name() string        { return "remember" }
func (r *Remember) Description() string { return rememberDescription }
func (r *Remember) ReadOnly() bool      { return false }

func (r *Remember) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"fact": map[string]any{
				"type":        "string",
				"description": "A durable fact about this environment worth recalling in every future session — a log-source location, a known-good baseline, a naming convention, a crown-jewel system. One specific sentence; never a secret.",
			},
		},
		Required: []string{"fact"},
	}
}

type rememberInput struct {
	Fact string `json:"fact"`
}

func (r *Remember) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in rememberInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	fact := strings.TrimSpace(in.Fact)
	if fact == "" {
		return tool.Errorf("fact is required"), nil
	}
	if r.RC == nil || r.RC.Brain == nil {
		return tool.Errorf("no brain configured to remember into"), nil
	}

	if _, err := r.RC.Brain.Remember(ctx, brain.Memory{
		Fact:   fact,
		Author: r.RC.Author,
		Hunt:   r.RC.HuntID,
	}); err != nil {
		return tool.Errorf("could not save memory: %v", err), nil
	}

	who := strings.TrimSpace(r.RC.Author)
	if who == "" {
		who = "you"
	}
	return tool.Text(fmt.Sprintf("remembered (as %s), shared to the brain: %s", who, fact)), nil
}
