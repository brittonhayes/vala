package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/reference"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed reference.md
var referenceDescription string

// ReferenceDetection surfaces the curated, gold-standard Sigma exemplars
// embedded in the binary. Read-only: it only returns reference material.
type ReferenceDetection struct{}

func (r *ReferenceDetection) Name() string        { return "reference_detection" }
func (r *ReferenceDetection) Description() string { return referenceDescription }
func (r *ReferenceDetection) ReadOnly() bool      { return true }

func (r *ReferenceDetection) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"list": map[string]any{"type": "boolean", "description": "List every reference detection (name, title, level, techniques)."},
			"name": map[string]any{"type": "string", "description": "Return the full YAML of one reference detection by name."},
		},
	}
}

type referenceInput struct {
	List bool   `json:"list"`
	Name string `json:"name"`
}

func (r *ReferenceDetection) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in referenceInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Name != "" {
		data, err := reference.Get(in.Name)
		if err != nil {
			return tool.Errorf("%v (use {\"list\": true} to see available names)", err), nil
		}
		return tool.Text(string(data)), nil
	}

	// Default to the index listing.
	metas, err := reference.List()
	if err != nil {
		return tool.Errorf("cannot list references: %v", err), nil
	}
	if len(metas) == 0 {
		return tool.Text("no reference detections embedded"), nil
	}
	var b strings.Builder
	b.WriteString("Gold-standard Sigma exemplars (each carries an inline runbook and executable tests).\n")
	b.WriteString("Request one in full with {\"name\": \"<name>\"}.\n\n")
	for _, m := range metas {
		fmt.Fprintf(&b, "• %s — %s", m.Name, m.Title)
		if m.Level != "" {
			fmt.Fprintf(&b, " [%s]", m.Level)
		}
		b.WriteString("\n")
		if techniques := attackTags(m.Tags); techniques != "" {
			fmt.Fprintf(&b, "    %s\n", techniques)
		}
		if m.Description != "" {
			fmt.Fprintf(&b, "    %s\n", m.Description)
		}
	}
	return tool.Text(b.String()), nil
}

// attackTags returns the MITRE ATT&CK tags from a tag list, comma-joined.
func attackTags(tags []string) string {
	var out []string
	for _, t := range tags {
		if strings.HasPrefix(t, "attack.t") {
			out = append(out, strings.TrimPrefix(t, "attack."))
		}
	}
	return strings.Join(out, ", ")
}
