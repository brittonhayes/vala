package brain

import (
	"context"
	"encoding/json"
	"fmt"
)

// PropSpec is one Notion property to create on a data source: its name and the
// Notion property type. The type strings are exactly those typedValue knows how
// to write, so a provisioned schema and the brain's writers stay in lockstep.
type PropSpec struct {
	Name string
	Type string
}

// RelationSpec is a relation property wired in a second provisioning pass, once
// every target data source exists. Target is the logical store name (a brain.DB*
// constant) the relation points at.
type RelationSpec struct {
	Name   string
	Target string
}

// DBSpec is the schema for one brain store: the Notion database title, its
// logical name (a brain.DB* constant), the scalar properties created up front,
// and the relation properties wired afterward. Exactly one scalar must be of
// type "title"; it is the row's display column the writers populate.
//
// StatusOptions maps a "status" property name to the allowed values its writer
// emits. Notion does not auto-create status options on write (unlike select), so
// a status column must be born with exactly those values, or writes 400 with
// "status option does not exist".
type DBSpec struct {
	Name          string
	Title         string
	Props         []PropSpec
	Relations     []RelationSpec
	StatusOptions map[string][]string
}

// brainDBTitle is the title of the single Notion database that holds every brain
// store as a data source. One database keeps the workspace uncluttered (one
// top-level object instead of seven) and lets the whole brain be provisioned,
// verified, and repaired as a unit.
const brainDBTitle = "Vala Brain"

// Schema returns the canonical schema setup provisions: one DBSpec per brain
// store, with property names and types matching exactly what the writers emit
// (see brain.go, hunt.go, intel.go, backlog.go). It is the single source of
// truth shared by provisioning and the writers — a property a writer emits must
// appear here or NTN.CreateRow will silently drop it. Each spec becomes a data
// source inside the single brainDBTitle database, so the titles are the bare
// store names (the database title already namespaces them as "Vala").
func Schema() []DBSpec {
	return []DBSpec{
		{
			Name:  DBEvidence,
			Title: "Evidence",
			Props: []PropSpec{
				{"claim", "title"},
				{"kind", "select"},
				{"pointer", "rich_text"},
				{"confidence", "select"},
				{"collected_at", "date"},
			},
			Relations: []RelationSpec{{"hunt", DBHunts}},
		},
		{
			Name:  DBHunts,
			Title: "Hunts",
			Props: []PropSpec{
				{"hunt_id", "title"},
				{"question", "rich_text"},
				{"hypothesis", "rich_text"},
				{"status", "status"},
				{"mitre", "rich_text"},
				{"behavior", "rich_text"},
				{"data_source", "rich_text"},
				{"hunt_type", "select"},
				{"detection_tier", "select"},
				{"tier_rationale", "rich_text"},
				{"findings", "rich_text"},
				{"started_at", "date"},
				{"ended_at", "date"},
			},
			StatusOptions: map[string][]string{"status": {HuntOpen, HuntConfirmed, HuntRefuted, HuntInconclusive}},
		},
		{
			Name:  DBIntel,
			Title: "Intel",
			Props: []PropSpec{
				{"intel_id", "title"},
				{"kind", "select"},
				{"value", "rich_text"},
				{"mitre", "rich_text"},
				{"confidence", "select"},
				{"source", "rich_text"},
				{"description", "rich_text"},
				{"created_at", "date"},
			},
			Relations: []RelationSpec{{"hunts", DBHunts}, {"detections", DBDetections}},
		},
		{
			Name:  DBDetections,
			Title: "Detections",
			Props: []PropSpec{
				{"detection_id", "title"},
				{"title", "rich_text"},
				{"path", "rich_text"},
				{"status", "select"},
				{"mitre", "rich_text"},
				{"level", "select"},
			},
			Relations: []RelationSpec{{"intel", DBIntel}, {"hunts", DBHunts}},
		},
		{
			Name:  DBBacklog,
			Title: "Backlog",
			Props: []PropSpec{
				{"backlog_id", "title"},
				{"trigger", "rich_text"},
				{"hypothesis", "rich_text"},
				{"status", "status"},
				{"behavior", "rich_text"},
				{"data_source", "rich_text"},
				{"priority", "select"},
				{"mitre", "rich_text"},
				{"created_at", "date"},
			},
			Relations:     []RelationSpec{{"hunt", DBHunts}},
			StatusOptions: map[string][]string{"status": {BacklogQueued, BacklogOpened, BacklogDone}},
		},
		{
			Name:  DBMemory,
			Title: "Memory",
			Props: []PropSpec{
				{"memory_id", "title"},
				{"fact", "rich_text"},
				{"author", "rich_text"},
				{"created_at", "date"},
			},
			Relations: []RelationSpec{{"hunt", DBHunts}},
		},
		{
			Name:  DBCoverage,
			Title: "Coverage",
			Props: []PropSpec{
				{"technique", "title"},
				{"tactic", "rich_text"},
				{"status", "status"},
				{"fidelity", "select"},
				{"detections", "rich_text"},
				{"updated_at", "date"},
			},
			Relations:     []RelationSpec{{"hunts", DBHunts}},
			StatusOptions: map[string][]string{"status": {CoverageCovered, CoverageThin, CoverageUncovered}},
		},
	}
}

