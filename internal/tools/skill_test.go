package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/skills"
)

func runSkill(t *testing.T, set *skills.Set, input string) (string, bool) {
	t.Helper()
	tl := &Skill{Set: set}
	res, err := tl.Run(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Content, res.IsError
}

func TestSkillListsAndLoads(t *testing.T) {
	set := skills.NewSet(
		skills.Skill{Name: "sigma-authoring", Description: "authoring checklist", Body: "FULL BODY"},
	)

	// List.
	content, isErr := runSkill(t, set, `{"list": true}`)
	if isErr {
		t.Fatal("list should not error")
	}
	if !strings.Contains(content, "sigma-authoring") || !strings.Contains(content, "authoring checklist") {
		t.Errorf("list missing skill: %q", content)
	}

	// Load by name returns the full body.
	content, isErr = runSkill(t, set, `{"name": "sigma-authoring"}`)
	if isErr {
		t.Fatal("loading a known skill should not error")
	}
	if content != "FULL BODY" {
		t.Errorf("body = %q, want FULL BODY", content)
	}
}

func TestSkillUnknownNameErrors(t *testing.T) {
	set := skills.NewSet(skills.Skill{Name: "sigma-authoring", Description: "x", Body: "y"})
	content, isErr := runSkill(t, set, `{"name": "nope"}`)
	if !isErr {
		t.Error("unknown skill should return an error result")
	}
	if !strings.Contains(content, "list") {
		t.Errorf("error should hint at {\"list\": true}: %q", content)
	}
}

func TestSkillEmptySet(t *testing.T) {
	content, isErr := runSkill(t, skills.NewSet(), `{"list": true}`)
	if isErr {
		t.Error("empty set list should not error")
	}
	if !strings.Contains(content, "no skills") {
		t.Errorf("expected a no-skills message, got %q", content)
	}
}
