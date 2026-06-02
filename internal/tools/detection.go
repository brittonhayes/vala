package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/detect"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed detection.md
var detectionDescription string

// ValidateDetection validates Sigma rules against the embedded Sigma schema.
// Read-only: it only reads files, so it never needs permission.
type ValidateDetection struct{ Dir string }

func (v *ValidateDetection) Name() string        { return "validate_detection" }
func (v *ValidateDetection) Description() string { return detectionDescription }
func (v *ValidateDetection) ReadOnly() bool      { return true }

func (v *ValidateDetection) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path":      map[string]any{"type": "string", "description": "A single rule file to validate."},
			"dir":       map[string]any{"type": "string", "description": "A directory of rule files to validate."},
			"recursive": map[string]any{"type": "boolean", "description": "With dir, descend into subdirectories."},
		},
	}
}

type validateInput struct {
	Path      string `json:"path"`
	Dir       string `json:"dir"`
	Recursive bool   `json:"recursive"`
}

func (v *ValidateDetection) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in validateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Path == "" && in.Dir == "" {
		return tool.Errorf("provide either 'path' (a file) or 'dir' (a directory)"), nil
	}

	var results []detect.FileResult
	if in.Path != "" {
		results = append(results, detect.ValidateFile(resolve(v.Dir, in.Path)))
	} else {
		results = detect.ValidateDir(resolve(v.Dir, in.Dir), in.Recursive)
	}
	if len(results) == 0 {
		return tool.Text("no rule files (*.yml / *.yaml) found"), nil
	}

	var b strings.Builder
	valid, invalid := 0, 0
	for _, r := range results {
		if r.Valid {
			valid++
			fmt.Fprintf(&b, "✓ %s\n", r.Path)
			continue
		}
		invalid++
		fmt.Fprintf(&b, "✗ %s\n", r.Path)
		for _, is := range r.Issues {
			fmt.Fprintf(&b, "    - %s\n", is.String())
		}
	}
	fmt.Fprintf(&b, "\n%d valid, %d invalid (%d total)", valid, invalid, len(results))

	res := tool.Text(b.String())
	res.IsError = invalid > 0
	return res, nil
}
