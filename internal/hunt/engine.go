// Package hunt drives a hypothesis-driven threat hunt. It opens a Hunt in the
// brain, runs the LLM-driven explore and conclude phases (each with a
// phase-filtered, read-mostly tool set), stores the hunt narrative, and can
// optionally promote a confirmed hunt into a Sigma detection. Hunting has no
// real-world action path, so it reuses the governance phase filter for tool
// exposure but never the propose/approval/execute machinery.
package hunt

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
	"github.com/brittonhayes/vala/internal/tools"
)

// Engine orchestrates a single threat hunt.
type Engine struct {
	Client *llm.Client
	Gate   *permission.Gate
	Brain  *brain.Client
	Policy *policy.Set

	Env              string
	Dir              string
	Commit           string
	MaxStepsPerPhase int

	// Promote, when true, authors a Sigma detection from a confirmed hunt and
	// links it back to the hunt in the brain.
	Promote bool

	// Events observes the underlying agent loop (optional).
	Events agent.Events
}

// Result summarizes a completed hunt.
type Result struct {
	HuntID            string
	Findings          []brain.Evidence
	Status            string
	PageURL           string
	DetectionPromoted bool
}

// RunHunt walks a question through the explore and conclude phases and returns a
// summary. The hunt is stored in the brain regardless of how the model behaves;
// a model that never calls store_hunt yields an Inconclusive hunt.
func (e *Engine) RunHunt(ctx context.Context, question string) (*Result, error) {
	if e.Gate.Policy == nil {
		e.Gate.Policy = e.Policy
	}
	maxSteps := e.MaxStepsPerPhase
	if maxSteps <= 0 {
		maxSteps = 20
	}

	huntID, err := e.Brain.OpenHunt(ctx, brain.Hunt{Question: question})
	if err != nil {
		return nil, fmt.Errorf("open hunt: %w", err)
	}
	runID, _ := e.Brain.StartRun(ctx, huntID, e.modelName(), e.Commit)

	rc := tools.NewHuntContext(e.Env, huntID, e.Brain, e.Policy)
	registry := tools.HuntRegistry(e.Dir, question, rc)
	ag := agent.New(e.Client, registry, e.Gate, e.Dir, maxSteps)

	res := &Result{HuntID: huntID}

	// --- Explore phase ------------------------------------------------------
	gov := agent.Governor{Phase: governance.PhaseEvidence, Ledger: rc.Ledger, Policy: e.Policy, Env: e.Env}
	emsg := explorePrompt(question)
	msgs := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(emsg))}
	if _, err := ag.RunPhase(ctx, msgs, emsg, gov, maxSteps, e.Events); err != nil {
		return res, fmt.Errorf("explore phase: %w", err)
	}
	res.Findings = rc.Evidence()

	// --- Conclude phase -----------------------------------------------------
	gov.Phase = governance.PhaseReport
	cmsg := concludePrompt(question, res.Findings)
	msgs = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(cmsg))}
	if _, err := ag.RunPhase(ctx, msgs, cmsg, gov, maxSteps, e.Events); err != nil {
		return res, fmt.Errorf("conclude phase: %w", err)
	}

	// store_hunt closes the hunt; if the model never called it, close the hunt
	// Inconclusive so the brain row is always finalized.
	status, url := rc.HuntOutcome()
	if status == "" {
		status = brain.HuntInconclusive
		_ = e.Brain.CloseHunt(ctx, huntID, status, "hunt ended without a stored verdict")
	}
	res.Status = status
	res.PageURL = url

	_ = e.Brain.EndRun(ctx, runID, string(governance.PhaseReport), 0, 0)

	// --- Promote (optional) -------------------------------------------------
	if e.Promote && status == brain.HuntConfirmed {
		if err := e.promote(ctx, huntID, question, res.Findings); err != nil {
			return res, fmt.Errorf("promote to detection: %w", err)
		}
		res.DetectionPromoted = true
	}

	return res, nil
}

// promote authors a Sigma detection from a confirmed hunt using the existing
// detection-authoring tools (the same single-phase path as `vala run`), records
// a Detections row, and links it back to the hunt. File writes go through the
// permission gate exactly as in a normal authoring run.
func (e *Engine) promote(ctx context.Context, huntID, question string, findings []brain.Evidence) error {
	registry := tools.Default(e.Dir)
	ag := agent.New(e.Client, registry, e.Gate, e.Dir, e.MaxStepsPerPhase)
	prompt := promotePrompt(question, findings)
	if _, err := ag.Run(ctx, nil, prompt, e.Events); err != nil {
		return err
	}
	detID, err := e.Brain.RecordDetection(ctx, brain.DetectionRef{
		Title: question,
		Hunts: []string{huntID},
	})
	if err != nil {
		return err
	}
	return e.Brain.Link(ctx, huntID, "detections", detID)
}

func (e *Engine) modelName() string {
	if e.Client == nil {
		return ""
	}
	return e.Client.Model()
}
