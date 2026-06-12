package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*/SKILL.md
var builtinFS embed.FS

// nameRE constrains a skill id so it is safe as a tool argument and a directory
// name.
var nameRE = regexp.MustCompile(`^[a-z0-9-]+$`)

// frontmatter is the parsed YAML header of a SKILL.md.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Load discovers skills from three roots, later roots overriding earlier ones by
// name so a project can override a user or builtin skill:
//
//   - builtin (embedded in the binary)
//   - <user config dir>/vala/skills/<name>/SKILL.md — user-wide
//   - <workdir>/.vala/skills/<name>/SKILL.md        — project, version-controlled
//
// Loading is best-effort like operator context: a missing or unreadable root is
// skipped, and a malformed SKILL.md is skipped (collected in the returned
// warnings) rather than failing the session.
func Load(workdir string) (*Set, []string) {
	set := NewSet()
	var warnings []string

	warnings = append(warnings, loadBuiltin(set)...)
	if dir, err := os.UserConfigDir(); err == nil {
		warnings = append(warnings, loadDir(set, filepath.Join(dir, "vala", "skills"), "user")...)
	}
	if workdir != "" {
		warnings = append(warnings, loadDir(set, filepath.Join(workdir, ".vala", "skills"), "project")...)
	}
	return set, warnings
}

// loadBuiltin reads the embedded skills into the set.
func loadBuiltin(set *Set) []string {
	var warnings []string
	entries, err := fs.ReadDir(builtinFS, "builtin")
	if err != nil {
		return []string{fmt.Sprintf("builtin skills: %v", err)}
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := e.Name()
		data, err := builtinFS.ReadFile("builtin/" + dir + "/SKILL.md")
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("builtin skill %s: %v", dir, err))
			continue
		}
		sk, err := parse(data, dir, "builtin")
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("builtin skill %s: %v", dir, err))
			continue
		}
		set.byName[sk.Name] = sk
	}
	return warnings
}

// loadDir reads SKILL.md files from <root>/<name>/SKILL.md on the filesystem.
// A missing root is not an error (returns no warnings).
func loadDir(set *Set, root, source string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil // absent or unreadable root: nothing to load
	}
	var warnings []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := e.Name()
		path := filepath.Join(root, dir, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue // a directory without a SKILL.md is simply not a skill
		}
		sk, err := parse(data, dir, source)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s skill %s: %v", source, dir, err))
			continue
		}
		set.byName[sk.Name] = sk
	}
	return warnings
}

// parse splits a SKILL.md into its YAML frontmatter and markdown body and
// validates it. The frontmatter name must be present, match ^[a-z0-9-]+$, and
// equal the directory name — so the id an operator types is always the folder
// they see.
func parse(data []byte, dir, source string) (Skill, error) {
	fmText, body, ok := splitFrontmatter(string(data))
	if !ok {
		return Skill{}, fmt.Errorf("missing YAML frontmatter delimited by ---")
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return Skill{}, fmt.Errorf("invalid frontmatter: %w", err)
	}
	if fm.Name == "" {
		return Skill{}, fmt.Errorf("frontmatter missing name")
	}
	if !nameRE.MatchString(fm.Name) {
		return Skill{}, fmt.Errorf("name %q must match ^[a-z0-9-]+$", fm.Name)
	}
	if fm.Name != dir {
		return Skill{}, fmt.Errorf("name %q must equal directory %q", fm.Name, dir)
	}
	return Skill{
		Name:        fm.Name,
		Description: strings.TrimSpace(fm.Description),
		Body:        strings.TrimSpace(body),
		Source:      source,
	}, nil
}

// splitFrontmatter separates a leading ---\n...\n--- YAML block from the markdown
// body that follows. It reports false when the document does not open with a
// frontmatter fence.
func splitFrontmatter(s string) (fm, body string, ok bool) {
	s = strings.TrimPrefix(s, "\uFEFF") // tolerate a UTF-8 BOM
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return "", "", false
	}
	rest := s[strings.IndexByte(s, '\n')+1:]
	// Find the closing fence at the start of a line.
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", false
	}
	fm = rest[:idx]
	after := rest[idx+1:] // starts at "---"
	if nl := strings.IndexByte(after, '\n'); nl >= 0 {
		body = after[nl+1:]
	} else {
		body = ""
	}
	return fm, body, true
}
