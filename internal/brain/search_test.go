package brain

import (
	"context"
	"errors"
	"testing"
)

// searchMem is a Mem that also implements Searcher, recording the queries it is
// asked and returning canned rows (or an error) so Recall's backend selection
// can be exercised deterministically.
type searchMem struct {
	*Mem
	enabled bool
	queries []string
	rows    []Row
	err     error
}

func (s *searchMem) SearchEnabled() bool { return s.enabled }

func (s *searchMem) Search(_ context.Context, _, query string, _ int) ([]Row, error) {
	s.queries = append(s.queries, query)
	return s.rows, s.err
}

func TestRecallPrefersSearchBackend(t *testing.T) {
	ctx := context.Background()
	sm := &searchMem{Mem: NewMem(), enabled: true, rows: []Row{{ID: "x", DB: DBHunts, Props: map[string]any{"title": "from search"}}}}
	bc := New(sm)

	// A row exists in the store that a window scan would surface, distinct from
	// what the search backend returns — so we can tell which path answered.
	if _, err := sm.CreateRow(ctx, DBHunts, map[string]any{"behavior": "DeleteDetector"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Non-empty query goes through the search backend.
	got, err := bc.Recall(ctx, DBHunts, "guardduty", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(got) != 1 || got[0].ID != "x" {
		t.Fatalf("expected the search row, got %+v", got)
	}
	if len(sm.queries) != 1 || sm.queries[0] != "guardduty" {
		t.Fatalf("expected one search for %q, got %v", "guardduty", sm.queries)
	}
	if !bc.HasSearch() {
		t.Fatal("HasSearch should be true when the backend is enabled")
	}
}

func TestRecallEmptyQuerySkipsSearch(t *testing.T) {
	ctx := context.Background()
	sm := &searchMem{Mem: NewMem(), enabled: true}
	bc := New(sm)
	if _, err := sm.CreateRow(ctx, DBHunts, map[string]any{"behavior": "x"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Empty query means "list the most recent" — the window scan, not search.
	got, err := bc.Recall(ctx, DBHunts, "", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected the window-scan row, got %d", len(got))
	}
	if len(sm.queries) != 0 {
		t.Fatalf("search should not be called for an empty query, got %v", sm.queries)
	}
}

func TestRecallFallsBackWhenSearchFails(t *testing.T) {
	ctx := context.Background()
	sm := &searchMem{Mem: NewMem(), enabled: true, err: errors.New("notion down")}
	bc := New(sm)
	if _, err := sm.CreateRow(ctx, DBHunts, map[string]any{"behavior": "DeleteDetector"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Search errors, so recall degrades to the window scan rather than failing.
	got, err := bc.Recall(ctx, DBHunts, "DeleteDetector", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(got) != 1 || got[0].Props["behavior"] != "DeleteDetector" {
		t.Fatalf("expected window-scan fallback row, got %+v", got)
	}
}

func TestHasSearchFalseWithoutBackend(t *testing.T) {
	// Plain Mem does not implement Searcher; an NTN with no hook reports false.
	if New(NewMem()).HasSearch() {
		t.Fatal("Mem-backed brain should report no search backend")
	}
	if New(&NTN{}).HasSearch() {
		t.Fatal("NTN without a SearchFn should report no search backend")
	}
	if !New(&NTN{SearchFn: func(context.Context, string, string, int) ([]Row, error) { return nil, nil }}).HasSearch() {
		t.Fatal("NTN with a SearchFn should report a search backend")
	}
}
