package brain

// Database names. The Mem store keys rows by these; the NTN store maps them to
// configured Notion database IDs.
const (
	DBEvidence   = "evidence"
	DBHunts      = "hunts"
	DBIntel      = "intel"
	DBDetections = "detections"
	DBBacklog    = "backlog"
)

// Hunt status values (the Hunts state machine). A hunt opens Open, and closes
// in one of the three terminal states once the hypothesis has been evaluated.
const (
	HuntOpen         = "Open"
	HuntConfirmed    = "Confirmed"
	HuntRefuted      = "Refuted"
	HuntInconclusive = "Inconclusive"
)

// Backlog status values. A trigger is queued as a hypothesis, gets opened into
// an active hunt, and is retired once that hunt reaches a verdict.
const (
	BacklogQueued = "Queued"
	BacklogOpened = "Opened"
	BacklogDone   = "Done"
)

// Client provides typed, schema-shaped writers over a Notion store. It is the
// only thing that knows the hunt-brain data model; the model talks to it via
// the hunt, intel, and detection tools.
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
			case DBEvidence:
				return ntn.DBs.Evidence
			case DBHunts:
				return ntn.DBs.Hunts
			case DBIntel:
				return ntn.DBs.Intel
			case DBDetections:
				return ntn.DBs.Detections
			case DBBacklog:
				return ntn.DBs.Backlog
			}
			return logical
		}
	}
	return c
}

// Store returns the underlying Notion store (test introspection).
func (c *Client) Store() Notion { return c.n }
