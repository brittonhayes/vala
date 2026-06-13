package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/mcp"
)

// NotionSearchHook builds a brain search hook backed by a Notion MCP server. It
// discovers the server's search tool once, then answers each recall by calling
// that tool with the free-text query and parsing the response into loose rows.
// Wiring it into a brain.NTN's SearchFn routes recall through Notion's own
// relevance-ranked, full-text search instead of the client-side window scan —
// the more capable, agent-natural search the operator chose.
//
// The hook is intentionally loose: it passes only the query (no per-database
// structured filter), and returns whatever the server ranks highest, mapped to
// rows that recall renders for context. Recall stays the single curated read
// surface over the brain — the raw Notion MCP tools are not exposed to the agent.
func NotionSearchHook(ctx context.Context, sess mcp.Session) (func(ctx context.Context, db, query string, limit int) ([]brain.Row, error), error) {
	descs, err := sess.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("list notion mcp tools: %w", err)
	}
	tool, ok := pickSearchTool(descs)
	if !ok {
		return nil, fmt.Errorf("notion mcp server %q exposes no search tool", sess.Name())
	}

	return func(ctx context.Context, db, query string, limit int) ([]brain.Row, error) {
		args, err := json.Marshal(map[string]any{"query": query})
		if err != nil {
			return nil, err
		}
		res, err := sess.CallTool(ctx, tool.Name, args)
		if err != nil {
			return nil, fmt.Errorf("notion search: %w", err)
		}
		if res.IsError {
			return nil, fmt.Errorf("notion search: %s", res.Text)
		}
		return parseSearchResults(db, res.Text, limit), nil
	}, nil
}

// pickSearchTool finds the server's search tool by name. Hosted Notion MCP
// servers name it "search" or "notion-search"; matching on the substring keeps
// the hook robust to either.
func pickSearchTool(descs []mcp.ToolDesc) (mcp.ToolDesc, bool) {
	for _, d := range descs {
		if strings.Contains(strings.ToLower(d.Name), "search") {
			return d, true
		}
	}
	return mcp.ToolDesc{}, false
}

// parseSearchResults turns a search tool's flattened output into loose rows. It
// tolerates a JSON object with a "results" array, a bare JSON array, or plain
// prose — the realistic case for a server that returns markdown — which becomes
// a single contextual row so recall always has something to show.
func parseSearchResults(db, text string, limit int) []brain.Row {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if rows := parseJSONResults(db, text); rows != nil {
		return capRows(rows, limit)
	}
	return []brain.Row{{DB: db, Props: map[string]any{"text": text}}}
}

// parseJSONResults extracts rows from structured search JSON, pulling the common
// id/title/url/snippet keys into props. It returns nil when text is not the
// expected JSON shape, so the caller can fall back to treating it as prose.
func parseJSONResults(db, text string) []brain.Row {
	var obj struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &obj); err == nil && obj.Results != nil {
		return rowsFromResults(db, obj.Results)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(text), &arr); err == nil {
		return rowsFromResults(db, arr)
	}
	return nil
}

func rowsFromResults(db string, ms []map[string]any) []brain.Row {
	out := make([]brain.Row, 0, len(ms))
	for _, m := range ms {
		r := brain.Row{DB: db, Props: map[string]any{}}
		if id, ok := m["id"].(string); ok {
			r.ID = id
		}
		for _, k := range []string{"title", "url", "snippet", "text", "preview"} {
			if v, ok := m[k]; ok {
				r.Props[k] = v
			}
		}
		// Nothing recognized: keep the raw object so no result is silently dropped.
		if len(r.Props) == 0 {
			r.Props = m
		}
		out = append(out, r)
	}
	return out
}

func capRows(rows []brain.Row, limit int) []brain.Row {
	if limit > 0 && len(rows) > limit {
		return rows[:limit]
	}
	return rows
}
