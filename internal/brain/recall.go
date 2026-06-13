package brain

import "context"

// Recall returns up to limit rows in the named logical database whose contents
// match the free-text query (an empty query matches everything). It is the read
// counterpart to the brain's writers: the agent calls it to check what has
// already been hunted, which intel exists, and whether a detection already
// covers a behavior before opening new work — the move that turns the brain from
// a write-only ledger into the memory each hunt builds on.
func (c *Client) Recall(ctx context.Context, db, query string, limit int) ([]Row, error) {
	if limit <= 0 {
		limit = 5
	}
	// A configured search backend (e.g. a Notion MCP server) answers free-text
	// recall with relevance-ranked search rather than the window scan. An empty
	// query still uses Query — it means "list the most recent", which the scan
	// does directly. On search failure we fall through to the scan so recall
	// degrades rather than going dark.
	if s, ok := c.n.(Searcher); ok && s.SearchEnabled() && query != "" {
		if rows, err := s.Search(ctx, db, query, limit); err == nil {
			return rows, nil
		}
	}
	return c.n.Query(ctx, c.dbName(db), query, limit)
}
