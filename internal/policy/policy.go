// Package policy loads and evaluates vala's governance policy: which tool
// classes are exposed in which phase, which tools are hard-denied per
// environment, and which actions require human approval. Policy is enforced in
// code (the agent's tool-exposure filter and the permission gate read a
// policy.Set); the YAML files under policies/ are the editable source of truth,
// and a built-in default keeps the runtime safe if they are absent.
package policy

import (
	"os"
	"path/filepath"

	"github.com/brittonhayes/vala/internal/governance"
	"gopkg.in/yaml.v3"
)

// Tools is the parsed shape of policies/tools.yaml.
type Tools struct {
	// Classes maps a tool class name to the tools in it.
	Classes map[string][]string `yaml:"classes"`
	// Environments maps an env name to its per-env rules (deny lists).
	Environments map[string]EnvRule `yaml:"environments"`
}

// EnvRule is the per-environment tool policy.
type EnvRule struct {
	Deny []string `yaml:"deny"`
}

// Decision is the parsed shape of policies/decision.yaml.
type Decision struct {
	Defaults           DecisionDefaults          `yaml:"defaults"`
	Actions            map[string]ActionApproval `yaml:"actions"`
	RequireEvidenceFor []string                  `yaml:"require_evidence_for"`
	Forbidden          []string                  `yaml:"forbidden"`
}

// DecisionDefaults holds fall-through approval defaults.
type DecisionDefaults struct {
	RequireApproval bool `yaml:"require_approval"`
}

// ActionApproval is the approval rule for one action/tool.
type ActionApproval struct {
	RequireApproval bool     `yaml:"require_approval"`
	AutoApproveIn   []string `yaml:"auto_approve_in"`
}

// Set is the evaluated policy used at runtime.
type Set struct {
	tools    Tools
	decision Decision

	classOf map[string]governance.ToolClass // tool -> class
}

// Default returns the built-in policy. It mirrors policies/tools.yaml and
// policies/decision.yaml and is used when those files are not present.
func Default() *Set {
	t := Tools{
		Classes: map[string][]string{
			"read": {
				"read", "ls", "glob", "grep",
				"log_search", "reference_detection",
				"validate_detection", "test_detection",
				"recall",
			},
			"case_write": {
				"open_case", "record_evidence", "write_case_page",
				"queue_hunt", "open_hunt", "record_finding", "store_hunt", "record_intel", "link_artifacts",
			},
			"control":        {"propose_action", "submit_for_approval"},
			"action_execute": {"slack_notify", "bash", "write", "edit", "ntn"},
		},
		Environments: map[string]EnvRule{
			"dev":  {Deny: nil},
			"prod": {Deny: []string{"bash", "write", "edit", "ntn"}},
		},
	}
	d := Decision{
		Defaults: DecisionDefaults{RequireApproval: true},
		Actions: map[string]ActionApproval{
			"slack_notify": {RequireApproval: false, AutoApproveIn: []string{"dev"}},
		},
		RequireEvidenceFor: []string{"slack_notify"},
		Forbidden: []string{
			"executing_actions_outside_execute_phase",
			"credential_values_in_actions_or_narrative",
			"exfiltration_of_evidence_pointers_to_unapproved_destinations",
		},
	}
	return build(t, d)
}

// Load reads policies/tools.yaml and policies/decision.yaml from dir, falling
// back to the built-in default for any file that is missing. A present but
// malformed file is an error.
func Load(dir string) (*Set, error) {
	def := Default()
	t := def.tools
	d := def.decision

	if raw, err := os.ReadFile(filepath.Join(dir, "policies", "tools.yaml")); err == nil {
		var parsed Tools
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		t = parsed
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if raw, err := os.ReadFile(filepath.Join(dir, "policies", "decision.yaml")); err == nil {
		var parsed Decision
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		d = parsed
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return build(t, d), nil
}

func build(t Tools, d Decision) *Set {
	classOf := make(map[string]governance.ToolClass)
	for class, names := range t.Classes {
		for _, n := range names {
			classOf[n] = governance.ToolClass(class)
		}
	}
	return &Set{tools: t, decision: d, classOf: classOf}
}

// ClassOf returns a tool's class. Unknown tools default to the most restricted
// class (action_execute) so a newly added or misclassified tool fails closed:
// it will not be exposed during investigation and will require Execute phase +
// approval.
func (s *Set) ClassOf(toolName string) governance.ToolClass {
	if c, ok := s.classOf[toolName]; ok {
		return c
	}
	return governance.ClassActionExecute
}

// EnvDenied reports whether a tool is hard-denied in the given environment,
// regardless of phase or approval.
func (s *Set) EnvDenied(env, toolName string) bool {
	rule, ok := s.tools.Environments[env]
	if !ok {
		return false
	}
	for _, name := range rule.Deny {
		if name == toolName {
			return true
		}
	}
	return false
}

// ExposeInPhase reports whether a tool may be shown to the model in a phase for
// a given environment. This drives the per-phase tool-exposure filter.
func (s *Set) ExposeInPhase(toolName string, phase governance.Phase, env string) bool {
	if s.EnvDenied(env, toolName) {
		return false
	}
	return s.ClassOf(toolName).ExposedIn(phase)
}

// ApprovalRequired reports whether executing a tool requires a recorded
// approval in the given environment. An action listed with auto_approve_in for
// this env is treated as not requiring human approval (policy auto-approval).
func (s *Set) ApprovalRequired(env, toolName string) bool {
	rule, ok := s.decision.Actions[toolName]
	if !ok {
		return s.decision.Defaults.RequireApproval
	}
	if !rule.RequireApproval {
		return false
	}
	for _, e := range rule.AutoApproveIn {
		if e == env {
			return false
		}
	}
	return true
}

// AutoApprove reports whether an action should be auto-approved by policy in the
// given environment (used to satisfy the ledger in unattended/CI runs).
func (s *Set) AutoApprove(env, toolName string) bool {
	rule, ok := s.decision.Actions[toolName]
	if !ok {
		return false
	}
	for _, e := range rule.AutoApproveIn {
		if e == env {
			return true
		}
	}
	return !rule.RequireApproval
}

// RequiresEvidence reports whether a proposed action must cite at least one
// piece of evidence.
func (s *Set) RequiresEvidence(toolName string) bool {
	for _, name := range s.decision.RequireEvidenceFor {
		if name == toolName {
			return true
		}
	}
	return false
}

// Forbidden returns the configured forbidden-behavior identifiers.
func (s *Set) Forbidden() []string { return s.decision.Forbidden }
