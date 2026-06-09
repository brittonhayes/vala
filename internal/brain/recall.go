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
	return c.n.Query(ctx, c.dbName(db), query, limit)
}
