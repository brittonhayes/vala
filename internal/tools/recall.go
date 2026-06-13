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
	{"coverage", brain.DBCoverage, []string{"technique", "status", "fidelity"}},
}

func (t *Recall) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"query": map[string]any{"type": "string", "description": "Free-text to match against prior artifacts: a behavior, MITRE technique, entity, or keyword. Empty lists the most recent."},
			"scope": map[string]any{"type": "string", "enum": []string{"all", "hunts", "intel", "detections", "backlog", "coverage"}, "description": "Which part of the brain to search. Defaults to all. Use 'coverage' to surface thin/uncovered ATT&CK techniques when scoping the next hunt."},
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

	// With a search backend (e.g. a Notion MCP server) recall is one
	// relevance-ranked, full-text search over the brain rather than a
	// per-database scan, so a single call replaces the scope loop. An empty query
	// still uses the loop below — it means "list the most recent" per scope.
	if in.Query != "" && t.RC.Brain.HasSearch() {
		return t.runSearch(ctx, in.Scope, in.Query, in.Limit)
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

// runSearch answers recall through the brain's search backend in a single call.
// Scope picks the logical database to search (empty for the whole brain); the
// loose, relevance-ranked rows are rendered with a generic summarizer since a
// search backend returns titles/snippets rather than schema-shaped props.
func (t *Recall) runSearch(ctx context.Context, scope, query string, limit int) (tool.Result, error) {
	db := ""
	if scope != "all" {
		db = scopeDB(scope)
	}
	rows, err := t.RC.Brain.Recall(ctx, db, query, limit)
	if err != nil {
		return tool.Errorf("recall search failed: %v", err), nil
	}
	if len(rows) == 0 {
		subject := scope
		if subject == "all" {
			subject = "artifacts"
		}
		return tool.Text(fmt.Sprintf("No prior %s match %q — this looks like new ground.", subject, query)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n", scope, len(rows))
	for _, r := range rows {
		line := summarizeSearch(r)
		if r.ID != "" {
			fmt.Fprintf(&b, "- %s — %s\n", r.ID, line)
		} else {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	return tool.Text(strings.TrimSpace(b.String())), nil
}

// scopeDB maps a recall scope name to its brain database (empty if unknown).
func scopeDB(scope string) string {
	for _, s := range recallScopes {
		if s.name == scope {
			return s.db
		}
	}
	return ""
}

// summarizeRow renders the salient fields of a brain row as a compact line. When
// none of the scope's typed fields are present (e.g. a loose search result) it
// falls back to the generic title/snippet/url shape a search backend returns.
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
		return summarizeSearch(r)
	}
	return strings.Join(parts, " · ")
}

// summarizeSearch renders a loose search row: a title (or any text), trimmed to
// a single readable line, with a URL appended when present.
func summarizeSearch(r brain.Row) string {
	var lead string
	for _, f := range []string{"title", "name", "snippet", "preview", "text"} {
		if v, ok := r.Props[f]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				lead = oneLine(s, 160)
				break
			}
		}
	}
	if lead == "" {
		return "(no summary)"
	}
	if v, ok := r.Props["url"]; ok {
		if u := strings.TrimSpace(fmt.Sprint(v)); u != "" {
			return lead + " — " + u
		}
	}
	return lead
}

// oneLine collapses whitespace and truncates to max runes for a compact line.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
