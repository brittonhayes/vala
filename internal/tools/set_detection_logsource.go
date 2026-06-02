package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/sigma"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed set_detection_logsource.md
var setLogsourceDescription string

// SetDetectionLogsource sets fields inside the rule's logsource map. Not
// read-only: it edits a file.
type SetDetectionLogsource struct{ Dir string }

func (s *SetDetectionLogsource) Name() string        { return "set_detection_logsource" }
func (s *SetDetectionLogsource) Description() string { return setLogsourceDescription }
func (s *SetDetectionLogsource) ReadOnly() bool      { return false }

func (s *SetDetectionLogsource) Schema() tool.Schema {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	return tool.Schema{
		Properties: map[string]any{
			"path":       str("The rule file to edit."),
			"product":    str("Log source product, e.g. aws, windows, okta."),
			"service":    str("Log source service, e.g. cloudtrail, security."),
			"category":   str("Log source category, e.g. process_creation."),
			"definition": str("Free-text note on the exact data required."),
		},
		Required: []string{"path"},
	}
}

type setLogsourceInput struct {
	Path       string `json:"path"`
	Product    string `json:"product"`
	Service    string `json:"service"`
	Category   string `json:"category"`
	Definition string `json:"definition"`
}

func (s *SetDetectionLogsource) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in setLogsourceInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}

	fields := []struct{ key, val string }{
		{"product", in.Product}, {"service", in.Service},
		{"category", in.Category}, {"definition", in.Definition},
	}
	var set []string
	mutate := func(ed *sigma.Editor) error {
		ls, err := ed.EnsureMapping("logsource")
		if err != nil {
			return err
		}
		for _, f := range fields {
			if f.val == "" {
				continue
			}
			sigma.SetScalarInMapping(ls, f.key, f.val)
			set = append(set, fmt.Sprintf("%s=%s", f.key, f.val))
		}
		if len(set) == 0 {
			return fmt.Errorf("no logsource fields provided (set at least one of product/service/category/definition)")
		}
		return nil
	}
	return editDetection(s.Dir, in.Path, "set logsource "+strings.Join(set, ", "), mutate)
}
