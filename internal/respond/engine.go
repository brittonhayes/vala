// Package respond drives an alert through vala's phase-separated governance
// loop. It opens a Case in the brain, runs the LLM-driven evidence/propose/report
// phases (each with a shrinking, policy-filtered tool set), and runs the
// approval and execute phases deterministically in code so that no write action
// ever runs without an approval on record.
package respond

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/policy"
	"github.com/brittonhayes/vala/internal/tool"
	"github.com/brittonhayes/vala/internal/tools"
)

// Engine orchestrates a single governed incident-response run.
type Engine struct {
	Client *llm.Client
	Gate   *permission.Gate
	Brain  *brain.Client
	Policy *policy.Set

	Env              string
	Dir              string
	Commit           string
	Webhook          string
	MaxStepsPerPhase int

	// Approver decides a proposed action that policy does not auto-approve. If
	// nil, only policy-auto-approved actions are approved (everything else is
	// denied) — the safe default for unattended runs.
	Approver func(governance.ProposedAction) bool

	// Events observes the underlying agent loop (optional).
	Events agent.Events
}

// Result summarizes a completed run.
type Result struct {
	CaseID       string
	RunID        string
	PhaseReached governance.Phase
	Evidence     []brain.Evidence
	Actions      []brain.Action
	Executed     []string // action IDs executed
	Violations   []string
}

// RunCase walks the alert through the full loop and returns a summary.
func (e *Engine) RunCase(ctx context.Context, alert brain.Alert, title string) (*Result, error) {
	if e.Gate.Policy == nil {
		e.Gate.Policy = e.Policy
	}
	maxSteps := e.MaxStepsPerPhase
	if maxSteps <= 0 {
		maxSteps = 20
	}

	caseID, err := e.Brain.OpenCase(ctx, alert, title)
	if err != nil {
		return nil, fmt.Errorf("open case: %w", err)
	}
	runID, _ := e.Brain.StartRun(ctx, caseID, e.modelName(), e.Commit)

	ledger := governance.NewLedger()
	rc := tools.NewRunContext(e.Env, caseID, e.Brain, ledger, e.Policy)
	registry := tools.GovernedRegistry(e.Dir, rc, e.Webhook)
	ag := agent.New(e.Client, registry, e.Gate, e.Dir, maxSteps)

	res := &Result{CaseID: caseID, RunID: runID, PhaseReached: governance.PhasePlan}

	// --- Evidence phase -----------------------------------------------------
	gov := agent.Governor{Phase: governance.PhaseEvidence, Ledger: ledger, Policy: e.Policy, Env: e.Env}
	msgs := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(evidencePrompt(alert)))}
	if _, err := ag.RunPhase(ctx, msgs, evidencePrompt(alert), gov, maxSteps, e.Events); err != nil {
		return res, fmt.Errorf("evidence phase: %w", err)
	}
	res.PhaseReached = governance.PhaseEvidence
	res.Evidence = rc.Evidence()

	// --- Propose phase ------------------------------------------------------
	gov.Phase = governance.PhasePropose
	pmsg := proposePrompt(res.Evidence)
	msgs = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(pmsg))}
	if _, err := ag.RunPhase(ctx, msgs, pmsg, gov, maxSteps, e.Events); err != nil {
		return res, fmt.Errorf("propose phase: %w", err)
	}
	res.PhaseReached = governance.PhasePropose
	res.Actions = rc.Actions()

	// --- Approval phase (deterministic) -------------------------------------
	e.approve(ctx, ledger, rc)
	res.PhaseReached = governance.PhaseApproval

	// --- Execute phase (deterministic) --------------------------------------
	res.Executed = e.execute(ctx, registry, ledger, rc)
	res.PhaseReached = governance.PhaseExecute

	// --- Report phase -------------------------------------------------------
	gov.Phase = governance.PhaseReport
	rmsg := reportPrompt(rc.Evidence(), rc.Actions())
	msgs = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(rmsg))}
	if _, err := ag.RunPhase(ctx, msgs, rmsg, gov, maxSteps, e.Events); err != nil {
		return res, fmt.Errorf("report phase: %w", err)
	}
	res.PhaseReached = governance.PhaseReport
	res.Actions = rc.Actions()

	_ = e.Brain.EndRun(ctx, runID, string(res.PhaseReached), 0, len(res.Violations))
	return res, nil
}

// ApproveAndExecute runs the deterministic approval and execute phases against an
// already-populated ledger and run context. It is exported so the harness runner
// can drive the same gate-enforced settlement that `vala respond` uses.
func (e *Engine) ApproveAndExecute(ctx context.Context, registry *tool.Registry, ledger *governance.Ledger, rc *tools.RunContext) []string {
	e.approve(ctx, ledger, rc)
	return e.execute(ctx, registry, ledger, rc)
}

// approve decides each proposed action: policy auto-approval first, then the
// operator approver if present; otherwise denied. Decisions are recorded in the
// ledger and mirrored to the Actions rows.
func (e *Engine) approve(ctx context.Context, ledger *governance.Ledger, rc *tools.RunContext) {
	for _, p := range ledger.Proposed() {
		approved := false
		switch {
		case e.Policy.AutoApprove(e.Env, p.Tool):
			approved = true
			ledger.Approve(p.ID, "policy:auto_approve")
		case e.Approver != nil && e.Approver(p):
			approved = true
			ledger.Approve(p.ID, "operator")
		default:
			ledger.Deny(p.ID, "policy")
		}
		status := governance.StatusApproved
		if !approved {
			status = governance.StatusDenied
		}
		rc.SetActionStatus(ctx, p.ID, status, ledger.Approver(p.ID), "")
	}
}

// execute runs every approved action through the permission gate (which is the
// authoritative check) and records the outcome. It returns the executed IDs.
func (e *Engine) execute(ctx context.Context, registry *tool.Registry, ledger *governance.Ledger, rc *tools.RunContext) []string {
	var executed []string
	for _, p := range ledger.Proposed() {
		if !ledger.Satisfied(p.ID) {
			continue
		}
		req := governance.Request{
			Tool: p.Tool, Summary: p.Rationale, Phase: governance.PhaseExecute,
			Class: e.Policy.ClassOf(p.Tool), ActionID: p.ID, Env: e.Env,
		}
		if d := e.Gate.Decide(req, ledger); !d.Allow {
			rc.SetActionStatus(ctx, p.ID, governance.StatusFailed, "", "blocked: "+d.Reason)
			continue
		}
		ledger.MarkExecuted(p.ID)
		t, ok := registry.Get(p.Tool)
		if !ok {
			rc.SetActionStatus(ctx, p.ID, governance.StatusFailed, "", "no such tool")
			continue
		}
		out, err := t.Run(ctx, p.Input)
		if err != nil || out.IsError {
			msg := out.Content
			if err != nil {
				msg = err.Error()
			}
			rc.SetActionStatus(ctx, p.ID, governance.StatusFailed, "", msg)
			continue
		}
		rc.SetActionStatus(ctx, p.ID, governance.StatusExecuted, "", out.Content)
		executed = append(executed, p.ID)
	}
	return executed
}

func (e *Engine) modelName() string {
	if e.Client == nil {
		return ""
	}
	return e.Client.Model()
}
