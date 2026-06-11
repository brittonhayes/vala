package brain

import "context"

// Coverage is one ATT&CK technique's detection-coverage state: is it covered by
// a reliable detection, thinly covered, or not at all, and at what fidelity. It
// is the durable, cross-hunt map the Feedback stage upserts as hunts conclude
// and that scoping reads to weight the next hypothesis toward the weakest spots.
type Coverage struct {
	ID         string `json:"id"`
	Technique  string `json:"technique"` // ATT&CK ID, e.g. attack.t1562.001
	Tactic     string `json:"tactic"`
	Status     string `json:"status"`     // Covered | Thin | Uncovered
	Fidelity   string `json:"fidelity"`   // high | medium | low | none
	Detections string `json:"detections"` // summary of the detections that cover it
}

// UpsertCoverage records or updates the coverage state for a technique. There is
// no native upsert in the Notion surface, so it recalls by technique and updates
// the existing row when one matches, else creates a new one. It returns the row
// ID written.
func (c *Client) UpsertCoverage(ctx context.Context, cov Coverage) (string, error) {
	props := map[string]any{
		"technique":  cov.Technique,
		"updated_at": nowRFC3339(),
	}
	if cov.Tactic != "" {
		props["tactic"] = cov.Tactic
	}
	if cov.Status != "" {
		props["status"] = cov.Status
	}
	if cov.Fidelity != "" {
		props["fidelity"] = cov.Fidelity
	}
	if cov.Detections != "" {
		props["detections"] = cov.Detections
	}

	if existing := c.findCoverage(ctx, cov.Technique); existing != "" {
		if err := c.n.UpdateRow(ctx, existing, props); err != nil {
			return "", err
		}
		return existing, nil
	}
	return c.n.CreateRow(ctx, c.dbName(DBCoverage), props)
}

// findCoverage returns the ID of the coverage row whose technique matches, or
// the empty string if none does. It matches on the exact technique value rather
// than the free-text query so an upsert does not collide on a substring.
func (c *Client) findCoverage(ctx context.Context, technique string) string {
	if technique == "" {
		return ""
	}
	rows, err := c.n.Query(ctx, c.dbName(DBCoverage), technique, 25)
	if err != nil {
		return ""
	}
	for _, r := range rows {
		if v, ok := r.Props["technique"]; ok {
			if s, ok := v.(string); ok && s == technique {
				return r.ID
			}
		}
	}
	return ""
}
