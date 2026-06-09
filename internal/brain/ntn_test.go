package brain

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypedValue(t *testing.T) {
	cases := []struct {
		typ  string
		in   any
		want string // substrings that must appear in the marshaled value
	}{
		{"title", "Hunt A", `"title"`},
		{"title", "Hunt A", `"content":"Hunt A"`},
		{"rich_text", "note", `"rich_text"`},
		{"select", "high", `"select":{"name":"high"}`},
		{"status", "Open", `"status":{"name":"Open"}`},
		{"date", "2026-06-09T00:00:00Z", `"date":{"start":"2026-06-09T00:00:00Z"}`},
		{"checkbox", true, `"checkbox":true`},
		{"url", "https://x", `"url":"https://x"`},
		{"relation", []string{"a", "b"}, `"relation":[{"id":"a"},{"id":"b"}]`},
		{"multi_select", []string{"x"}, `"multi_select":[{"name":"x"}]`},
		// An unknown type must not drop data — it falls back to rich_text.
		{"phone_number", "555", `"rich_text"`},
	}
	for _, c := range cases {
		b, err := json.Marshal(typedValue(c.typ, c.in))
		if err != nil {
			t.Fatalf("marshal %s: %v", c.typ, err)
		}
		if !strings.Contains(string(b), c.want) {
			t.Errorf("typedValue(%q, %v) = %s, want substring %q", c.typ, c.in, b, c.want)
		}
	}
}

func TestToPropertiesSkipsUnknownAndEmpty(t *testing.T) {
	schema := map[string]string{
		"hunt_id":    "title",
		"status":     "status",
		"started_at": "date",
		"hunts":      "relation",
	}
	props := map[string]any{
		"hunt_id":  "did anyone disable GuardDuty?",
		"status":   "Open",
		"behavior": "DeleteDetector", // not in schema -> dropped
		"hunts":    nil,              // nil -> omitted
	}
	got := toProperties(schema, props)
	if _, ok := got["behavior"]; ok {
		t.Error("property not in schema should be dropped")
	}
	if _, ok := got["hunts"]; ok {
		t.Error("nil property should be omitted")
	}
	if _, ok := got["hunt_id"]; !ok {
		t.Error("hunt_id should be present")
	}
	if _, ok := got["started_at"]; ok {
		t.Error("started_at absent from props should not appear")
	}
}

// TestNTNCreateRowTypesProperties drives the real NTN write path against a fake
// `ntn` binary: it asserts CreateRow fetches the data source schema, sends a
// typed `POST /v1/pages` with a data_source_id parent, and parses the new id.
func TestNTNCreateRowTypesProperties(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	tmp := t.TempDir()
	bodyFile := filepath.Join(tmp, "body.json")
	script := filepath.Join(tmp, "ntn")
	src := `#!/bin/sh
case "$1 $2" in
  "api /v1/data_sources/"*)
    echo '{"properties":{"hunt_id":{"type":"title"},"status":{"type":"status"},"started_at":{"type":"date"},"hunts":{"type":"relation"}}}'
    ;;
  "api /v1/pages")
    prev=""
    for a in "$@"; do
      if [ "$prev" = "-d" ]; then printf '%s' "$a" > ` + bodyFile + `; fi
      prev="$a"
    done
    echo '{"id":"page_1","url":"https://notion.so/page_1"}'
    ;;
  *) echo '{}' ;;
esac
`
	if err := os.WriteFile(script, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}

	n := &NTN{Bin: script, Dir: tmp, DBs: DBIDs{Hunts: "ds_hunts"}}
	bc := New(n)
	id, err := bc.OpenHunt(context.Background(), Hunt{Question: "did anyone disable GuardDuty?", Behavior: "DeleteDetector"})
	if err != nil {
		t.Fatalf("OpenHunt: %v", err)
	}
	if id != "page_1" {
		t.Fatalf("CreateRow id = %q, want page_1", id)
	}

	raw, err := os.ReadFile(bodyFile)
	if err != nil {
		t.Fatalf("request body not captured: %v", err)
	}
	var body struct {
		Parent struct {
			Type         string `json:"type"`
			DataSourceID string `json:"data_source_id"`
		} `json:"parent"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse captured body: %v\n%s", err, raw)
	}
	if body.Parent.Type != "data_source_id" || body.Parent.DataSourceID != "ds_hunts" {
		t.Fatalf("parent = %+v, want data_source_id ds_hunts", body.Parent)
	}
	if !strings.Contains(string(body.Properties["hunt_id"]), `"title"`) {
		t.Errorf("hunt_id not typed as title: %s", body.Properties["hunt_id"])
	}
	if s := string(body.Properties["status"]); !strings.Contains(s, `"status"`) || !strings.Contains(s, "Open") {
		t.Errorf("status not typed correctly: %s", s)
	}
	if !strings.Contains(string(body.Properties["started_at"]), `"date"`) {
		t.Errorf("started_at not typed as date: %s", body.Properties["started_at"])
	}
	// A property absent from the schema must not be sent.
	if _, ok := body.Properties["behavior"]; ok {
		t.Error("behavior is not in the schema and must be dropped")
	}
}
