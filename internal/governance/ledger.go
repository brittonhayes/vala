package governance

import "sync"

// Decision status for an action in the ledger.
const (
	StatusProposed = "proposed"
	StatusApproved = "approved"
	StatusDenied   = "denied"
	StatusExecuted = "executed"
	StatusFailed   = "failed"
)

// Ledger records the proposed → approved/denied → executed lifecycle of actions
// within a single run and binds approvals to specific action IDs. It is the
// authoritative record the permission gate consults before executing a write
// action: an action runs only if it is approved and not already executed.
type Ledger struct {
	mu       sync.Mutex
	proposed map[string]ProposedAction
	approved map[string]bool
	denied   map[string]bool
	executed map[string]bool
	approver map[string]string
}

// NewLedger returns an empty ledger.
func NewLedger() *Ledger {
	return &Ledger{
		proposed: map[string]ProposedAction{},
		approved: map[string]bool{},
		denied:   map[string]bool{},
		executed: map[string]bool{},
		approver: map[string]string{},
	}
}

// Propose records an action proposal. Re-proposing the same action (identical
// ID) is idempotent and does not reset any prior approval/denial.
func (l *Ledger) Propose(a ProposedAction) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.proposed[a.ID]; !ok {
		l.proposed[a.ID] = a
	}
}

// Proposed returns the proposals recorded so far, in no particular order.
func (l *Ledger) Proposed() []ProposedAction {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]ProposedAction, 0, len(l.proposed))
	for _, a := range l.proposed {
		out = append(out, a)
	}
	return out
}

// Approve marks an action approved by the named approver. An approval only
// applies to the exact action ID it was granted for.
func (l *Ledger) Approve(actionID, by string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.approved[actionID] = true
	delete(l.denied, actionID)
	l.approver[actionID] = by
}

// Deny marks an action denied.
func (l *Ledger) Deny(actionID, by string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.denied[actionID] = true
	delete(l.approved, actionID)
	l.approver[actionID] = by
}

// Satisfied reports whether an action may execute right now: it must be
// approved and not yet executed.
func (l *Ledger) Satisfied(actionID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.approved[actionID] && !l.executed[actionID]
}

// Status returns the lifecycle status of an action ID.
func (l *Ledger) Status(actionID string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.executed[actionID]:
		return StatusExecuted
	case l.denied[actionID]:
		return StatusDenied
	case l.approved[actionID]:
		return StatusApproved
	case func() bool { _, ok := l.proposed[actionID]; return ok }():
		return StatusProposed
	default:
		return ""
	}
}

// Approver returns who decided an action, if recorded.
func (l *Ledger) Approver(actionID string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.approver[actionID]
}

// MarkExecuted records that an action ran, enforcing execute-at-most-once. It
// returns false if the action was already executed (a replayed execution).
func (l *Ledger) MarkExecuted(actionID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.executed[actionID] {
		return false
	}
	l.executed[actionID] = true
	return true
}
