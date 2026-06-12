package agent

import (
	"os"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/mode"
	"github.com/brittonhayes/vala/internal/skills"
)

// huntPrompt is a small helper that renders the default (hunt) mode prompt.
func huntPrompt(workdir string, tools []string, maturity int, ctx string) string {
	return SystemPrompt(mode.Default(), mode.PromptInput{
		Workdir: workdir, ToolNames: tools, MaturityLevel: maturity,
	}, nil, ctx)
}

// TestSystemPromptEnumeratesLoopAndTiers guards the contract that the hunt
// prompt presents the full eight-stage loop and the five detection-output tiers.
func TestSystemPromptEnumeratesLoopAndTiers(t *testing.T) {
	p := huntPrompt("/work", []string{"open_hunt", "validate_data", "store_hunt", "update_coverage"}, 1, "")

	stages := []string{
		"Scope & prioritize",
		"Form hypothesis",
		"Plan & validate data",
		"Execute & analyze",
		"Deep dive",
		"Document & decide",
		"Convert to detection",
		"Feed back",
	}
	for _, s := range stages {
		if !strings.Contains(p, s) {
			t.Errorf("system prompt missing loop stage %q", s)
		}
	}

	tiers := []string{
		"tier1_automated",
		"tier2_triage",
		"tier3_recurring_hunt",
		"tier4_playbook",
		"tier5_none_documented",
	}
	for _, tr := range tiers {
		if !strings.Contains(p, tr) {
			t.Errorf("system prompt missing detection tier %q", tr)
		}
	}
}

func TestSystemPromptMaturityFraming(t *testing.T) {
	if p := huntPrompt("/w", []string{"read"}, 0, ""); !strings.Contains(p, "HMM0") {
		t.Error("HMM0 prompt should frame investigate-only autonomy")
	}
	if p := huntPrompt("/w", []string{"read"}, 4, ""); !strings.Contains(p, "autonomously") {
		t.Error("HMM4 prompt should frame autonomous operation")
	}
}

// TestHuntPromptGolden is the backward-compatibility contract: the hunt mode
// prompt must reproduce the pre-modes output byte-for-byte for fixed inputs. The
// golden files were captured from the original SystemPrompt before the modes
// refactor (regenerate with -update only for an intentional change).
func TestHuntPromptGolden(t *testing.T) {
	cases := []struct {
		golden   string
		workdir  string
		tools    []string
		maturity int
		ctx      string
	}{
		{"hunt_m1_ctx.golden", "/work", []string{"open_hunt", "validate_data", "store_hunt", "read", "write"}, 1, "STANDING CONTEXT HERE"},
		{"hunt_m0_noctx.golden", "/srv/repo", []string{"read", "recall"}, 0, ""},
		{"hunt_m4_noctx.golden", "/srv/repo", []string{"read", "recall"}, 4, ""},
	}
	for _, c := range cases {
		t.Run(c.golden, func(t *testing.T) {
			got := huntPrompt(c.workdir, c.tools, c.maturity, c.ctx)
			want, err := os.ReadFile("testdata/" + c.golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("hunt prompt drifted from golden %s.\n--- got ---\n%s", c.golden, got)
			}
		})
	}
}

// TestDetectPromptIsSpecialized confirms the detect mode produces a distinct,
// detection-focused prompt and that bundled skills surface in the Skills section.
func TestDetectPromptIsSpecialized(t *testing.T) {
	m, ok := mode.Get("detect")
	if !ok {
		t.Fatal("detect mode not registered")
	}
	active := []skills.Skill{{Name: "sigma-authoring", Description: "Sigma rule authoring checklist."}}
	p := SystemPrompt(m, mode.PromptInput{Workdir: "/w", ToolNames: []string{"validate_detection", "skill"}, MaturityLevel: 1}, active, "")

	if !strings.Contains(p, "Detection Engineering") {
		t.Error("detect prompt should announce Detection Engineering mode")
	}
	if strings.Contains(p, "# The hunt loop") {
		t.Error("detect prompt should not include the hunt loop")
	}
	if !strings.Contains(p, "# Skills") || !strings.Contains(p, "sigma-authoring") {
		t.Error("detect prompt should list bundled skills in a Skills section")
	}
}
