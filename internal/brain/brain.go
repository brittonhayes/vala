package brain

// Database names. The Mem store keys rows by these; the NTN store maps them to
// configured Notion database IDs.
const (
	DBEvidence   = "evidence"
	DBHunts      = "hunts"
	DBIntel      = "intel"
	DBDetections = "detections"
	DBBacklog    = "backlog"
	DBMemory     = "memory"
	DBCoverage   = "coverage"
)

// Hunt status values (the Hunts state machine). A hunt opens Open, and closes
// in one of the three terminal states once the hypothesis has been evaluated.
const (
	HuntOpen         = "Open"
	HuntConfirmed    = "Confirmed"
	HuntRefuted      = "Refuted"
	HuntInconclusive = "Inconclusive"
)

// Hunt types (PEAK). Every hunt declares which style it runs: hypothesis-driven
// (a specific predicted TTP), baseline / exploratory data analysis (characterize
// normal, then surface deviations), or model-assisted (M-ATH — reason over
// algorithmic leads like clustering or outliers in the data the agent pulls).
const (
	HuntHypothesis    = "hypothesis"
	HuntBaseline      = "baseline"
	HuntModelAssisted = "model_assisted"
)

// Detection-output tiers (the hierarchy of detection outputs). Every hunt closes
// with one tier decision and a rationale: pick the highest-fidelity output the
// finding supports. Tiers 1–2 produce a Sigma rule; tier 3 a recurring hunt;
// tier 4 a playbook; tier 5 a justified decision to build nothing.
const (
	TierAutomated   = "tier1_automated"      // production-grade, high-fidelity Sigma rule
	TierTriage      = "tier2_triage"         // lower-fidelity Sigma that surfaces candidates for review
	TierRecurring   = "tier3_recurring_hunt" // re-run the hunt on a cadence; no rule yet feasible
	TierPlaybook    = "tier4_playbook"       // documented investigation method for future hunts
	TierNoDetection = "tier5_none_documented" // justified no-build (benign, out of scope, or a visibility gap)
)

// Evidence kinds (the Source field on an Evidence row). The first four are the
// classic pointer kinds; data_plan and visibility_gap let the data-validation
// stage record its result as a first-class, hunt-linked finding rather than a
// silent skip.
const (
	EvidenceQuery    = "query"
	EvidenceURL      = "url"
	EvidenceFileHash = "file_hash"
	EvidenceLogRef   = "log_ref"
	EvidenceDataPlan = "data_plan"      // a validated telemetry plan: sources, window, retention
	EvidenceGap      = "visibility_gap" // a failed telemetry check: telemetry missing or incomplete
)

// Coverage status values (the Coverage state machine for a technique): is this
// ATT&CK technique covered by a reliable detection, thinly covered, or not at
// all? The Feedback stage upserts coverage as hunts conclude.
const (
	CoverageCovered    = "Covered"
	CoverageThin       = "Thin"
	CoverageUncovered  = "Uncovered"
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
			case DBMemory:
				return ntn.DBs.Memory
			case DBCoverage:
				return ntn.DBs.Coverage
			}
			return logical
		}
	}
	return c
}

// Store returns the underlying Notion store (test introspection).
func (c *Client) Store() Notion { return c.n }
