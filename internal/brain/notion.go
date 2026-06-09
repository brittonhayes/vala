// Package brain is vala's Notion "case brain": typed writers for the Alerts,
// Cases, Evidence, Actions, and Runs databases plus the narrative case page.
//
// All writes go through the Notion interface. At runtime the default
// implementation shells out to the operator's authenticated `ntn` CLI (the same
// transport as internal/tools/ntn.go); tests and the harness use the in-memory
// Mem implementation, so the whole case-brain can be exercised deterministically
// without a network or Notion workspace.
package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Row is a created database row: an ID and the properties it was written with.
type Row struct {
	ID    string         `json:"id"`
	DB    string         `json:"db"`
	Props map[string]any `json:"props"`
}

// Notion is the read/write surface the case brain needs.
type Notion interface {
	// CreateRow appends a row to a database and returns its ID.
	CreateRow(ctx context.Context, db string, props map[string]any) (string, error)
	// UpdateRow patches properties on an existing row.
	UpdateRow(ctx context.Context, id string, props map[string]any) error
	// CreatePage creates a narrative page and returns its ID and URL.
	CreatePage(ctx context.Context, title string, markdown string) (id, url string, err error)
	// Query returns up to limit rows in a database whose contents match the
	// free-text query (an empty query matches everything). It is the read
	// counterpart to CreateRow that lets the agent recall what is already in the
	// brain before opening new work.
	Query(ctx context.Context, db, query string, limit int) ([]Row, error)
}

// Mem is an in-memory Notion implementation for tests, the harness, and
// unconfigured local runs. It records everything it is asked to write so callers
// can assert on the resulting case brain.
type Mem struct {
	mu    sync.Mutex
	seq   int
	Rows  map[string]*Row
	Pages map[string]string // id -> markdown
	URLs  map[string]string // id -> url
}

// NewMem returns an empty in-memory store.
func NewMem() *Mem {
	return &Mem{Rows: map[string]*Row{}, Pages: map[string]string{}, URLs: map[string]string{}}
}

func (m *Mem) next(prefix string) string {
	m.seq++
	return fmt.Sprintf("%s_%04d", prefix, m.seq)
}

// CreateRow records a new row and returns a synthetic ID.
func (m *Mem) CreateRow(_ context.Context, db string, props map[string]any) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.next(db)
	cp := make(map[string]any, len(props))
	for k, v := range props {
		cp[k] = v
	}
	m.Rows[id] = &Row{ID: id, DB: db, Props: cp}
	return id, nil
}

// UpdateRow patches an existing row.
func (m *Mem) UpdateRow(_ context.Context, id string, props map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.Rows[id]
	if !ok {
		return fmt.Errorf("no such row %q", id)
	}
	for k, v := range props {
		row.Props[k] = v
	}
	return nil
}

// CreatePage records a narrative page and returns a synthetic id/url.
func (m *Mem) CreatePage(_ context.Context, title, markdown string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.next("page")
	url := "mem://" + id
	m.Pages[id] = markdown
	m.URLs[id] = url
	return id, url, nil
}

// RowsIn returns the recorded rows for a database (test helper).
func (m *Mem) RowsIn(db string) []*Row {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Row
	for _, r := range m.Rows {
		if r.DB == db {
			out = append(out, r)
		}
	}
	return out
}

