package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/tool"
)

// run is a helper that marshals an input map and runs a tool.
func run(t *testing.T, tl tool.Tool, input map[string]any) tool.Result {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	res, err := tl.Run(context.Background(), raw)
	if err != nil {
		t.Fatalf("%s.Run error: %v", tl.Name(), err)
	}
	return res
}

func TestWriteReadEditRoundTrip(t *testing.T) {
	dir := t.TempDir()
	w := &Write{Dir: dir}
	r := &Read{Dir: dir}
	e := &Edit{Dir: dir}

	if res := run(t, w, map[string]any{"path": "rule.yml", "content": "name: test\nseverity: Low\n"}); res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}
	if _, err := os.Stat(filepath.Join(dir, "rule.yml")); err != nil {
		t.Fatalf("file not written: %v", err)
	}

	res := run(t, r, map[string]any{"path": "rule.yml"})
	if res.IsError || !strings.Contains(res.Content, "severity: Low") {
		t.Fatalf("read missing content: %q", res.Content)
	}
	if !strings.Contains(res.Content, "     1\t") {
		t.Fatalf("read should number lines: %q", res.Content)
	}

	if res := run(t, e, map[string]any{"path": "rule.yml", "old_string": "Low", "new_string": "High"}); res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "rule.yml"))
	if !strings.Contains(string(got), "severity: High") {
		t.Fatalf("edit not applied: %q", string(got))
	}
}

func TestEditUniqueness(t *testing.T) {
	dir := t.TempDir()
	w := &Write{Dir: dir}
	e := &Edit{Dir: dir}
	run(t, w, map[string]any{"path": "f.txt", "content": "x\nx\n"})

	if res := run(t, e, map[string]any{"path": "f.txt", "old_string": "x", "new_string": "y"}); !res.IsError {
		t.Fatal("expected error editing non-unique string without replace_all")
	}
	if res := run(t, e, map[string]any{"path": "f.txt", "old_string": "x", "new_string": "y", "replace_all": true}); res.IsError {
		t.Fatalf("replace_all should succeed: %s", res.Content)
	}
}

func TestGlobAndGrep(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "detections", "a.yml"), "eventName: BatchUpdateFindings\n")
	mustWrite(t, filepath.Join(dir, "detections", "b.yml"), "eventName: GetObject\n")
	mustWrite(t, filepath.Join(dir, "notes.txt"), "nothing here\n")

	g := &Glob{Dir: dir}
	res := run(t, g, map[string]any{"pattern": "**/*.yml"})
	if res.IsError || !strings.Contains(res.Content, "a.yml") || !strings.Contains(res.Content, "b.yml") {
		t.Fatalf("glob missing matches: %q", res.Content)
	}
	if strings.Contains(res.Content, "notes.txt") {
		t.Fatalf("glob should not match notes.txt: %q", res.Content)
	}

	gr := &Grep{Dir: dir}
	res = run(t, gr, map[string]any{"pattern": "BatchUpdateFindings"})
	if res.IsError || !strings.Contains(res.Content, "a.yml") {
		t.Fatalf("grep missing match: %q", res.Content)
	}
	if strings.Contains(res.Content, "b.yml") {
		t.Fatalf("grep matched wrong file: %q", res.Content)
	}
}

func TestLS(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "sub", "x.yml"), "x\n")
	mustWrite(t, filepath.Join(dir, "top.txt"), "y\n")

	res := run(t, &LS{Dir: dir}, map[string]any{})
	if res.IsError || !strings.Contains(res.Content, "sub/") || !strings.Contains(res.Content, "top.txt") {
		t.Fatalf("ls output unexpected: %q", res.Content)
	}
}

func TestReadOnlyFlags(t *testing.T) {
	readOnly := map[string]bool{
		"read": true, "ls": true, "glob": true, "grep": true,
		"validate_detection": true, "reference_detection": true, "test_detection": true,
		"recall": true,
	}
	notReadOnly := map[string]bool{
		"bash": true, "write": true, "edit": true, "ntn": true,
		"set_detection_meta": true, "set_detection_logsource": true,
		"edit_detection_logic": true, "manage_detection_list": true,
		"set_detection_runbook": true, "manage_detection_tests": true,
	}
	for _, tl := range Toolbox(t.TempDir(), nil, "", nil).All() {
		if readOnly[tl.Name()] && !tl.ReadOnly() {
			t.Errorf("%s should be read-only", tl.Name())
		}
		if notReadOnly[tl.Name()] && tl.ReadOnly() {
			t.Errorf("%s should not be read-only", tl.Name())
		}
	}
}

