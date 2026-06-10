package brain

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestFilePersistsAcrossReopen is the core compounding-memory guarantee: a row
// written and updated in one session is recalled, with its update, in the next.
func TestFilePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "brain", "brain.json")

	f, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	id, err := f.CreateRow(ctx, DBHunts, map[string]any{"hunt_id": "did anyone disable GuardDuty?", "status": "Open"})
	if err != nil {
		t.Fatalf("CreateRow: %v", err)
	}
	if err := f.UpdateRow(ctx, id, map[string]any{"status": "Confirmed"}); err != nil {
		t.Fatalf("UpdateRow: %v", err)
	}

	reopened, err := NewFile(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	rows, err := reopened.Query(ctx, DBHunts, "GuardDuty", 0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row after reopen, got %d", len(rows))
	}
	if rows[0].Props["status"] != "Confirmed" {
		t.Fatalf("update not persisted: status=%v", rows[0].Props["status"])
	}
}

// TestFileSeqResumesAfterReopen ensures the ID sequence survives a reopen so a
// new session cannot collide with IDs minted by an earlier one.
func TestFileSeqResumesAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "brain.json")

	f, _ := NewFile(path)
	id1, err := f.CreateRow(ctx, DBIntel, map[string]any{"intel_id": "a"})
	if err != nil {
		t.Fatalf("CreateRow: %v", err)
	}
	reopened, _ := NewFile(path)
	id2, err := reopened.CreateRow(ctx, DBIntel, map[string]any{"intel_id": "b"})
	if err != nil {
		t.Fatalf("CreateRow after reopen: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("IDs collided across reopen: both %s", id1)
	}
}

// TestFileCreatePageWritesMarkdown asserts narrative pages land as readable .md
// files beside the brain.
func TestFileCreatePageWritesMarkdown(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f, err := NewFile(filepath.Join(dir, "brain.json"))
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	_, url, err := f.CreatePage(ctx, "Hunt: GuardDuty", "Findings here.")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if url == "" {
		t.Fatal("empty page url")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "pages"))
	if err != nil {
		t.Fatalf("read pages dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 page file, got %d", len(entries))
	}
}

// TestNewFileMissingIsEmptyBrain confirms opening a non-existent path yields a
// usable empty brain rather than an error.
func TestNewFileMissingIsEmptyBrain(t *testing.T) {
	f, err := NewFile(filepath.Join(t.TempDir(), "nope", "brain.json"))
	if err != nil {
		t.Fatalf("NewFile on missing path: %v", err)
	}
	rows, err := f.Query(context.Background(), DBHunts, "", 0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("want empty brain, got %d rows", len(rows))
	}
}
