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

//go:embed test_detection.md
var testDetectionDescription string

// TestDetection runs a rule's inline `tests:` through the evaluation engine.
// Read-only: it only reads the rule file and evaluates sample events in memory.
type TestDetection struct{ Dir string }

func (t *TestDetection) Name() string        { return "test_detection" }
func (t *TestDetection) Description() string { return testDetectionDescription }
func (t *TestDetection) ReadOnly() bool      { return true }

func (t *TestDetection) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path": map[string]any{"type": "string", "description": "The rule file whose inline tests should run."},
		},
		Required: []string{"path"},
	}
}

type testDetectionInput struct {
	Path string `json:"path"`
}

func (t *TestDetection) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in testDetectionInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Path == "" {
		return tool.Errorf("provide a rule file 'path'"), nil
	}

	results := detect.TestFile(resolve(t.Dir, in.Path))

	var b strings.Builder
	pass, fail := 0, 0
	for _, r := range results {
		label := in.Path
		if r.Title != "" {
			label = r.Title
		}
		if r.Doc > 1 {
			label = fmt.Sprintf("%s (doc %d)", label, r.Doc)
		}
		if r.Err != "" {
			fail++
			fmt.Fprintf(&b, "✗ %s: %s\n", label, r.Err)
			continue
		}
		for _, c := range r.Cases {
			if c.Passed() {
				pass++
				fmt.Fprintf(&b, "✓ %s\n", c.Name)
				continue
			}
			fail++
			if c.Err != "" {
				fmt.Fprintf(&b, "✗ %s: %s\n", c.Name, c.Err)
			} else {
				fmt.Fprintf(&b, "✗ %s: expected match=%v, got match=%v\n", c.Name, c.Want, c.Got)
			}
		}
	}
	fmt.Fprintf(&b, "\n%d passed, %d failed", pass, fail)

	res := tool.Text(b.String())
	res.IsError = fail > 0
	return res, nil
}
