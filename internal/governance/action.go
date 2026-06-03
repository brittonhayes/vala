package governance

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// ProposedAction is an action the model has proposed during PhasePropose,
// before it has been approved or executed. ID is a deterministic hash of the
// tool name and the canonicalized input, which is the backbone of replay /
// reorder / idempotency resistance: the same proposal always yields the same
// ID, the ledger refuses to execute an ID twice, and an approval for one ID
// cannot authorize a different action.
type ProposedAction struct {
	ID        string          `json:"id"`
	Tool      string          `json:"tool"`
	Input     json.RawMessage `json:"input"`
	Class     string          `json:"class"`
	Rationale string          `json:"rationale"`
	Evidence  []string        `json:"evidence"`
}

// ActionID returns a stable identifier for an action defined by a tool name and
// its JSON input. Inputs that are semantically equal (same keys/values, any key
// order) produce the same ID, so duplicate proposals collapse to one action.
func ActionID(toolName string, input json.RawMessage) string {
	sum := sha256.Sum256([]byte(toolName + "\x00" + canonicalJSON(input)))
	return fmt.Sprintf("act_%x", sum[:8])
}

// canonicalJSON re-serializes JSON so that logically-equal inputs hash
// identically. encoding/json marshals map keys in sorted order at every level,
// so a round-trip through `any` is enough to canonicalize key ordering and
// whitespace. Invalid JSON is returned verbatim.
func canonicalJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}
