package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/brittonhayes/vala/internal/brain"
)

// TestSaveNotionPreservesUnrelatedKeys is the core idempotency guarantee: writing
// the provisioned brain IDs must set only the "notion" key and leave every other
// key (model, detections_dir, mcp, …) untouched.
func TestSaveNotionPreservesUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vala.json")
	seed := `{
  "model": "claude-custom",
  "detections_dir": "rules",
  "mcp": [{"name": "scanner", "url": "https://x"}]
}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	ids := brain.DBIDs{Evidence: "ds_evidence", Hunts: "ds_hunts", Intel: "ds_intel", Parent: "page_1"}
	if err := SaveNotion(dir, ids); err != nil {
		t.Fatalf("SaveNotion: %v", err)
	}

	// Unrelated keys must survive, and the new config must still load.
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "claude-custom" {
		t.Errorf("model = %q, want claude-custom (clobbered)", cfg.Model)
	}
	if cfg.DetectionsDir != "rules" {
		t.Errorf("detections_dir = %q, want rules (clobbered)", cfg.DetectionsDir)
	}
	if len(cfg.MCP) != 1 || cfg.MCP[0].Name != "scanner" {
		t.Errorf("mcp dropped: %+v", cfg.MCP)
	}
	if cfg.Notion.Evidence != "ds_evidence" || cfg.Notion.Hunts != "ds_hunts" || cfg.Notion.Parent != "page_1" {
		t.Errorf("notion IDs not written: %+v", cfg.Notion)
	}

	// And the raw file must not have introduced unexpected top-level keys.
	data, _ := os.ReadFile(path)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	for _, want := range []string{"model", "detections_dir", "mcp", "notion"} {
		if _, ok := raw[want]; !ok {
			t.Errorf("key %q missing from saved config", want)
		}
	}
}

// TestSaveNotionCreatesFile covers the clean-checkout path: no .vala.json yet.
func TestSaveNotionCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := SaveNotion(dir, brain.DBIDs{Hunts: "ds_hunts"}); err != nil {
		t.Fatalf("SaveNotion: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notion.Hunts != "ds_hunts" {
		t.Errorf("notion.hunts = %q, want ds_hunts", cfg.Notion.Hunts)
	}
}
