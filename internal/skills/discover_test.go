package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkill creates <root>/.vala/skills/<name>/SKILL.md with the given content.
func writeProjectSkill(t *testing.T, workdir, name, content string) {
	t.Helper()
	dir := filepath.Join(workdir, ".vala", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFindsBuiltin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	set, warns := Load(t.TempDir())
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	sk, ok := set.Get("sigma-authoring")
	if !ok {
		t.Fatal("builtin sigma-authoring skill should be discovered")
	}
	if sk.Source != "builtin" {
		t.Errorf("source = %q, want builtin", sk.Source)
	}
	if sk.Description == "" || sk.Body == "" {
		t.Error("builtin skill should have a description and a body")
	}
}

func TestProjectOverridesBuiltin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	work := t.TempDir()
	writeProjectSkill(t, work, "sigma-authoring",
		"---\nname: sigma-authoring\ndescription: project override\n---\nproject body\n")

	set, _ := Load(work)
	sk, _ := set.Get("sigma-authoring")
	if sk.Source != "project" {
		t.Errorf("project skill should override builtin; source = %q", sk.Source)
	}
	if sk.Description != "project override" {
		t.Errorf("description = %q, want project override", sk.Description)
	}
}

func TestMalformedSkillIsSkipped(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	work := t.TempDir()
	// Frontmatter name does not match the directory name -> rejected, not fatal.
	writeProjectSkill(t, work, "good", "---\nname: good\ndescription: ok\n---\nbody\n")
	writeProjectSkill(t, work, "mismatch", "---\nname: other\ndescription: bad\n---\nbody\n")
	writeProjectSkill(t, work, "nofrontmatter", "just a body, no fence\n")

	set, warns := Load(work)
	if _, ok := set.Get("good"); !ok {
		t.Error("valid skill should load alongside malformed ones")
	}
	if _, ok := set.Get("other"); ok {
		t.Error("name/dir mismatch should be rejected")
	}
	if len(warns) < 2 {
		t.Errorf("expected warnings for the two malformed skills, got %v", warns)
	}
}

// TestBuiltinSkillsValid guards that every embedded skill parses and that its
// frontmatter name equals its directory — the same invariant the loader enforces.
func TestBuiltinSkillsValid(t *testing.T) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one builtin skill")
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := builtinFS.ReadFile("builtin/" + e.Name() + "/SKILL.md")
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if _, err := parse(data, e.Name(), "builtin"); err != nil {
			t.Errorf("builtin skill %s invalid: %v", e.Name(), err)
		}
	}
}
