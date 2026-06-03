package brain

import "context"

// Database names. The Mem store keys rows by these; the NTN store maps them to
// configured Notion database IDs.
const (
	DBAlerts     = "alerts"
	DBCases      = "cases"
	DBEvidence   = "evidence"
	DBActions    = "actions"
	DBRuns       = "runs"
	DBHunts      = "hunts"
	DBIntel      = "intel"
	DBDetections = "detections"
)

// Case status values (the Cases state machine).
const (
	StatusTriage        = "Triage"
	StatusInvestigating = "Investigating"
	StatusContained     = "Contained"
	StatusResolved      = "Resolved"
)

// Hunt status values (the Hunts state machine). A hunt opens Open, and closes
// in one of the three terminal states once the hypothesis has been evaluated.
const (
	HuntOpen         = "Open"
	HuntConfirmed    = "Confirmed"
	HuntRefuted      = "Refuted"
	HuntInconclusive = "Inconclusive"
)

// Alert is a normalized alert ingressing into the system.
type Alert struct {
	AlertID  string `json:"alert_id" yaml:"alert_id"`
	Source   string `json:"source" yaml:"source"`
	Severity string `json:"severity" yaml:"severity"`
	Raw      string `json:"raw" yaml:"raw"`
}

// Client provides typed, schema-shaped writers over a Notion store. It is the
// only thing that knows the case-brain data model; the model talks to it via
// the open_case / record_evidence / propose_action / write_case_page tools.
type Client struct {
	n      Notion
	dbName func(string) string
}

// New returns a Client backed by the given Notion store. For NTN the logical
// database names are mapped to configured IDs; for Mem they are used directly.
func New(n Notion) *Client {
	c := &Client{n: n, dbName: func(s string) string { return s }}
	if ntn, ok := n.(*NTN); ok {
		c.dbName = func(logical string) string {
			switch logical {
			case DBAlerts:
				return ntn.DBs.Alerts
			case DBCases:
				return ntn.DBs.Cases
			case DBEvidence:
				return ntn.DBs.Evidence
			case DBActions:
				return ntn.DBs.Actions
			case DBRuns:
				return ntn.DBs.Runs
			case DBHunts:
				return ntn.DBs.Hunts
			case DBIntel:
				return ntn.DBs.Intel
			case DBDetections:
				return ntn.DBs.Detections
			}
			return logical
		}
	}
	return c
}

// Store returns the underlying Notion store (test/harness introspection).
func (c *Client) Store() Notion { return c.n }

// OpenCase creates an Alerts row and a linked Case row in Triage, returning the
// case ID.
func (c *Client) OpenCase(ctx context.Context, a Alert, title string) (caseID string, err error) {
	alertID, err := c.n.CreateRow(ctx, c.dbName(DBAlerts), map[string]any{
		"alert_id":    a.AlertID,
		"source":      a.Source,
		"severity":    a.Severity,
		"raw":         a.Raw,
		"received_at": nowRFC3339(),
		"status":      "linked",
	})
	if err != nil {
		return "", err
	}
	caseID, err = c.n.CreateRow(ctx, c.dbName(DBCases), map[string]any{
		"case_id":   title,
		"status":    StatusInvestigating,
		"severity":  a.Severity,
		"opened_at": nowRFC3339(),
		"alerts":    alertID,
	})
	return caseID, err
}

// StartRun records a Runs row for the agent session.
func (c *Client) StartRun(ctx context.Context, caseID, model, commit string) (string, error) {
	return c.n.CreateRow(ctx, c.dbName(DBRuns), map[string]any{
		"case":       caseID,
		"model":      model,
		"commit":     commit,
		"started_at": nowRFC3339(),
	})
}

// EndRun finalizes a Runs row with the phase reached and counters.
func (c *Client) EndRun(ctx context.Context, runID, phase string, toolCalls, violations int) error {
	return c.n.UpdateRow(ctx, runID, map[string]any{
		"ended_at":      nowRFC3339(),
		"phase_reached": phase,
		"tool_calls":    toolCalls,
		"violations":    violations,
	})
}

// RecordEvidence appends an immutable Evidence row and returns its ID.
func (c *Client) RecordEvidence(ctx context.Context, caseID string, e Evidence) (string, error) {
	return c.n.CreateRow(ctx, c.dbName(DBEvidence), map[string]any{
		"case":         caseID,
		"claim":        e.Claim,
		"kind":         e.Source,
		"pointer":      e.Pointer,
		"confidence":   e.Confidence,
		"collected_at": nowRFC3339(),
	})
}

// RecordAction writes a proposed Action row and returns its ID.
func (c *Client) RecordAction(ctx context.Context, caseID string, a Action) (string, error) {
	return c.n.CreateRow(ctx, c.dbName(DBActions), map[string]any{
		"case":      caseID,
		"action_id": a.ID,
		"type":      a.Class,
		"params":    a.Params,
		"rationale": a.Rationale,
		"status":    a.Status,
	})
}

// UpdateActionStatus transitions an Action row's status.
func (c *Client) UpdateActionStatus(ctx context.Context, rowID, status, by, result string) error {
	props := map[string]any{"status": status}
	if by != "" {
		props["approved_by"] = by
		props["approved_at"] = nowRFC3339()
	}
	if result != "" {
		props["result"] = result
		props["executed_at"] = nowRFC3339()
	}
	return c.n.UpdateRow(ctx, rowID, props)
}

// WriteCasePage renders and creates the narrative case page, returning its URL.
func (c *Client) WriteCasePage(ctx context.Context, title string, p CasePage) (string, error) {
	_, url, err := c.n.CreatePage(ctx, title, p.Render())
	return url, err
}