// Query returns rows in db whose properties contain the query substring
// (case-insensitive; an empty query matches all), sorted by ID for a stable
// result and capped at limit.
func (m *Mem) Query(_ context.Context, db, query string, limit int) ([]Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := strings.ToLower(query)
	var out []Row
	for _, r := range m.Rows {
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

// rowText renders a row's properties as a single searchable string.
func rowText(r *Row) string {
	b, _ := json.Marshal(r.Props)
	return string(b)
}

// NTN is a Notion implementation backed by the official `ntn` CLI. It is
// best-effort: Notion writes touch the network and require `ntn login`, so a
// failure is returned to the caller, which mirrors the durable record in the
// local session transcript regardless.
type NTN struct {
	Bin string // defaults to "ntn"
	Dir string
	DBs DBIDs // database IDs from config
}

// DBIDs holds the Notion database IDs the brain writes to.
type DBIDs struct {
	Alerts     string `json:"alerts"`
	Cases      string `json:"cases"`
	Evidence   string `json:"evidence"`
	Actions    string `json:"actions"`
	Runs       string `json:"runs"`
	Hunts      string `json:"hunts"`
	Intel      string `json:"intel"`
	Detections string `json:"detections"`
	Backlog    string `json:"backlog"`
	Parent     string `json:"case_page_parent"`
}

func (n *NTN) bin() string {
	if n.Bin != "" {
		return n.Bin
	}
	return "ntn"
}

func (n *NTN) run(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath(n.bin()); err != nil {
		return "", fmt.Errorf("ntn CLI not available: %w", err)
	}
	cmd := exec.CommandContext(ctx, n.bin(), args...)
	cmd.Dir = n.Dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("ntn %v: %w", args, err)
	}
	return out.String(), nil
}

// CreateRow creates a row by calling `ntn datasources rows create` with a JSON
// properties payload. The exact ntn flags may evolve; this routes through one
// place so the contract is easy to adjust.
func (n *NTN) CreateRow(ctx context.Context, db string, props map[string]any) (string, error) {
	payload, _ := json.Marshal(props)
	out, err := n.run(ctx, "datasources", "rows", "create", "--datasource", db, "--properties", string(payload))
	if err != nil {
		return "", err
	}
	return extractID(out), nil
}

// UpdateRow patches a row.
func (n *NTN) UpdateRow(ctx context.Context, id string, props map[string]any) error {
	payload, _ := json.Marshal(props)
	_, err := n.run(ctx, "datasources", "rows", "update", "--id", id, "--properties", string(payload))
	return err
}

// CreatePage creates a markdown page under the configured parent.
func (n *NTN) CreatePage(ctx context.Context, title, markdown string) (string, string, error) {
	out, err := n.run(ctx, "pages", "create", "--parent", n.DBs.Parent, "--title", title, "--content", markdown)
	if err != nil {
		return "", "", err
	}
	return extractID(out), "", nil
}

// Query reads rows back from a Notion database via `ntn datasources rows query`.
// Like CreateRow, the exact ntn flags may evolve; it routes through one place so
// the contract is easy to adjust, and parses the output tolerantly. This is the
// path that lets the agent recall prior hunts, intel, and detections so each
// hunt compounds on the last rather than repeating settled ground.
func (n *NTN) Query(ctx context.Context, db, query string, limit int) ([]Row, error) {
	args := []string{"datasources", "rows", "query", "--datasource", db}
	if query != "" {
		args = append(args, "--query", query)
	}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	out, err := n.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseRows(db, out), nil
}

// parseRows extracts rows from ntn query output, tolerating either a bare JSON
// array of rows or an object with a "results" array, and either inline
// properties or a nested "properties" object.
func parseRows(db, out string) []Row {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	var obj struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &obj); err == nil && obj.Results != nil {
		return rowsFromMaps(db, obj.Results)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err == nil {
		return rowsFromMaps(db, arr)
	}
	return nil
}

func rowsFromMaps(db string, ms []map[string]any) []Row {
	out := make([]Row, 0, len(ms))
	for _, m := range ms {
		r := Row{DB: db}
		if id, ok := m["id"].(string); ok {
			r.ID = id
		}
		if props, ok := m["properties"].(map[string]any); ok {
			r.Props = props
		} else {
			r.Props = m
		}
		out = append(out, r)
	}
	return out
}

// extractID pulls an "id" field out of ntn JSON output, tolerating plain text.
func extractID(out string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err == nil {
		if id, ok := m["id"].(string); ok {
			return id
		}
	}
	return ""
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
