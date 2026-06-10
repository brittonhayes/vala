package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// File is a Notion implementation that persists the brain to a JSON file on
// disk. It is the durable middle ground between the ephemeral Mem store and a
// networked Notion workspace: hunts, intel, evidence, and detections compound
// across sessions with no external account, and the whole brain is a single
// portable, version-controllable artifact. Narrative hunt pages are written as
// readable Markdown files in a "pages" directory beside the JSON.
type File struct {
	mu       sync.Mutex
	path     string
	pagesDir string
	d        fileData
}

// fileData is the on-disk shape of a File brain. Narrative pages live as
// separate .md files, so only rows and the ID sequence are serialized here.
type fileData struct {
	Seq  int             `json:"seq"`
	Rows map[string]*Row `json:"rows"`
}

// NewFile opens (or prepares) a file-backed brain at path, loading any existing
// rows so the brain resumes where the last session left off. The file itself is
// created lazily on the first write; a missing file is not an error.
func NewFile(path string) (*File, error) {
	f := &File{
		path:     path,
		pagesDir: filepath.Join(filepath.Dir(path), "pages"),
		d:        fileData{Rows: map[string]*Row{}},
	}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(data) > 0 {
			if err := json.Unmarshal(data, &f.d); err != nil {
				return nil, fmt.Errorf("parse brain file %s: %w", path, err)
			}
		}
		if f.d.Rows == nil {
			f.d.Rows = map[string]*Row{}
		}
	case os.IsNotExist(err):
		// Fresh brain; nothing to load.
	default:
		return nil, fmt.Errorf("read brain file %s: %w", path, err)
	}
	return f, nil
}

func (f *File) next(prefix string) string {
	f.d.Seq++
	return fmt.Sprintf("%s_%04d", prefix, f.d.Seq)
}

// persist writes the brain to disk atomically (temp file + rename) so a crash
// mid-write cannot corrupt an existing brain. Callers hold f.mu.
func (f *File) persist() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

// CreateRow records a new row, persists, and returns its ID.
func (f *File) CreateRow(_ context.Context, db string, props map[string]any) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.next(db)
	cp := make(map[string]any, len(props))
	for k, v := range props {
		cp[k] = v
	}
	f.d.Rows[id] = &Row{ID: id, DB: db, Props: cp}
	if err := f.persist(); err != nil {
		return "", err
	}
	return id, nil
}

// UpdateRow patches an existing row and persists.
func (f *File) UpdateRow(_ context.Context, id string, props map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.d.Rows[id]
	if !ok {
		return fmt.Errorf("no such row %q", id)
	}
	for k, v := range props {
		row.Props[k] = v
	}
	return f.persist()
}

// CreatePage writes a narrative page as a Markdown file beside the brain and
// returns its ID and a file:// URL pointing at the readable artifact.
func (f *File) CreatePage(_ context.Context, title, markdown string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.next("page")
	content := markdown
	if title != "" && !strings.HasPrefix(strings.TrimSpace(content), "#") {
		content = "# " + title + "\n\n" + markdown
	}
	if err := os.MkdirAll(f.pagesDir, 0o755); err != nil {
		return "", "", err
	}
	pagePath := filepath.Join(f.pagesDir, id+".md")
	if err := os.WriteFile(pagePath, []byte(content), 0o644); err != nil {
		return "", "", err
	}
	if err := f.persist(); err != nil {
		return "", "", err
	}
	abs, err := filepath.Abs(pagePath)
	if err != nil {
		abs = pagePath
	}
	return id, "file://" + abs, nil
}

// Query returns rows in db whose properties contain the query substring
// (case-insensitive; an empty query matches all), sorted by ID and capped at
// limit — the same substring semantics as Mem and NTN.
func (f *File) Query(_ context.Context, db, query string, limit int) ([]Row, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := strings.ToLower(query)
	var out []Row
	for _, r := range f.d.Rows {
		if r.DB != db {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(rowText(r)), q) {
			continue
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
