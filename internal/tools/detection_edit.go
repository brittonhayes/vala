package tools

import (
	"fmt"
	"os"
	"strings"

	"github.com/brittonhayes/vala/internal/detect"
	"github.com/brittonhayes/vala/internal/sigma"
	"github.com/brittonhayes/vala/internal/tool"
)

// editDetection loads the Sigma rule at path, applies mutate to a comment- and
// order-preserving editor, writes it back, and re-validates against the Sigma
// schema. It returns a concise confirmation (the summary plus validation
// status) — never the whole file — so field edits stay cheap on tokens.
//
// All field-editing tools funnel through here so they share one load → mutate →
// validate → write pipeline and one result shape.
func editDetection(dir, path, summary string, mutate func(*sigma.Editor) error) (tool.Result, error) {
	if strings.TrimSpace(path) == "" {
		return tool.Errorf("provide a rule file 'path'"), nil
	}
	full := resolve(dir, path)
	data, err := os.ReadFile(full)
	if err != nil {
		return tool.Errorf("cannot read %s: %v", path, err), nil
	}
	ed, err := sigma.Load(data)
	if err != nil {
		return tool.Errorf("%s: %v", path, err), nil
	}
	if err := mutate(ed); err != nil {
		return tool.Errorf("%v", err), nil
	}
	out, err := ed.Bytes()
	if err != nil {
		return tool.Errorf("serialize %s: %v", path, err), nil
	}
	if err := os.WriteFile(full, out, 0o644); err != nil {
		return tool.Errorf("write %s: %v", path, err), nil
	}

	issues, err := detect.ValidateBytes(out)
	if err != nil {
		return tool.Errorf("validation error: %v", err), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s in %s\n", summary, path)
	if len(issues) == 0 {
		b.WriteString("✓ rule is valid against the Sigma schema")
		return tool.Text(b.String()), nil
	}
	b.WriteString("✗ rule is now INVALID — fix these before finishing:\n")
	for _, is := range issues {
		fmt.Fprintf(&b, "    - %s\n", is.String())
	}
	res := tool.Text(b.String())
	res.IsError = true
	return res, nil
}
