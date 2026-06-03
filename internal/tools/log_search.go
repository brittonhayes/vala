package tools

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed log_search.md
var logSearchDescription string

// LogSearch is the v1 evidence source: a read-only log query tool. By default it
// searches a newline-delimited JSON log file (<Dir>/logs.jsonl) with a naive
// substring match; tests and the harness inject Search for deterministic
// results. Every result set carries a stable query_id that becomes an Evidence
// pointer.
type LogSearch struct {
	Dir    string
	Search func(query string) []map[string]any // optional override
}

func (t *LogSearch) Name() string        { return "log_search" }
func (t *LogSearch) Description() string { return logSearchDescription }
func (t *LogSearch) ReadOnly() bool      { return true }

func (t *LogSearch) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"query": map[string]any{"type": "string", "description": "Search expression (substring match against log events)."},
			"from":  map[string]any{"type": "string", "description": "Optional start time (RFC3339)."},
			"to":    map[string]any{"type": "string", "description": "Optional end time (RFC3339)."},
		},
		Required: []string{"query"},
	}
}

func (t *LogSearch) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct{ Query, From, To string }
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Query == "" {
		return tool.Errorf("query is required"), nil
	}
	search := t.Search
	if search == nil {
		search = t.fileSearch
	}
	results := search(in.Query)

	queryID := fmt.Sprintf("logq_%x", sha256.Sum256([]byte(in.Query+"|"+in.From+"|"+in.To)))[:14]
	out := map[string]any{
		"query_id": queryID,
		"query":    in.Query,
		"count":    len(results),
		"results":  results,
	}
	body, _ := json.MarshalIndent(out, "", "  ")
	return tool.Text(string(body)), nil
}

// fileSearch reads <Dir>/logs.jsonl and returns events whose JSON contains the
// query as a substring. A missing file yields no results (not an error) so the
// tool is usable out of the box.
func (t *LogSearch) fileSearch(query string) []map[string]any {
	f, err := os.Open(t.Dir + "/logs.jsonl")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if query != "*" && !strings.Contains(line, query) {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err == nil {
			out = append(out, ev)
		}
	}
	return out
}