// TestDetectionAuthoringRoundTrip builds a rule entirely with the field tools,
// then confirms it validates and its inline tests pass — the end-to-end path the
// agent follows.
func TestDetectionAuthoringRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Start from a minimal *valid* scaffold; the field tools refine it in place
	// while keeping it schema-valid at every step.
	mustWrite(t, filepath.Join(dir, "rule.yml"),
		"title: scaffold\nlogsource:\n  product: aws\ndetection:\n  selection:\n    placeholder: x\n  condition: selection\n")

	steps := []struct {
		tool  tool.Tool
		input map[string]any
	}{
		{&SetDetectionMeta{Dir: dir}, map[string]any{
			"path": "rule.yml", "title": "AWS Root Console Login",
			"id": "generate", "status": "experimental", "level": "high",
			"description": "Detects console logins by the AWS account root user.",
		}},
		{&SetDetectionLogsource{Dir: dir}, map[string]any{
			"path": "rule.yml", "product": "aws", "service": "cloudtrail",
		}},
		{&EditDetectionLogic{Dir: dir}, map[string]any{
			"path": "rule.yml", "selection": "selection",
			"fields":    map[string]any{"eventName": "ConsoleLogin", "userIdentity.type": "Root"},
			"condition": "selection",
		}},
		{&ManageDetectionList{Dir: dir}, map[string]any{
			"path": "rule.yml", "field": "tags", "add": "attack.t1078.004",
		}},
		{&SetDetectionRunbook{Dir: dir}, map[string]any{
			"path": "rule.yml", "triage": []string{"Confirm the login was the root user."},
			"contain": []string{"Rotate root credentials if unexpected."},
		}},
		{&ManageDetectionTests{Dir: dir}, map[string]any{
			"path": "rule.yml", "name": "root login fires",
			"event": map[string]any{"eventName": "ConsoleLogin", "userIdentity.type": "Root"},
			"match": true,
		}},
		{&ManageDetectionTests{Dir: dir}, map[string]any{
			"path": "rule.yml", "name": "iam user login ignored",
			"event": map[string]any{"eventName": "ConsoleLogin", "userIdentity.type": "IAMUser"},
			"match": false,
		}},
	}
	for _, s := range steps {
		res := run(t, s.tool, s.input)
		if res.IsError {
			t.Fatalf("%s failed: %s", s.tool.Name(), res.Content)
		}
		if !strings.Contains(res.Content, "valid against the Sigma schema") {
			t.Fatalf("%s did not report validation: %s", s.tool.Name(), res.Content)
		}
	}

	// The rule must now validate and its inline tests must all pass.
	if res := run(t, &ValidateDetection{Dir: dir}, map[string]any{"path": "rule.yml"}); res.IsError {
		t.Fatalf("final rule invalid: %s", res.Content)
	}
	res := run(t, &TestDetection{Dir: dir}, map[string]any{"path": "rule.yml"})
	if res.IsError || !strings.Contains(res.Content, "2 passed, 0 failed") {
		t.Fatalf("inline tests did not all pass: %s", res.Content)
	}

	// A generated id should be a real UUID, and comments/edits round-tripped.
	got, _ := os.ReadFile(filepath.Join(dir, "rule.yml"))
	if strings.Contains(string(got), "id: generate") {
		t.Fatalf("id 'generate' sentinel was not replaced with a UUID:\n%s", got)
	}
}

// TestManageTestsRemoveAndReplace covers removal and idempotent replace.
func TestManageTestsRemoveAndReplace(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "r.yml"),
		"title: t\nlogsource:\n  product: aws\ndetection:\n  selection:\n    a: 1\n  condition: selection\n")
	mt := &ManageDetectionTests{Dir: dir}

	run(t, mt, map[string]any{"path": "r.yml", "name": "c1", "event": map[string]any{"a": 1}, "match": true})
	// Re-adding the same name replaces rather than duplicating.
	run(t, mt, map[string]any{"path": "r.yml", "name": "c1", "event": map[string]any{"a": 2}, "match": false})
	got, _ := os.ReadFile(filepath.Join(dir, "r.yml"))
	if strings.Count(string(got), "name: c1") != 1 {
		t.Fatalf("expected one c1 case after replace:\n%s", got)
	}
	// Remove it.
	if res := run(t, mt, map[string]any{"path": "r.yml", "name": "c1", "remove": true}); res.IsError {
		t.Fatalf("remove failed: %s", res.Content)
	}
	if res := run(t, mt, map[string]any{"path": "r.yml", "name": "missing", "remove": true}); !res.IsError {
		t.Fatal("removing a missing case should error")
	}
}

// TestReferenceDetectionTool exercises the read-only reference tool.
func TestReferenceDetectionTool(t *testing.T) {
	rt := &ReferenceDetection{}
	list := run(t, rt, map[string]any{"list": true})
	if list.IsError || !strings.Contains(list.Content, "aws_cloudtrail_disable_logging") {
		t.Fatalf("list missing known reference: %s", list.Content)
	}
	one := run(t, rt, map[string]any{"name": "aws_cloudtrail_disable_logging"})
	if one.IsError || !strings.Contains(one.Content, "title:") || !strings.Contains(one.Content, "runbook:") {
		t.Fatalf("get did not return full exemplar YAML: %s", one.Content)
	}
	if res := run(t, rt, map[string]any{"name": "nope"}); !res.IsError {
		t.Fatal("unknown reference should error")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
