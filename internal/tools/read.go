package tools

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed read.md
var readDescription string

const (
	defaultReadLimit = 2000
	maxLineLength    = 2000
)

// Read returns the contents of a file with line numbers. Read-only.
type Read struct{ Dir string }

func (r *Read) Name() string        { return "read" }
func (r *Read) Description() string { return readDescription }
func (r *Read) ReadOnly() bool      { return true }

func (r *Read) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path":   map[string]any{"type": "string", "description": "Path to the file to read."},
			"offset": map[string]any{"type": "integer", "description": "1-based line to start from."},
			"limit":  map[string]any{"type": "integer", "description": "Max lines to return (default 2000)."},
		},
		Required: []string{"path"},
	}
}

type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func (r *Read) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	f, err := os.Open(resolve(r.Dir, in.Path))
	if err != nil {
		return tool.Errorf("cannot open %s: %v", in.Path, err), nil
	}
	defer f.Close()

	limit := in.Limit
	if limit <= 0 {
		limit = defaultReadLimit
	}
	start := in.Offset
	if start < 1 {
		start = 1
	}

	var b strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	lineNo := 0
	shown := 0
	for sc.Scan() {
		lineNo++
		if lineNo < start {
			continue
		}
		if shown >= limit {
			fmt.Fprintf(&b, "... [more lines; increase limit or use offset] ...\n")
			break
		}
		line := sc.Text()
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "…"
		}
		fmt.Fprintf(&b, "%6d\t%s\n", lineNo, line)
		shown++
	}
	if err := sc.Err(); err != nil {
		return tool.Errorf("read error: %v", err), nil
	}
	if shown == 0 {
		return tool.Text("(empty file or offset past end of file)"), nil
	}
	return tool.Text(b.String()), nil
}
