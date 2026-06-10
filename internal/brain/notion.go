// Package brain is vala's Notion "hunt brain": typed writers for the Hunts,
// Evidence, Intel, Detections, and Backlog databases plus the narrative hunt
// page.
//
// All writes go through the Notion interface. At runtime the default
// implementation shells out to the operator's authenticated `ntn` CLI (the same
// transport as internal/tools/ntn.go); tests use the in-memory Mem
// implementation, so the whole brain can be exercised deterministically without
// a network or Notion workspace.
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

// Notion is the read/write surface the brain needs.
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

// Mem is an in-memory Notion implementation for tests and unconfigured local
// runs. It records everything it is asked to write so callers can assert on the
// resulting brain.
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
//
// Rows are Notion pages in a data source. Because the Notion API requires typed
// property objects matched to the target schema by name, NTN fetches each data
// source's schema (property name -> type) and coerces the brain's flat props to
// typed values. The configured DB IDs are treated as data-source IDs; the
// brain's prop keys are expected to match the Notion property names.
type NTN struct {
	Bin string // defaults to "ntn"
	Dir string
	DBs DBIDs // data-source IDs from config

	mu      sync.Mutex
	schemas map[string]map[string]string // data-source ID -> (property name -> type)
}

// DBIDs holds the Notion database IDs the brain writes to.
type DBIDs struct {
	Evidence   string `json:"evidence"`
	Hunts      string `json:"hunts"`
	Intel      string `json:"intel"`
	Detections string `json:"detections"`
	Backlog    string `json:"backlog"`
	Memory     string `json:"memory"`
	Parent     string `json:"page_parent"`
}

func (n *NTN) bin() string {
	if n.Bin != "" {
		return n.Bin
	}
	return "ntn"
}

// runOut runs ntn capturing stdout separately from stderr, so JSON responses
// parse cleanly and the error carries ntn's diagnostic text.
func (n *NTN) runOut(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(n.bin()); err != nil {
		return nil, fmt.Errorf("ntn CLI not available: %w", err)
	}
	cmd := exec.CommandContext(ctx, n.bin(), args...)
	cmd.Dir = n.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ntn %v: %w: %s", args, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// CreateRow creates a row (a page in a data source) via `POST /v1/pages`. It
// fetches the data source's schema to type each property, then sends the typed
// payload through `ntn api`. db is a data-source ID.
func (n *NTN) CreateRow(ctx context.Context, db string, props map[string]any) (string, error) {
	schema, err := n.schema(ctx, db)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"parent":     map[string]any{"type": "data_source_id", "data_source_id": db},
		"properties": toProperties(schema, props),
	}
	out, err := n.api(ctx, "POST", "/v1/pages", body)
	if err != nil {
		return "", err
	}
	return extractID(string(out)), nil
}

// UpdateRow patches a row's properties via `PATCH /v1/pages/{id}`. The row's
// data source (and thus its schema) is discovered from the page's parent so
// properties can be typed.
func (n *NTN) UpdateRow(ctx context.Context, id string, props map[string]any) error {
	dsID, err := n.pageDataSource(ctx, id)
	if err != nil {
		return err
	}
	schema, err := n.schema(ctx, dsID)
	if err != nil {
		return err
	}
	body := map[string]any{"properties": toProperties(schema, props)}
	_, err = n.api(ctx, "PATCH", "/v1/pages/"+id, body)
	return err
}

// CreatePage creates a narrative markdown page under the configured parent page.
// `ntn pages create` derives the page title from the Markdown's leading heading,
// so we ensure the content opens with one rather than passing a (non-existent)
// --title flag.
func (n *NTN) CreatePage(ctx context.Context, title, markdown string) (string, string, error) {
	content := markdown
	if title != "" && !strings.HasPrefix(strings.TrimSpace(content), "#") {
		content = "# " + title + "\n\n" + markdown
	}
	args := []string{"pages", "create", "--content", content, "--json"}
	if n.DBs.Parent != "" {
		args = append(args, "--parent", "page:"+n.DBs.Parent)
	}
	out, err := n.runOut(ctx, args...)
	if err != nil {
		return "", "", err
	}
	return extractID(string(out)), extractField(string(out), "url"), nil
}

// api calls a Notion API endpoint through `ntn api`, sending body (if non-nil)
// as the JSON request body. It returns the raw response bytes.
func (n *NTN) api(ctx context.Context, method, path string, body any) ([]byte, error) {
	args := []string{"api", path, "-X", method}
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		args = append(args, "-d", string(payload))
	}
	return n.runOut(ctx, args...)
}

// schema returns a data source's property-name -> type map, fetched once via
// `GET /v1/data_sources/{id}` and cached for the life of the store.
func (n *NTN) schema(ctx context.Context, dsID string) (map[string]string, error) {
	n.mu.Lock()
	if s, ok := n.schemas[dsID]; ok {
		n.mu.Unlock()
		return s, nil
	}
	n.mu.Unlock()

	out, err := n.api(ctx, "GET", "/v1/data_sources/"+dsID, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse data source schema: %w", err)
	}
	s := make(map[string]string, len(resp.Properties))
	for name, p := range resp.Properties {
		s[name] = p.Type
	}
	n.mu.Lock()
	if n.schemas == nil {
		n.schemas = map[string]map[string]string{}
	}
	n.schemas[dsID] = s
	n.mu.Unlock()
	return s, nil
}

