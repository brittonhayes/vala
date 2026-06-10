package brain

import (
	"context"
	"testing"
)

// TestRememberAndRecallMemory is the round trip that makes memory multiplayer:
// a fact written with an author is read back through Memories, linked to the
// hunt that taught it.
func TestRememberAndRecallMemory(t *testing.T) {
	ctx := context.Background()
	c := New(NewMem())

	if _, err := c.Remember(ctx, Memory{Fact: "auth logs live in Okta", Author: "alice", Hunt: "hunts_0001"}); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if _, err := c.Remember(ctx, Memory{Fact: "svc-deploy rotates keys nightly", Author: "bob"}); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	all, err := c.Memories(ctx, "", 5)
	if err != nil {
		t.Fatalf("Memories: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 memories, got %d", len(all))
	}

	hit, err := c.Memories(ctx, "Okta", 5)
	if err != nil {
		t.Fatalf("Memories(query): %v", err)
	}
	if len(hit) != 1 || hit[0].Author != "alice" || hit[0].Fact != "auth logs live in Okta" {
		t.Fatalf("query recall wrong: %+v", hit)
	}
}

// TestPropTextAcrossShapes checks the extractor handles both flat scalars (Mem /
// File) and the typed property objects a Notion read returns.
func TestPropTextAcrossShapes(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"flat string", "hello", "hello"},
		{"rich_text read shape", map[string]any{
			"type":      "rich_text",
			"rich_text": []any{map[string]any{"plain_text": "from notion"}},
		}, "from notion"},
		{"title write shape", map[string]any{
			"title": []any{map[string]any{"text": map[string]any{"content": "a title"}}},
		}, "a title"},
		{"select name", map[string]any{"select": map[string]any{"name": "indicator"}}, "indicator"},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		if got := propText(tc.in); got != tc.want {
			t.Errorf("%s: propText = %q, want %q", tc.name, got, tc.want)
		}
	}
}
