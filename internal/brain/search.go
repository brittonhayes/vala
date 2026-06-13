package brain

import "context"

// Searcher is an optional capability a Notion store implements to answer recall
// with the backend's own relevance-ranked search instead of the brain's
// client-side window scan in Query. A store that implements it and reports
// SearchEnabled is preferred by Recall for non-empty queries; otherwise recall
// falls back to the structured window scan.
//
// Results are best-effort and need NOT carry the store's full typed properties:
// recall reads them for context (what has already been hunted, what intel
// exists), not as structured records, so a search backend may return loose rows
// (a title, a snippet, a URL) rather than the schema-shaped props writers use.
type Searcher interface {
	// SearchEnabled reports whether a search backend is actually wired up. When
	// false, Recall uses the window scan even though the type satisfies Searcher.
	SearchEnabled() bool
	// Search returns up to limit rows matching the free-text query. db is the
	// logical database recall is scoped to (empty for the whole brain); an
	// implementation may narrow to it or search broadly.
	Search(ctx context.Context, db, query string, limit int) ([]Row, error)
}

// SearchEnabled reports whether a search hook has been injected into the store.
func (n *NTN) SearchEnabled() bool { return n.SearchFn != nil }

// Search delegates to the injected SearchFn. It panics-safely degrades: with no
// hook it reports no rows, which Recall treats as a miss and falls back to the
// window scan (callers gate on SearchEnabled first, so this is defensive).
func (n *NTN) Search(ctx context.Context, db, query string, limit int) ([]Row, error) {
	if n.SearchFn == nil {
		return nil, nil
	}
	return n.SearchFn(ctx, db, query, limit)
}

// HasSearch reports whether recall is backed by a search engine rather than the
// client-side window scan. The recall tool uses it to issue a single workspace
// search instead of scanning each database in turn.
func (c *Client) HasSearch() bool {
	s, ok := c.n.(Searcher)
	return ok && s.SearchEnabled()
}
