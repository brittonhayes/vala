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

//go:embed set_detection_runbook.md
var setRunbookDescription string

// SetDetectionRunbook sets keys in the rule's inline `runbook:` map. Not
// read-only: it edits a file.
type SetDetectionRunbook struct{ Dir string }

func (s *SetDetectionRunbook) Name() string        { return "set_detection_runbook" }
func (s *SetDetectionRunbook) Description() string { return setRunbookDescription }
func (s *SetDetectionRunbook) ReadOnly() bool      { return false }

func (s *SetDetectionRunbook) Schema() tool.Schema {
	steps := func(desc string) map[string]any {
		return map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": desc,
		}
	}
	return tool.Schema{
		Properties: map[string]any{
			"path":        map[string]any{"type": "string", "description": "The rule file to edit."},
			"triage":      steps("First steps to size up the alert (who/what/where)."),
			"investigate": steps("How to dig in: related events, scope, intent."),
			"contain":     steps("Actions to stop or limit the activity."),
			"escalate":    steps("When and to whom to escalate."),
			"references":  steps("Links supporting the response."),
		},
		Required: []string{"path"},
	}
}

type setRunbookInput struct {
	Path        string   `json:"path"`
	Triage      []string `json:"triage"`
	Investigate []string `json:"investigate"`
	Contain     []string `json:"contain"`
	Escalate    []string `json:"escalate"`
	References  []string `json:"references"`
}

func (s *SetDetectionRunbook) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in setRunbookInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}

	sections := []struct {
		key  string
		vals []string
	}{
		{"triage", in.Triage}, {"investigate", in.Investigate},
		{"contain", in.Contain}, {"escalate", in.Escalate},
		{"references", in.References},
	}
	var set []string
	mutate := func(ed *sigma.Editor) error {
		rb, err := ed.EnsureMapping("runbook")
		if err != nil {
			return err
		}
		for _, s := range sections {
			if len(s.vals) == 0 {
				continue
			}
			if err := sigma.SetInMapping(rb, s.key, s.vals); err != nil {
				return err
			}
			set = append(set, s.key)
		}
		if len(set) == 0 {
			return fmt.Errorf("provide at least one runbook section (triage/investigate/contain/escalate/references)")
		}
		return nil
	}
	return editDetection(s.Dir, in.Path, "set runbook section(s): "+strings.Join(set, ", "), mutate)
}
