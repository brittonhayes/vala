package tools

import (
	"sync"

	"github.com/brittonhayes/vala/internal/brain"
)

// RunContext is the per-run state shared by the hunt brain tools (record_finding,
// record_intel, link_artifacts, store_hunt, …). The harness session holds one;
// open_hunt sets its active hunt at runtime.
type RunContext struct {
	// HuntID is the hunt the brain tools write to. open_hunt sets it at runtime;
	// record_finding and store_hunt refuse to run until it is. HuntQuestion
	// carries the question open_hunt opened the hunt with, so store_hunt can title
	// the page.
	HuntID       string
	HuntQuestion string
	Brain        *brain.Client
	// Author identifies the operator this session runs as; the remember tool
	// stamps it onto shared memories so a team can see who learned what.
	Author string

	mu          sync.Mutex
	evidence    []brain.Evidence
	huntOutcome string // set by store_hunt
	huntPageURL string // set by store_hunt
}

// NewRunContext builds a RunContext over the given brain client. A hunt is set
// later by the open_hunt tool via SetHunt.
func NewRunContext(b *brain.Client) *RunContext {
	return &RunContext{Brain: b}
}

// SetHunt records the active hunt opened by the open_hunt tool so the hunt
// brain tools (record_finding, store_hunt) have a hunt to write to.
func (rc *RunContext) SetHunt(huntID, question string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.HuntID = huntID
	rc.HuntQuestion = question
}

func (rc *RunContext) addEvidence(e brain.Evidence) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.evidence = append(rc.evidence, e)
}

// Evidence returns the findings recorded so far.
func (rc *RunContext) Evidence() []brain.Evidence {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := make([]brain.Evidence, len(rc.evidence))
	copy(out, rc.evidence)
	return out
}

func (rc *RunContext) setHuntOutcome(outcome, pageURL string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.huntOutcome = outcome
	rc.huntPageURL = pageURL
}

// HuntOutcome returns the outcome status and page URL set by store_hunt, or
// empty strings if the hunt has not been stored.
func (rc *RunContext) HuntOutcome() (outcome, pageURL string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.huntOutcome, rc.huntPageURL
}
