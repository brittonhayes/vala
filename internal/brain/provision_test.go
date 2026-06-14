package brain

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestSchemaStatusOptions guards the alignment that the live workspace caught:
// Notion does not auto-create status options on write, so every "status"
// property in the schema must declare the option set its writer emits. A status
// column born without options 400s the first write.
func TestSchemaStatusOptions(t *testing.T) {
	for _, s := range Schema() {
		for _, p := range s.Props {
			if p.Type != "status" {
				continue
			}
			if len(s.StatusOptions[p.Name]) == 0 {
				t.Errorf("%s.%s is a status property with no StatusOptions", s.Name, p.Name)
			}
		}
	}
}

// TestSchemaHasExactlyOneTitle asserts each store has exactly one title column —
// the row's display field the writers populate.
func TestSchemaHasExactlyOneTitle(t *testing.T) {
	for _, s := range Schema() {
		titles := 0
		for _, p := range s.Props {
			if p.Type == "title" {
				titles++
			}
		}
		if titles != 1 {
			t.Errorf("%s has %d title properties, want exactly 1", s.Name, titles)
		}
	}
}

func TestSchemaHasNoPropertyRelationNameCollisions(t *testing.T) {
	for _, s := range Schema() {
		props := map[string]bool{}
		for _, p := range s.Props {
			props[p.Name] = true
		}
		for _, r := range s.Relations {
			if props[r.Name] {
				t.Errorf("%s defines %q as both a scalar property and relation", s.Name, r.Name)
			}
		}
	}
}

// TestPropConfigStatusOptions checks the status property configuration carries
// the seeded options in the Notion shape the create API expects.
func TestPropConfigStatusOptions(t *testing.T) {
	cfg := propConfig("status", []string{"Open", "Confirmed"})
	b, _ := json.Marshal(cfg)
	got := string(b)
	for _, want := range []string{`"type":"status"`, `"name":"Open"`, `"name":"Confirmed"`} {
		if !strings.Contains(got, want) {
			t.Errorf("propConfig status = %s, missing %q", got, want)
		}
	}
}

// fakeNTN writes an executable `ntn` shim onto PATH that answers the calls the
// provisioning methods make: whoami, `api POST /v1/databases` (a fixed parent DB
// + initial data source), `api POST /v1/data_sources` (a data-source ID slugged
// from the title so the test maps titles back to stores), `api GET` existence
// probes (failing for any ID containing "missing" so Verify can be exercised),
// and `api PATCH` (relations/rename).
func fakeNTN(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	bindir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  whoami) echo '{"bot":{}}' ;;
  api)
    path="$2"
    body=""
    prev=""
    for a in "$@"; do
      [ "$prev" = "-d" ] && body="$a"
      prev="$a"
    done
    case "$path" in
      /v1/databases)
        echo '{"id":"db_brain","data_sources":[{"id":"ds_initial"}]}' ;;
      /v1/data_sources)
        title=$(printf '%s' "$body" | sed -n 's/.*"content":"\([^"]*\)".*/\1/p' | head -1)
        slug=$(printf '%s' "$title" | tr ' ' '_')
        printf '{"id":"ds_%s"}\n' "$slug" ;;
      /v1/databases/*|/v1/data_sources/*)
        id=${path##*/}
        case "$id" in
          *missing*|*gone*) echo "not found" >&2; exit 1 ;;
          *) echo '{}' ;;
        esac ;;
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

// TestProvision drives single-database provisioning end to end against the shim:
// the first store becomes the database's initial data source (ds_initial), every
// other store is created as a data source slugged from its title, and the
// returned DBIDs carry the parent database and the brain's home page (where
// narrative hunt pages now live, with no separate wrapper page).
func TestProvision(t *testing.T) {
	fakeNTN(t)
	store := &NTN{}

	// The first schema store becomes the initial data source; assert the
	// assumption so this test stays honest if the order ever changes.
	if first := Schema()[0].Name; first != DBEvidence {
		t.Fatalf("test assumes %q is the first schema store, got %q", DBEvidence, first)
	}

	ids, err := store.Provision(context.Background(), "page_home")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	want := DBIDs{
		Database:   "db_brain",
		Evidence:   "ds_initial",
		Hunts:      "ds_Hunts",
		Intel:      "ds_Intel",
		Detections: "ds_Detections",
		Backlog:    "ds_Backlog",
		Memory:     "ds_Memory",
		Coverage:   "ds_Coverage",
		Parent:     "page_home",
	}
	if ids != want {
		t.Errorf("Provision DBIDs =\n  %+v\nwant\n  %+v", ids, want)
	}
}

// TestVerifyReportsMissingAndDatabase covers the read side of repair: an empty or
// unreachable store ID is reported missing, and databaseOK reflects whether the
// parent database resolves.
func TestVerifyReportsMissingAndDatabase(t *testing.T) {
	fakeNTN(t)
	store := &NTN{}

	ids := DBIDs{
		Database:   "db_brain",
		Evidence:   "ds_evidence",
		Hunts:      "ds_hunts",
		Intel:      "ds_intel",
		Detections: "ds_detections",
		Backlog:    "ds_backlog",
		Memory:     "ds_memory",
		Coverage:   "", // unset → missing without a network probe
	}
	missing, databaseOK := store.Verify(context.Background(), ids)
	if !databaseOK {
		t.Error("databaseOK = false, want true for a resolvable database")
	}
	if !slices.Equal(missing, []string{DBCoverage}) {
		t.Errorf("missing = %v, want [%s]", missing, DBCoverage)
	}

	// A database ID that does not resolve flips databaseOK, and an unreachable
	// store ID is reported alongside the unset one.
	ids.Database = "db_missing"
	ids.Memory = "ds_missing_mem"
	missing, databaseOK = store.Verify(context.Background(), ids)
	if databaseOK {
		t.Error("databaseOK = true, want false for an unresolvable database")
	}
	if !slices.Contains(missing, DBMemory) || !slices.Contains(missing, DBCoverage) {
		t.Errorf("missing = %v, want it to contain %s and %s", missing, DBMemory, DBCoverage)
	}
}

// TestAddMissingRepairsInPlace asserts repair recreates only the missing store
// under the existing database and preserves every other ID.
func TestAddMissingRepairsInPlace(t *testing.T) {
	fakeNTN(t)
	store := &NTN{}

	ids := DBIDs{
		Database:   "db_brain",
		Evidence:   "ds_evidence",
		Hunts:      "ds_hunts",
		Intel:      "ds_intel",
		Detections: "ds_detections",
		Backlog:    "ds_backlog",
		Memory:     "ds_memory",
		Coverage:   "", // the broken store
		Parent:     "page_home",
	}
	patched, err := store.AddMissing(context.Background(), ids, []string{DBCoverage})
	if err != nil {
		t.Fatalf("AddMissing: %v", err)
	}
	if patched.Coverage != "ds_Coverage" {
		t.Errorf("Coverage = %q, want ds_Coverage (recreated)", patched.Coverage)
	}
	ids.Coverage = "ds_Coverage" // everything else is untouched
	if patched != ids {
		t.Errorf("AddMissing changed unrelated IDs:\n  got  %+v\n  want %+v", patched, ids)
	}
}

// TestAddMissingNeedsDatabase asserts repair refuses to run without a parent
// database (the legacy case that must re-provision from scratch instead).
func TestAddMissingNeedsDatabase(t *testing.T) {
	store := &NTN{}
	if _, err := store.AddMissing(context.Background(), DBIDs{}, []string{DBCoverage}); err == nil {
		t.Error("AddMissing with no parent database should error")
	}
}
