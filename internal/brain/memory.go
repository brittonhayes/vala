package brain

import (
	"context"
	"fmt"
	"strings"
)

// Memory is a durable operator fact stored as a first-class, shareable brain
// artifact: what a hunter learns about the environment — where a log source
// lives, a known-good baseline, a naming convention, a crown-jewel system.
// Because memory lives in the brain, a team pointed at the same workspace shares
// one another's memories, and each one carries the author who recorded it. It is
// what makes vala a multiplayer hunting system rather than a solo tool.
type Memory struct {
	ID     string `json:"id"`
	Fact   string `json:"fact"`
	Author string `json:"author"`
	Hunt   string `json:"hunt"` // optional: the hunt that taught it
}

// Remember writes a Memory row and returns its ID. It is the write side of the
// brain's shared memory; Memories reads it back.
func (c *Client) Remember(ctx context.Context, m Memory) (string, error) {
	props := map[string]any{
		"memory_id":  m.Fact,
		"fact":       m.Fact,
		"author":     m.Author,
		"created_at": nowRFC3339(),
	}
	if m.Hunt != "" {
		setRelation(props, "hunt", []string{m.Hunt})
	}
	return c.n.CreateRow(ctx, c.dbName(DBMemory), props)
}

// Memories reads shared memory back: up to limit memory rows matching the
// free-text query (empty lists the most recent), newest-relevant first. It is
// what the harness loads into a session so every hunt starts with what the team
// already knows about the environment.
func (c *Client) Memories(ctx context.Context, query string, limit int) ([]Memory, error) {
	rows, err := c.Recall(ctx, DBMemory, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Memory, 0, len(rows))
	for _, r := range rows {
		fact := propText(r.Props["fact"])
		if strings.TrimSpace(fact) == "" {
			continue
		}
		out = append(out, Memory{ID: r.ID, Fact: fact, Author: propText(r.Props["author"])})
	}
	return out, nil
}

// propText extracts plain text from a brain property value across backends: the
// Mem and File stores hold flat scalars, while a row read back from Notion holds
// the API's typed property objects (rich_text/title arrays, select/status names,
// a date start). It returns "" for anything it cannot interpret as text.
func propText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case map[string]any:
		for _, key := range []string{"title", "rich_text"} {
			if arr, ok := t[key].([]any); ok {
				return joinRichText(arr)
			}
		}
		for _, key := range []string{"select", "status"} {
			if m, ok := t[key].(map[string]any); ok {
				if name, ok := m["name"].(string); ok {
					return name
				}
			}
		}
		if d, ok := t["date"].(map[string]any); ok {
			if s, ok := d["start"].(string); ok {
				return s
			}
		}
		if s, ok := t["plain_text"].(string); ok {
			return s
		}
		return ""
	default:
		return fmt.Sprint(t)
	}
}

// joinRichText concatenates the text of a Notion rich-text / title array,
// tolerating both the read shape (plain_text) and the write shape (text.content).
func joinRichText(arr []any) string {
	var b strings.Builder
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if s, ok := m["plain_text"].(string); ok {
			b.WriteString(s)
			continue
		}
		if txt, ok := m["text"].(map[string]any); ok {
			if c, ok := txt["content"].(string); ok {
				b.WriteString(c)
			}
		}
	}
	return b.String()
}
