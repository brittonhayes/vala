package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/config"
)

// TestBrainConfigured drives the unconfigured-brain predicate that gates the
// first-run prompt and brainStore's backend choice: any of hunts, intel, or
// evidence being set means the brain persists.
func TestBrainConfigured(t *testing.T) {
	cases := []struct {
		name string
		ids  brain.DBIDs
		want bool
	}{
		{"empty", brain.DBIDs{}, false},
		{"only parent set", brain.DBIDs{Parent: "page_1"}, false},
		{"evidence set", brain.DBIDs{Evidence: "ds_e"}, true},
		{"hunts set", brain.DBIDs{Hunts: "ds_h"}, true},
		{"intel set", brain.DBIDs{Intel: "ds_i"}, true},
		{"fully configured", brain.DBIDs{Evidence: "e", Hunts: "h", Intel: "i"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := brainConfigured(config.Config{Notion: c.ids}); got != c.want {
				t.Errorf("brainConfigured(%+v) = %v, want %v", c.ids, got, c.want)
			}
		})
	}
}

// TestDBIDsFromMap asserts provisioning output (logical name -> data-source ID)
// is mapped onto the right DBIDs fields, including the narrative-page parent.
func TestDBIDsFromMap(t *testing.T) {
	ds := map[string]string{
		brain.DBEvidence:   "ds_evidence",
		brain.DBHunts:      "ds_hunts",
		brain.DBIntel:      "ds_intel",
		brain.DBDetections: "ds_detections",
		brain.DBBacklog:    "ds_backlog",
	}
	got := brain.DBIDsFromMap(ds, "page_parent")
	want := brain.DBIDs{
		Evidence: "ds_evidence", Hunts: "ds_hunts",
		Intel: "ds_intel", Detections: "ds_detections", Backlog: "ds_backlog",
		Parent: "page_parent",
	}
	if got != want {
		t.Errorf("DBIDsFromMap = %+v, want %+v", got, want)
	}
}

// fakeNTN writes an executable `ntn` shim into a fresh dir and prepends it to
// PATH for the test. The shim answers the calls provisionBrain makes: whoami,
// `api POST /v1/databases` (returning a data-source ID slugged from the database
// title so the test can map titles back to logical stores), `api PATCH
// /v1/data_sources/...`, and `pages create`.
func fakeNTN(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	bindir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  whoami) echo '{"bot":{}}' ;;
  pages) echo '{"id":"page_child","url":"https://notion.so/page_child"}' ;;
  api)
    path="$2"
    case "$path" in
      /v1/databases)
        # Extract the database title's text content from the -d body and slug it
        # into a deterministic data-source ID.
        body=""
        prev=""
        for a in "$@"; do
          [ "$prev" = "-d" ] && body="$a"
          prev="$a"
        done
        title=$(printf '%s' "$body" | sed -n 's/.*"content":"\([^"]*\)".*/\1/p' | head -1)
        slug=$(printf '%s' "$title" | tr ' ' '_')
        printf '{"id":"db_%s","data_sources":[{"id":"ds_%s"}]}\n' "$slug" "$slug"
        ;;
      *) echo '{}' ;;
    esac
    ;;
  *) echo '{}' ;;
esac
`
	path := filepath.Join(bindir, "ntn")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestProvisionBrainWritesConfig is the end-to-end init test against a mocked
// ntn binary: it provisions the schema, then asserts .vala.json gains the right
// data-source IDs for every store while preserving an unrelated pre-existing key.
func TestProvisionBrainWritesConfig(t *testing.T) {
	fakeNTN(t)
	dir := t.TempDir()

	// Pre-seed an unrelated key to prove init merges rather than clobbers.
	if err := os.WriteFile(filepath.Join(dir, ".vala.json"), []byte(`{"model":"keep-me"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := provisionBrain(context.Background(), dir, "page_parent", false); err != nil {
		t.Fatalf("provisionBrain: %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "keep-me" {
		t.Errorf("unrelated key clobbered: model = %q", cfg.Model)
	}

	// The shim slugs each database title into ds_<Title_With_Underscores>; build
	// the expected mapping from the canonical schema so the test follows it.
	want := map[string]string{}
	for _, s := range brain.Schema() {
		want[s.Name] = "ds_" + strings.ReplaceAll(s.Title, " ", "_")
	}
	expected := brain.DBIDsFromMap(want, "page_child")
	if cfg.Notion != expected {
		t.Errorf("notion config = %+v\nwant %+v", cfg.Notion, expected)
	}
	if !brainConfigured(cfg) {
		t.Error("brain should read as configured after init")
	}
}

// TestProvisionBrainIdempotent asserts a second run on a configured project does
// not recreate databases: with the shim reporting every data source as existing,
// it should report success and leave config unchanged.
func TestProvisionBrainIdempotent(t *testing.T) {
	fakeNTN(t)
	dir := t.TempDir()

	if err := provisionBrain(context.Background(), dir, "page_parent", false); err != nil {
		t.Fatalf("first provisionBrain: %v", err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, ".vala.json"))

	// Second run without --force must take the verify path and not re-provision.
	if err := provisionBrain(context.Background(), dir, "page_parent", false); err != nil {
		t.Fatalf("second provisionBrain: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".vala.json"))
	if string(before) != string(after) {
		t.Errorf("idempotent run changed .vala.json:\nbefore %s\nafter %s", before, after)
	}
}
