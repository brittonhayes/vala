package brain

import "context"

// Intel kinds.
const (
	IntelIndicator = "indicator" // an IOC: hash, IP, domain, URL
	IntelTTP       = "ttp"       // a technique, e.g. a MITRE ATT&CK ID
	IntelActor     = "actor"     // a threat actor / group
	IntelNarrative = "narrative" // free-form intel writeup
)

// Intel is a piece of threat intelligence stored as a first-class brain
// artifact: an indicator, a TTP, an actor, or a narrative. Intel connects to the
// hunts that surfaced it and the detections it informs.
type Intel struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Value       string   `json:"value"`
	MITRE       string   `json:"mitre"`
	Confidence  string   `json:"confidence"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	Hunts       []string `json:"hunts"`
	Detections  []string `json:"detections"`
}

// DetectionRef is the brain's graph node for a Sigma detection. The rule YAML
// lives on disk; this row makes "hunt/intel -> detection" relations first-class.
type DetectionRef struct {
	ID     string   `json:"id"` // the Sigma rule id (title)
	Title  string   `json:"title"`
	Path   string   `json:"path"`
	Status string   `json:"status"`
	MITRE  string   `json:"mitre"`
	Level  string   `json:"level"`
	Intel  []string `json:"intel"`
	Hunts  []string `json:"hunts"`
}

// RecordIntel writes an Intel row and returns its ID. Initial relations (hunts,
// detections) are set inline; Link adds more after the fact.
func (c *Client) RecordIntel(ctx context.Context, i Intel) (intelID string, err error) {
	props := map[string]any{
		"intel_id":    i.Value,
		"kind":        i.Kind,
		"value":       i.Value,
		"mitre":       i.MITRE,
		"confidence":  i.Confidence,
		"source":      i.Source,
		"description": i.Description,
		"created_at":  nowRFC3339(),
	}
	setRelation(props, "hunts", i.Hunts)
	setRelation(props, "detections", i.Detections)
	return c.n.CreateRow(ctx, c.dbName(DBIntel), props)
}

// RecordDetection writes a Detections row (the graph node for a Sigma rule) and
// returns its ID.
func (c *Client) RecordDetection(ctx context.Context, d DetectionRef) (detID string, err error) {
	props := map[string]any{
		"detection_id": d.ID,
		"title":        d.Title,
		"path":         d.Path,
		"status":       d.Status,
		"mitre":        d.MITRE,
		"level":        d.Level,
	}
	setRelation(props, "intel", d.Intel)
	setRelation(props, "hunts", d.Hunts)
	return c.n.CreateRow(ctx, c.dbName(DBDetections), props)
}

// Link is the single linking primitive: it sets a relation property on an
// existing row to the given target row IDs, connecting brain artifacts (intel,
// hunts, detections) into one graph.
func (c *Client) Link(ctx context.Context, rowID, relation string, targetIDs ...string) error {
	if len(targetIDs) == 0 {
		return nil
	}
	return c.n.UpdateRow(ctx, rowID, map[string]any{relation: targetIDs})
}

// setRelation adds a relation property only when there are targets, keeping
// empty relations out of the written props.
func setRelation(props map[string]any, name string, ids []string) {
	if len(ids) > 0 {
		props[name] = ids
	}
}