// pageDataSource returns the data-source ID a page belongs to, read from its
// parent via `GET /v1/pages/{id}`.
func (n *NTN) pageDataSource(ctx context.Context, id string) (string, error) {
	out, err := n.api(ctx, "GET", "/v1/pages/"+id, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Parent struct {
			DataSourceID string `json:"data_source_id"`
			DatabaseID   string `json:"database_id"`
		} `json:"parent"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse page parent: %w", err)
	}
	if resp.Parent.DataSourceID != "" {
		return resp.Parent.DataSourceID, nil
	}
	return resp.Parent.DatabaseID, nil
}

// toProperties coerces the brain's flat props into Notion typed property values,
// keyed by the data source's property names. Props with no matching schema
// property are skipped, and nil values are omitted.
func toProperties(schema map[string]string, props map[string]any) map[string]any {
	out := make(map[string]any)
	for name, typ := range schema {
		v, ok := props[name]
		if !ok || v == nil {
			continue
		}
		if pv := typedValue(typ, v); pv != nil {
			out[name] = pv
		}
	}
	return out
}

// typedValue wraps a flat value in the Notion property shape for its type. It
// covers the property types the brain writes; an unknown type falls back to
// rich_text so data is preserved rather than dropped.
func typedValue(typ string, v any) any {
	switch typ {
	case "title":
		return map[string]any{"title": richText(fmt.Sprint(v))}
	case "rich_text":
		return map[string]any{"rich_text": richText(fmt.Sprint(v))}
	case "select":
		s := fmt.Sprint(v)
		if s == "" {
			return nil
		}
		return map[string]any{"select": map[string]any{"name": s}}
	case "status":
		s := fmt.Sprint(v)
		if s == "" {
			return nil
		}
		return map[string]any{"status": map[string]any{"name": s}}
	case "multi_select":
		return map[string]any{"multi_select": namedList(v)}
	case "date":
		s := fmt.Sprint(v)
		if s == "" {
			return nil
		}
		return map[string]any{"date": map[string]any{"start": s}}
	case "number":
		return map[string]any{"number": v}
	case "checkbox":
		b, _ := v.(bool)
		return map[string]any{"checkbox": b}
	case "url":
		return map[string]any{"url": fmt.Sprint(v)}
	case "email":
		return map[string]any{"email": fmt.Sprint(v)}
	case "relation":
		return map[string]any{"relation": idList(v)}
	default:
		return map[string]any{"rich_text": richText(fmt.Sprint(v))}
	}
}

// richText renders a string as a Notion rich-text array (empty for "").
func richText(s string) []any {
	if s == "" {
		return []any{}
	}
	return []any{map[string]any{"type": "text", "text": map[string]any{"content": s}}}
}

// asStrings normalizes a string, []string, or []any value to a string slice.
func asStrings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			out = append(out, fmt.Sprint(e))
		}
		return out
	}
	return nil
}

// namedList renders values as Notion {name} objects (multi_select).
func namedList(v any) []any {
	ss := asStrings(v)
	out := make([]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, map[string]any{"name": s})
	}
	return out
}

// idList renders values as Notion {id} objects (relation).
func idList(v any) []any {
	ss := asStrings(v)
	out := make([]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, map[string]any{"id": s})
	}
	return out
}

// Query reads rows back from a Notion data source via `ntn datasources query`.
// ntn 0.16's query takes a positional data-source ID and has no free-text
// search — only structured --filter JSON — so we fetch a window with --json and
// match the free-text query client-side, mirroring Mem's substring semantics.
// This is the path that lets the agent recall prior hunts, intel, and detections
// so each hunt compounds on the last rather than repeating settled ground.
//
// db must be a data-source ID, not a database ID; resolve a database ID with
// `ntn datasources resolve <database-id>`.
func (n *NTN) Query(ctx context.Context, db, query string, limit int) ([]Row, error) {
	// With a free-text query we fetch a larger window so the client-side filter
	// has rows to match against; without one we just take the most recent.
	fetch := limit
	if query != "" {
		fetch = queryFetchWindow
	}
	args := []string{"datasources", "query", db, "--json"}
	if fetch > 0 {
		args = append(args, "--limit", strconv.Itoa(fetch))
	}
	out, err := n.runOut(ctx, args...)
	if err != nil {
		return nil, err
	}
	rows := filterRows(parseRows(db, string(out)), query)
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

// queryFetchWindow is how many rows Query pulls from a data source before
// applying the client-side free-text filter (Notion caps a page at 100).
const queryFetchWindow = 100

// filterRows keeps rows whose serialized properties contain the query substring
// (case-insensitive). An empty query keeps everything.
func filterRows(rows []Row, query string) []Row {
	if query == "" {
		return rows
	}
	q := strings.ToLower(query)
	out := rows[:0]
	for _, r := range rows {
		if strings.Contains(strings.ToLower(rowText(&r)), q) {
			out = append(out, r)
		}
	}
	return out
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
func extractID(out string) string { return extractField(out, "id") }

// extractField pulls a top-level string field out of ntn JSON output,
// tolerating non-JSON text (returns "").
func extractField(out, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err == nil {
		if v, ok := m[field].(string); ok {
			return v
		}
	}
	return ""
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
