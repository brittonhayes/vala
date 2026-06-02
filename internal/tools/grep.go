package tools

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed grep.md
var grepDescription string

const maxGrepMatches = 200

// Grep searches file contents for a regular expression. Read-only. It prefers
// ripgrep when present and falls back to a Go implementation otherwise.
type Grep struct{ Dir string }

func (g *Grep) Name() string        { return "grep" }
func (g *Grep) Description() string { return grepDescription }
func (g *Grep) ReadOnly() bool      { return true }

func (g *Grep) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Regular expression to search for."},
			"path":    map[string]any{"type": "string", "description": "File or directory to search (default: working dir)."},
			"glob":    map[string]any{"type": "string", "description": "Only search files matching this glob (e.g. *.yml)."},
		},
		Required: []string{"pattern"},
	}
}

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Glob    string `json:"glob"`
}

func (g *Grep) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Pattern == "" {
		return tool.Errorf("pattern is required"), nil
	}
	searchPath := resolve(g.Dir, in.Path)
	if searchPath == "" {
		searchPath = "."
	}

	if rg, err := exec.LookPath("rg"); err == nil {
		if out, ok := g.runRipgrep(ctx, rg, in, searchPath); ok {
			return out, nil
		}
	}
	return g.runGo(in, searchPath)
}

// runRipgrep shells out to ripgrep. The bool reports whether the result is
// usable; false means fall back to the Go implementation.
func (g *Grep) runRipgrep(ctx context.Context, rg string, in grepInput, searchPath string) (tool.Result, bool) {
	args := []string{"--line-number", "--no-heading", "--color=never", "--max-count=50"}
	if in.Glob != "" {
		args = append(args, "--glob", in.Glob)
	}
	args = append(args, "--regexp", in.Pattern, searchPath)

	cmd := exec.CommandContext(ctx, rg, args...)
	out, err := cmd.Output()
	if err != nil {
		// rg exits 1 when there are no matches; treat that as a clean empty result.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return tool.Text("(no matches)"), true
		}
		return tool.Result{}, false
	}
	return tool.Text(capLines(string(out), maxGrepMatches)), true
}

// runGo walks the tree applying the regex, used when ripgrep is unavailable.
func (g *Grep) runGo(in grepInput, searchPath string) (tool.Result, error) {
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return tool.Errorf("invalid regex: %v", err), nil
	}
	var b strings.Builder
	matches := 0

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if in.Glob != "" {
			if ok, _ := doublestar.Match(in.Glob, d.Name()); !ok {
				return nil
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 5*1024*1024)
		line := 0
		for sc.Scan() {
			line++
			if re.MatchString(sc.Text()) {
				fmt.Fprintf(&b, "%s:%d:%s\n", path, line, strings.TrimSpace(sc.Text()))
				matches++
				if matches >= maxGrepMatches {
					return errStopWalk
				}
			}
		}
		return nil
	}

	info, err := os.Stat(searchPath)
	if err != nil {
		return tool.Errorf("cannot stat %s: %v", searchPath, err), nil
	}
	if info.IsDir() {
		err = filepath.WalkDir(searchPath, walk)
	} else {
		err = walk(searchPath, dirEntry{info}, nil)
	}
	if err != nil && err != errStopWalk {
		return tool.Errorf("search error: %v", err), nil
	}
	if matches == 0 {
		return tool.Text("(no matches)"), nil
	}
	return tool.Text(b.String()), nil
}

// dirEntry adapts a FileInfo to fs.DirEntry for the single-file walk case.
type dirEntry struct{ fi os.FileInfo }

func (d dirEntry) Name() string               { return d.fi.Name() }
func (d dirEntry) IsDir() bool                { return d.fi.IsDir() }
func (d dirEntry) Type() fs.FileMode          { return d.fi.Mode().Type() }
func (d dirEntry) Info() (fs.FileInfo, error) { return d.fi, nil }

// capLines truncates output to at most n lines.
func capLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "") + "... [more matches; narrow your search] ...\n"
}