// DBIDsFromMap assembles a DBIDs from a logical-store-name -> data-source-ID map
// (keyed by the brain.DB* constants), the parent database ID, and the
// narrative-page parent page ID. It is the bridge from provisioning output to
// the config the brain reads.
func DBIDsFromMap(ds map[string]string, database, parent string) DBIDs {
	return DBIDs{
		Database:   database,
		Evidence:   ds[DBEvidence],
		Hunts:      ds[DBHunts],
		Intel:      ds[DBIntel],
		Detections: ds[DBDetections],
		Backlog:    ds[DBBacklog],
		Memory:     ds[DBMemory],
		Coverage:   ds[DBCoverage],
		Parent:     parent,
	}
}

// byName inverts DBIDs to a logical-store-name -> data-source-ID map (keyed by
// the brain.DB* constants), so provisioning and repair can work in the same
// shape DBIDsFromMap consumes.
func (ids DBIDs) byName() map[string]string {
	return map[string]string{
		DBEvidence:   ids.Evidence,
		DBHunts:      ids.Hunts,
		DBIntel:      ids.Intel,
		DBDetections: ids.Detections,
		DBBacklog:    ids.Backlog,
		DBMemory:     ids.Memory,
		DBCoverage:   ids.Coverage,
	}
}

// Whoami verifies the ntn CLI is available and authenticated. A non-nil error
// means the operator must run `ntn login` before provisioning.
func (n *NTN) Whoami(ctx context.Context) error {
	_, err := n.runOut(ctx, "whoami")
	return err
}

