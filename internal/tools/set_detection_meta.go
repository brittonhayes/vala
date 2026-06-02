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

//go:embed set_detection_meta.md
var setMetaDescription string

// SetDetectionMeta sets scalar metadata fields on a rule (title, id, status,
// description, author, date, level). Not read-only: it edits a file.
type SetDetectionMeta struct{ Dir string }

func (s *SetDetectionMeta) Name() string        { return "set_detection_meta" }
func (s *SetDetectionMeta) Description() string { return setMetaDescription }
func (s *SetDetectionMeta) ReadOnly() bool      { return false }

func (s *SetDetectionMeta) Schema() tool.Schema {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	return tool.Schema{
		Properties: map[string]any{
			"path":        str("The rule file to edit."),
			"title":       str("Rule title."),
			"id":          str("Rule UUID. Pass \"generate\" to mint a fresh UUID v4."),
			"status":      str("experimental | test | stable | deprecated | unsupported."),
			"description": str("What the rule detects and why it matters."),
			"author":      str("Rule author."),
			"date":        str("Authoring date, YYYY-MM-DD."),
			"level":       str("informational | low | medium | high | critical."),
		},
		Required: []string{"path"},
	}
}

type setMetaInput struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	ID          string `json:"id"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Date        string `json:"date"`
	Level       string `json:"level"`
}

func (s *SetDetectionMeta) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in setMetaInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}

	fields := []struct{ key, val string }{
		{"title", in.Title}, {"id", in.ID}, {"status", in.Status},
		{"description", in.Description}, {"author", in.Author},
		{"date", in.Date}, {"level", in.Level},
	}
	var set []string
	mutate := func(ed *sigma.Editor) error {
		for _, f := range fields {
			if f.val == "" {
				continue
			}
			val := f.val
			if f.key == "id" && (val == "generate" || val == "uuid") {
				val = newUUIDv4()
			}
			ed.SetScalar(f.key, val)
			set = append(set, fmt.Sprintf("%s=%s", f.key, val))
		}
		if len(set) == 0 {
			return fmt.Errorf("no metadata fields provided (set at least one of title/id/status/description/author/date/level)")
		}
		return nil
	}
	return editDetection(s.Dir, in.Path, "set "+strings.Join(set, ", "), mutate)
}
