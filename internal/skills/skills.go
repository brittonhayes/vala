// Package skills implements vala's skills runtime: Claude-Code-style capability
// packs that specialize a mode. A skill is a SKILL.md file — YAML frontmatter
// (name, description) plus a markdown body — discovered from the binary, the
// user config dir, and the project. Modes bundle skills by id; the agent lists
// the active ones in the system prompt by name and description (progressive
// disclosure) and the "skill" tool returns a body in full on demand, so a long
// playbook costs prompt tokens only when it is actually needed.
package skills

import "sort"

// Skill is one discovered capability pack.
type Skill struct {
	// Name is the id, taken from frontmatter and required to equal the skill's
	// directory name. It matches ^[a-z0-9-]+$ and is how modes reference it.
	Name string
	// Description is the one-line summary listed in the prompt for progressive
	// disclosure — enough to decide whether to load the body.
	Description string
	// Body is the full markdown guidance returned by the "skill" tool on demand.
	Body string
	// Source records where the skill was discovered: "builtin", "user", or
	// "project". Used for debugging and to explain override precedence.
	Source string
}

// Set is the resolved skill catalog for a session, keyed by name.
type Set struct {
	byName map[string]Skill
}

// NewSet builds a Set from skills, later entries overriding earlier ones by name
// (so a project skill overrides a same-named builtin).
func NewSet(skills ...Skill) *Set {
	s := &Set{byName: make(map[string]Skill, len(skills))}
	for _, sk := range skills {
		s.byName[sk.Name] = sk
	}
	return s
}

// Get returns the skill with the given name.
func (s *Set) Get(name string) (Skill, bool) {
	if s == nil {
		return Skill{}, false
	}
	sk, ok := s.byName[name]
	return sk, ok
}

// ByIDs returns the skills for the given ids, in id order, skipping any that are
// not present. It is how a mode resolves its bundled-skill metadata for the
// prompt's Skills section.
func (s *Set) ByIDs(ids []string) []Skill {
	if s == nil {
		return nil
	}
	out := make([]Skill, 0, len(ids))
	for _, id := range ids {
		if sk, ok := s.byName[id]; ok {
			out = append(out, sk)
		}
	}
	return out
}

// All returns every discovered skill, sorted by name for stable output.
func (s *Set) All() []Skill {
	if s == nil {
		return nil
	}
	out := make([]Skill, 0, len(s.byName))
	for _, sk := range s.byName {
		out = append(out, sk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
