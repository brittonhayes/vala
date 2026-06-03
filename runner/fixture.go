// Package runner is vala's adversarial harness. It replays scenario fixtures
// through the real governance machine (the policy set, permission gate, approval
// ledger, and the actual tools with mocked I/O) in a deterministic "recorded"
// mode — no LLM — and scores each scenario against safety dimensions. A
// regression in the phase filter or approval policy makes a scenario fail, which
// is exactly what the harness is for.
package runner

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/brittonhayes/vala/internal/brain"
	"gopkg.in/yaml.v3"
)

// Fixture is one adversarial scenario.
type Fixture struct {
	Name     string           `yaml:"name"`
	Env      string           `yaml:"env"`
	Alert    brain.Alert      `yaml:"alert"`
	MockLogs []map[string]any `yaml:"mock_logs"`
	Steps    []Step           `yaml:"steps"`
	// Approve lists action tool names the (simulated) operator approves beyond
	// policy auto-approval.
	Approve  []string      `yaml:"approve"`
	CasePage *CasePageSpec `yaml:"case_page"`
	Expect   Expect        `yaml:"expect"`
}

// Step is one scripted tool call the simulated model makes, in a given phase.
type Step struct {
	Phase             string         `yaml:"phase"`
	Tool              string         `yaml:"tool"`
	Input             map[string]any `yaml:"input"`
	CaptureEvidenceAs string         `yaml:"capture_evidence_as"`
	// Adversarial is a human note describing the attack this step represents
	// (e.g. "return-channel injection"); it does not affect execution.
	Adversarial string `yaml:"adversarial"`
}

// CasePageSpec is the structured narrative the scenario asks to write.
type CasePageSpec struct {
	Summary    []brain.Claim `yaml:"summary"`
	Hypotheses []brain.Claim `yaml:"hypotheses"`
	NextSteps  []string      `yaml:"next_steps"`
}

// Expect is the assertion set for a scenario.
type Expect struct {
	// Denied lists tools that must be denied at least once.
	Denied []string `yaml:"denied"`
	// Executed lists action tools that must have executed.
	Executed []string `yaml:"executed"`
	// NotExecuted lists action tools that must never have executed.
	NotExecuted []string `yaml:"not_executed"`
	// EvidenceMin is the minimum number of evidence rows recorded.
	EvidenceMin int `yaml:"evidence_min"`
	// CasePageClean requires the case page to pass the evidence lint.
	CasePageClean *bool `yaml:"case_page_clean"`
	// SingleExecution lists action tools that must execute at most once even if
	// proposed/executed multiple times (idempotency).
	SingleExecution []string `yaml:"single_execution"`
}

// LoadDir loads every *.yaml fixture under root (recursively), sorted by path.
func LoadDir(root string) ([]Fixture, error) {
	var paths []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(p); ext == ".yaml" || ext == ".yml" {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []Fixture
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var fx Fixture
		if err := yaml.Unmarshal(raw, &fx); err != nil {
			return nil, err
		}
		out = append(out, fx)
	}
	return out, nil
}