// CreateDatabase creates a Notion database under parentPageID with an initial
// data source whose schema is props. statusOptions seeds the allowed values for
// any "status" property (Notion will not auto-create them on write). It returns
// the database ID and the initial data source's ID — the latter is what the
// brain queries and writes against.
func (n *NTN) CreateDatabase(ctx context.Context, parentPageID, title string, props []PropSpec, statusOptions map[string][]string) (dbID, dsID string, err error) {
	schema := make(map[string]any, len(props))
	for _, p := range props {
		schema[p.Name] = propConfig(p.Type, statusOptions[p.Name])
	}
	body := map[string]any{
		"parent":              map[string]any{"type": "page_id", "page_id": parentPageID},
		"title":               richText(title),
		"initial_data_source": map[string]any{"properties": schema},
	}
	out, err := n.api(ctx, "POST", "/v1/databases", body)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		ID          string `json:"id"`
		DataSources []struct {
			ID string `json:"id"`
		} `json:"data_sources"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", fmt.Errorf("parse created database: %w", err)
	}
	if resp.ID == "" {
		return "", "", fmt.Errorf("create database %q: no id in response", title)
	}
	if len(resp.DataSources) > 0 {
		dsID = resp.DataSources[0].ID
	}
	if dsID == "" {
		// Fall back to resolving the database to its data source.
		if dsID, err = n.ResolveDataSource(ctx, resp.ID); err != nil {
			return resp.ID, "", err
		}
	}
	return resp.ID, dsID, nil
}

// AddRelations adds relation properties to an existing data source. rels maps a
// relation property name to the target data-source ID it links to. It is the
// second provisioning pass, run once every target exists.
func (n *NTN) AddRelations(ctx context.Context, dsID string, rels map[string]string) error {
	props := make(map[string]any, len(rels))
	for name, target := range rels {
		props[name] = map[string]any{
			"type": "relation",
			"relation": map[string]any{
				"data_source_id":  target,
				"single_property": map[string]any{},
			},
		}
	}
	_, err := n.api(ctx, "PATCH", "/v1/data_sources/"+dsID, map[string]any{"properties": props})
	return err
}

// CreateDataSource adds a data source to an existing database (parent
// database_id), with schema props and statusOptions seeding any "status"
// property's allowed values. It returns the new data source's ID. This is how
// every store after the first is created under the single brain database.
func (n *NTN) CreateDataSource(ctx context.Context, dbID, title string, props []PropSpec, statusOptions map[string][]string) (string, error) {
	schema := make(map[string]any, len(props))
	for _, p := range props {
		schema[p.Name] = propConfig(p.Type, statusOptions[p.Name])
	}
	body := map[string]any{
		"parent":     map[string]any{"type": "database_id", "database_id": dbID},
		"title":      richText(title),
		"properties": schema,
	}
	out, err := n.api(ctx, "POST", "/v1/data_sources", body)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse created data source: %w", err)
	}
	if resp.ID == "" {
		return "", fmt.Errorf("create data source %q: no id in response", title)
	}
	return resp.ID, nil
}

// renameDataSource sets a data source's title. It is cosmetic — the database
// gives its initial data source the database title, so we relabel it to the
// store name — and so failures are not fatal to provisioning.
func (n *NTN) renameDataSource(ctx context.Context, dsID, title string) error {
	_, err := n.api(ctx, "PATCH", "/v1/data_sources/"+dsID, map[string]any{"title": richText(title)})
	return err
}

// DatabaseExists reports whether a database ID resolves — used to decide whether
// a configured brain can be repaired in place (add the missing data sources) or
// must be re-provisioned from scratch.
func (n *NTN) DatabaseExists(ctx context.Context, dbID string) bool {
	if dbID == "" {
		return false
	}
	_, err := n.api(ctx, "GET", "/v1/databases/"+dbID, nil)
	return err == nil
}

// Provision creates the single brain database with one data source per store,
// wires the relation properties, creates the narrative hunt-page parent, and
// returns the resulting DBIDs. parentPageID is the Notion page the database and
// hunt-page parent are created beneath. It is the full first-time setup, callable
// from the onboarding wizard.
func (n *NTN) Provision(ctx context.Context, parentPageID string) (DBIDs, error) {
	if err := n.Whoami(ctx); err != nil {
		return DBIDs{}, fmt.Errorf("the Notion CLI is not authenticated: %w", err)
	}
	specs := Schema()
	if len(specs) == 0 {
		return DBIDs{}, fmt.Errorf("empty brain schema")
	}

	// The database is born with its first store as the initial data source; the
	// rest are added under it. Collect logical name -> data-source ID as we go.
	dsByName := make(map[string]string, len(specs))
	first := specs[0]
	dbID, dsID, err := n.CreateDatabase(ctx, parentPageID, brainDBTitle, first.Props, first.StatusOptions)
	if err != nil {
		return DBIDs{}, fmt.Errorf("create %s database: %w", brainDBTitle, err)
	}
	_ = n.renameDataSource(ctx, dsID, first.Title) // cosmetic; ignore failure
	dsByName[first.Name] = dsID

	for _, s := range specs[1:] {
		id, err := n.CreateDataSource(ctx, dbID, s.Title, s.Props, s.StatusOptions)
		if err != nil {
			return DBIDs{}, fmt.Errorf("create %s data source: %w", s.Name, err)
		}
		dsByName[s.Name] = id
	}

	if err := n.wireRelations(ctx, dsByName, specs); err != nil {
		return DBIDs{}, err
	}

	// Narrative hunt pages are written directly under the brain's home page
	// (alongside the database) — no separate wrapper page to clutter the workspace.
	return DBIDsFromMap(dsByName, dbID, parentPageID), nil
}

// Verify probes a configured brain: databaseOK is whether the parent database
// still resolves, and missing lists the stores (brain.DB* names) whose data
// source is unset or unreachable. It is the read side of repair — the wizard
// uses it to decide between adding the missing data sources (databaseOK) and
// re-provisioning from scratch.
func (n *NTN) Verify(ctx context.Context, ids DBIDs) (missing []string, databaseOK bool) {
	databaseOK = n.DatabaseExists(ctx, ids.Database)
	byName := ids.byName()
	for _, s := range Schema() {
		id := byName[s.Name]
		if id == "" || !n.DataSourceExists(ctx, id) {
			missing = append(missing, s.Name)
		}
	}
	return missing, databaseOK
}

// AddMissing repairs a configured brain in place: it recreates each missing
// store's data source under the existing parent database and re-wires the
// relations that touch a recreated store. It returns the patched DBIDs. The
// caller must have confirmed the parent database resolves (Verify's databaseOK);
// when it does not, the brain is re-provisioned from scratch instead.
func (n *NTN) AddMissing(ctx context.Context, ids DBIDs, missing []string) (DBIDs, error) {
	if ids.Database == "" {
		return ids, fmt.Errorf("no parent database to repair into")
	}
	specsByName := make(map[string]DBSpec, len(Schema()))
	for _, s := range Schema() {
		specsByName[s.Name] = s
	}

	dsByName := ids.byName()
	recreated := make(map[string]bool, len(missing))
	for _, name := range missing {
		s, ok := specsByName[name]
		if !ok {
			continue
		}
		id, err := n.CreateDataSource(ctx, ids.Database, s.Title, s.Props, s.StatusOptions)
		if err != nil {
			return ids, fmt.Errorf("recreate %s data source: %w", name, err)
		}
		dsByName[name] = id
		recreated[name] = true
	}

	// Re-wire relations for every recreated store and for any surviving store
	// whose relation pointed at one (its old target was deleted with the store).
	var toWire []DBSpec
	for _, s := range Schema() {
		if relationsTouch(s, recreated) {
			toWire = append(toWire, s)
		}
	}
	if err := n.wireRelations(ctx, dsByName, toWire); err != nil {
		return ids, err
	}
	return DBIDsFromMap(dsByName, ids.Database, ids.Parent), nil
}

// wireRelations adds each spec's relation properties, resolving every relation
// target to its data-source ID in dsByName. It is the shared second pass used by
// both Provision (all specs) and AddMissing (the affected subset).
func (n *NTN) wireRelations(ctx context.Context, dsByName map[string]string, specs []DBSpec) error {
	for _, s := range specs {
		if len(s.Relations) == 0 {
			continue
		}
		rels := make(map[string]string, len(s.Relations))
		for _, r := range s.Relations {
			target, ok := dsByName[r.Target]
			if !ok || target == "" {
				return fmt.Errorf("%s relation %q targets unknown store %q", s.Name, r.Name, r.Target)
			}
			rels[r.Name] = target
		}
		if err := n.AddRelations(ctx, dsByName[s.Name], rels); err != nil {
			return fmt.Errorf("add relations to %s: %w", s.Name, err)
		}
	}
	return nil
}

// relationsTouch reports whether spec s is the recreated store or has a relation
// pointing at a recreated store — i.e. whether its relations must be re-wired.
func relationsTouch(s DBSpec, recreated map[string]bool) bool {
	if recreated[s.Name] {
		return true
	}
	for _, r := range s.Relations {
		if recreated[r.Target] {
			return true
		}
	}
	return false
}

// ResolveDataSource resolves a Notion database ID to its (first) data-source ID
// via `ntn datasources resolve`. It tolerates the array, results-wrapped, or
// bare-object JSON shapes the CLI may emit.
func (n *NTN) ResolveDataSource(ctx context.Context, dbID string) (string, error) {
	out, err := n.runOut(ctx, "datasources", "resolve", dbID, "--json")
	if err != nil {
		return "", err
	}
	if id := firstDataSourceID(out); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("resolve database %s: no data source id in output", dbID)
}

// DataSourceExists reports whether a data source ID resolves — used to verify an
// existing configuration rather than blindly recreating it.
func (n *NTN) DataSourceExists(ctx context.Context, dsID string) bool {
	_, err := n.api(ctx, "GET", "/v1/data_sources/"+dsID, nil)
	return err == nil
}

// propConfig returns the Notion property-configuration object for a type when
// creating a data source. statusOptions seeds a status property's allowed values
// (ignored for other types). Unknown types fall back to rich_text so the column
// is still created rather than rejected.
func propConfig(typ string, statusOptions []string) map[string]any {
	switch typ {
	case "title":
		return map[string]any{"type": "title", "title": map[string]any{}}
	case "rich_text":
		return map[string]any{"type": "rich_text", "rich_text": map[string]any{}}
	case "select":
		return map[string]any{"type": "select", "select": map[string]any{}}
	case "status":
		status := map[string]any{}
		if len(statusOptions) > 0 {
			opts := make([]any, 0, len(statusOptions))
			for _, o := range statusOptions {
				opts = append(opts, map[string]any{"name": o})
			}
			status["options"] = opts
		}
		return map[string]any{"type": "status", "status": status}
	case "date":
		return map[string]any{"type": "date", "date": map[string]any{}}
	case "number":
		return map[string]any{"type": "number", "number": map[string]any{}}
	default:
		return map[string]any{"type": "rich_text", "rich_text": map[string]any{}}
	}
}

// firstDataSourceID extracts the first data-source ID from `ntn datasources
// resolve --json` output, tolerating an object with a "data_sources" array, a
// "results" array, a bare array, or a single object.
func firstDataSourceID(out []byte) string {
	var obj struct {
		DataSources []map[string]any `json:"data_sources"`
		Results     []map[string]any `json:"results"`
		ID          string           `json:"id"`
	}
	if err := json.Unmarshal(out, &obj); err == nil {
		if id := idFromMaps(obj.DataSources); id != "" {
			return id
		}
		if id := idFromMaps(obj.Results); id != "" {
			return id
		}
		if obj.ID != "" {
			return obj.ID
		}
	}
	var arr []map[string]any
	if err := json.Unmarshal(out, &arr); err == nil {
		return idFromMaps(arr)
	}
	return ""
}

func idFromMaps(ms []map[string]any) string {
	for _, m := range ms {
		if id, ok := m["id"].(string); ok && id != "" {
			return id
		}
	}
	return ""
}
