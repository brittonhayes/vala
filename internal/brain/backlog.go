package brain

import "context"

// BacklogItem is a queued, prioritized hunt hypothesis — the durable record of a
// trigger (threat intel, a hunch, a fresh CVE, a past incident) before it becomes
// an active hunt. It is the TaHiTI "hunting backlog": triggers are never lost,
// and the program of work is visible and rankable rather than ad hoc.
//
// The hypothesis is scoped with ABLE: the testable adversary Behavior and the
// Location (DataSource) it would show up in. A backlog item opens into a Hunt and
// is then retired.
type BacklogItem struct {
	ID         string `json:"id"`
	Trigger    string `json:"trigger"`
	Hypothesis string `json:"hypothesis"`
	Behavior   string `json:"behavior"`
	DataSource string `json:"data_source"`
	Priority   string `json:"priority"`
	MITRE      string `json:"mitre"`
	Status     string `json:"status"`
}

// QueueHunt writes a Backlog row in the Queued state and returns its ID.
func (c *Client) QueueHunt(ctx context.Context, b BacklogItem) (backlogID string, err error) {
	props := map[string]any{
		"backlog_id": b.Trigger,
		"trigger":    b.Trigger,
		"hypothesis": b.Hypothesis,
		"status":     BacklogQueued,
		"created_at": nowRFC3339(),
	}
	if b.Behavior != "" {
		props["behavior"] = b.Behavior
	}
	if b.DataSource != "" {
		props["data_source"] = b.DataSource
	}
	if b.Priority != "" {
		props["priority"] = b.Priority
	}
	if b.MITRE != "" {
		props["mitre"] = b.MITRE
	}
	return c.n.CreateRow(ctx, c.dbName(DBBacklog), props)
}

// SetBacklogStatus transitions a Backlog row (Queued → Opened → Done) and links
// it to the hunt it became, when one is given.
func (c *Client) SetBacklogStatus(ctx context.Context, backlogID, status, huntID string) error {
	props := map[string]any{"status": status}
	if huntID != "" {
		props["hunt"] = []string{huntID}
	}
	return c.n.UpdateRow(ctx, backlogID, props)
}
