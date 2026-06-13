package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/brittonhayes/vala/internal/mcp"
)

func notionFake(onCall func(name string, args json.RawMessage) (mcp.CallResult, error), tools ...mcp.ToolDesc) *mcp.FakeSession {
	return &mcp.FakeSession{ServerName: "notion", Tools: tools, OnCall: onCall}
}

func TestNotionSearchHookParsesJSONResults(t *testing.T) {
	ctx := context.Background()
	var gotArgs string
	sess := notionFake(
		func(name string, args json.RawMessage) (mcp.CallResult, error) {
			gotArgs = string(args)
			return mcp.CallResult{Text: `{"results":[{"id":"p1","title":"GuardDuty disabled","url":"https://n/p1"},{"id":"p2","title":"S3 exposure"}]}`}, nil
		},
		mcp.ToolDesc{Name: "notion-search"},
	)
	hook, err := NotionSearchHook(ctx, sess)
	if err != nil {
		t.Fatalf("NotionSearchHook: %v", err)
	}
	rows, err := hook(ctx, "hunts", "guardduty", 5)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].ID != "p1" || rows[0].Props["title"] != "GuardDuty disabled" || rows[0].Props["url"] != "https://n/p1" {
		t.Fatalf("first row not mapped: %+v", rows[0])
	}
	if gotArgs != `{"query":"guardduty"}` {
		t.Fatalf("expected loose query-only args, got %s", gotArgs)
	}
}

func TestNotionSearchHookLimitCaps(t *testing.T) {
	ctx := context.Background()
	sess := notionFake(
		func(string, json.RawMessage) (mcp.CallResult, error) {
			return mcp.CallResult{Text: `[{"id":"a"},{"id":"b"},{"id":"c"}]`}, nil
		},
		mcp.ToolDesc{Name: "search"},
	)
	hook, _ := NotionSearchHook(ctx, sess)
	rows, err := hook(ctx, "", "q", 2)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected limit to cap at 2, got %d", len(rows))
	}
}

func TestNotionSearchHookProseFallback(t *testing.T) {
	ctx := context.Background()
	sess := notionFake(
		func(string, json.RawMessage) (mcp.CallResult, error) {
			return mcp.CallResult{Text: "  Found a prior hunt on DeleteDetector.  "}, nil
		},
		mcp.ToolDesc{Name: "notion-search"},
	)
	hook, _ := NotionSearchHook(ctx, sess)
	rows, err := hook(ctx, "hunts", "q", 5)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if len(rows) != 1 || rows[0].Props["text"] != "Found a prior hunt on DeleteDetector." {
		t.Fatalf("expected one trimmed prose row, got %+v", rows)
	}
}

func TestNotionSearchHookToolError(t *testing.T) {
	ctx := context.Background()
	sess := notionFake(
		func(string, json.RawMessage) (mcp.CallResult, error) {
			return mcp.CallResult{Text: "rate limited", IsError: true}, nil
		},
		mcp.ToolDesc{Name: "search"},
	)
	hook, _ := NotionSearchHook(ctx, sess)
	if _, err := hook(ctx, "", "q", 5); err == nil {
		t.Fatal("expected an error when the search tool reports IsError")
	}
}

func TestNotionSearchHookNoSearchTool(t *testing.T) {
	ctx := context.Background()
	sess := notionFake(nil, mcp.ToolDesc{Name: "notion-fetch"}, mcp.ToolDesc{Name: "create-pages"})
	if _, err := NotionSearchHook(ctx, sess); err == nil {
		t.Fatal("expected an error when no search tool is exposed")
	}
}
