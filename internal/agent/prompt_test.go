package agent

import (
	"strings"
	"testing"
)

// TestSystemPromptEnumeratesLoopAndTiers guards the contract that the prompt
// presents the full eight-stage PEAK loop and the five detection-output tiers.
func TestSystemPromptEnumeratesLoopAndTiers(t *testing.T) {
	p := SystemPrompt("/work", []string{"open_hunt", "validate_data", "store_hunt", "update_coverage"}, 1, "")

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
	if p := SystemPrompt("/w", []string{"read"}, 0, ""); !strings.Contains(p, "HMM0") {
		t.Error("HMM0 prompt should frame investigate-only autonomy")
	}
	if p := SystemPrompt("/w", []string{"read"}, 4, ""); !strings.Contains(p, "autonomously") {
		t.Error("HMM4 prompt should frame autonomous operation")
	}
}
