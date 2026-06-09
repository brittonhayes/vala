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

//go:embed recall.md
var recallDescription string

// Recall reads vala's Notion brain back: it searches prior hunts, intel,
// detections, and the backlog so the agent can check what is already known
// before opening new work. It is the dedup/recall move that opens the hunt loop
// and makes each hunt compound on the last instead of repeating settled ground.
type Recall struct{ RC *RunContext }

func (t *Recall) Name() string        { return "recall" }
func (t *Recall) Description() string { return recallDescription }
func (t *Recall) ReadOnly() bool      { return true }

// recallScopes maps each recall scope to its brain database and the row fields
// worth surfacing in a compact result line.
var recallScopes = []struct {
	name   string
	db     string
	fields []string
}{
	{"hunts", brain.DBHunts, []string{"question", "status", "behavior"}},
	{"intel", brain.DBIntel, []string{"kind", "value", "confidence"}},
	{"detections", brain.DBDetections, []string{"title", "status", "path"}},
	{"backlog", brain.DBBacklog, []string{"hypothesis", "status", "priority"}},
}

func (t *Recall) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"query": map[string]any{"type": "string", "description": "Free-text to match against prior artifacts: a behavior, MITRE technique, entity, or keyword. Empty lists the most recent."},
			"scope": map[string]any{"type": "string", "enum": []string{"all", "hunts", "intel", "detections", "backlog"}, "description": "Which part of the brain to search. Defaults to all."},
			"limit": map[string]any{"type": "integer", "description": "Max results per scope (default 5)."},
		},
		Required: []string{"query"},
	}
}

func (t *Recall) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Query string `json:"query"`
		Scope string `json:"scope"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if t.RC == nil || t.RC.Brain == nil {
		return tool.Errorf("no brain configured to recall from"), nil
	}
	if in.Scope == "" {
		in.Scope = "all"
	}
	if in.Limit <= 0 {
		in.Limit = 5
	}

	var b strings.Builder
	total := 0
	for _, s := range recallScopes {
		if in.Scope != "all" && in.Scope != s.name {
			continue
		}
		rows, err := t.RC.Brain.Recall(ctx, s.db, in.Query, in.Limit)
		if err != nil {
			fmt.Fprintf(&b, "%s: recall failed: %v\n\n", s.name, err)
			continue
		}
		if len(rows) == 0 {
			continue
		}
		total += len(rows)
		fmt.Fprintf(&b, "## %s (%d)\n", s.name, len(rows))
		for _, r := range rows {
			fmt.Fprintf(&b, "- %s — %s\n", r.ID, summarizeRow(r, s.fields))
		}
		b.WriteString("\n")
	}

	if total == 0 {
		subject := in.Scope
		if subject == "all" {
			subject = "artifacts"
		}
		if in.Query == "" {
			return tool.Text(fmt.Sprintf("The brain has no %s yet.", subject)), nil
		}
		return tool.Text(fmt.Sprintf("No prior %s match %q — this looks like new ground.", subject, in.Query)), nil
	}
	return tool.Text(strings.TrimSpace(b.String())), nil
}

// summarizeRow renders the salient fields of a brain row as a compact line.
func summarizeRow(r brain.Row, fields []string) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		if v, ok := r.Props[f]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				parts = append(parts, s)
			}
		}
	}
	if len(parts) == 0 {
		return "(no summary fields)"
	}
	return strings.Join(parts, " · ")
}
