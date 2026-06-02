package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/brittonhayes/vala/internal/tool"
)

// errStopWalk halts GlobWalk once the result cap is reached.
var errStopWalk = errors.New("stop walk")

//go:embed glob.md
var globDescription string

const maxGlobResults = 500

// Glob finds files matching a doublestar pattern. Read-only.
type Glob struct{ Dir string }

func (g *Glob) Name() string        { return "glob" }
func (g *Glob) Description() string { return globDescription }
func (g *Glob) ReadOnly() bool      { return true }

func (g *Glob) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern, supports ** for recursion."},
			"path":    map[string]any{"type": "string", "description": "Base directory (default: working directory)."},
		},
		Required: []string{"pattern"},
	}
}

func (g *Glob) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Pattern == "" {
		return tool.Errorf("pattern is required"), nil
	}
	base := resolve(g.Dir, in.Path)
	if base == "" {
		base = "."
	}

	var matches []string
	fsys := os.DirFS(base)
	err := doublestar.GlobWalk(fsys, in.Pattern, func(path string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		matches = append(matches, path)
		if len(matches) >= maxGlobResults {
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return tool.Errorf("glob error: %v", err), nil
	}
	if len(matches) == 0 {
		return tool.Text("(no matches)"), nil
	}
	return tool.Text(strings.Join(matches, "\n")), nil
}
