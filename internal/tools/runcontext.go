package tools

import (
	"context"
	"sync"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/policy"
)

// RunContext is the per-incident state shared by the governance control/case
// tools (record_evidence, propose_action, submit_for_approval, write_case_page)
// and read by the respond engine. One RunContext exists per `vala respond` run.
type RunContext struct {
	Env    string
	CaseID string
	Brain  *brain.Client
	Ledger *governance.Ledger
	Policy *policy.Set

	// Notifier sends comms for slack_notify; nil falls back to a no-op record.
	Notifier Notifier

	mu        sync.Mutex
	evidence  []brain.Evidence
	actions   map[string]*brain.Action
	rowIDs    map[string]string // action ID -> brain Actions row ID
	submitted bool
}

// NewRunContext builds a RunContext.
func NewRunContext(env, caseID string, b *brain.Client, led *governance.Ledger, pol *policy.Set) *RunContext {
	return &RunContext{
		Env:     env,
		CaseID:  caseID,
		Brain:   b,
		Ledger:  led,
		Policy:  pol,
		actions: map[string]*brain.Action{},
		rowIDs:  map[string]string{},
	}
}

func (rc *RunContext) addEvidence(e brain.Evidence) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.evidence = append(rc.evidence, e)
}

// Evidence returns the evidence collected so far.
func (rc *RunContext) Evidence() []brain.Evidence {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := make([]brain.Evidence, len(rc.evidence))
	copy(out, rc.evidence)
	return out
}

func (rc *RunContext) knownEvidence(id string) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, e := range rc.evidence {
		if e.ID == id {
			return true
		}
	}
	return false
}

func (rc *RunContext) addAction(a *brain.Action, rowID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.actions[a.ID] = a
	rc.rowIDs[a.ID] = rowID
}

// SetActionStatus updates a proposed action's status both in memory (so the case
// page reflects it) and on its Actions row in the brain.
func (rc *RunContext) SetActionStatus(ctx context.Context, actionID, status, by, result string) {
	rc.mu.Lock()
	a := rc.actions[actionID]
	if a != nil {
		a.Status = status
	}
	rowID := rc.rowIDs[actionID]
	rc.mu.Unlock()
	if rowID != "" {
		_ = rc.Brain.UpdateActionStatus(ctx, rowID, status, by, result)
	}
}

// Actions returns the proposed actions collected so far.
func (rc *RunContext) Actions() []brain.Action {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := make([]brain.Action, 0, len(rc.actions))
	for _, a := range rc.actions {
		out = append(out, *a)
	}
	return out
}

func (rc *RunContext) markSubmitted() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.submitted = true
}

// Submitted reports whether the model has submitted its proposals for approval.
func (rc *RunContext) Submitted() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.submitted
}

// Notifier sends a notification for an approved action.
type Notifier interface {
	Notify(message string) (pointer string, err error)
}
