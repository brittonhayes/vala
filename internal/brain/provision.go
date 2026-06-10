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

// Schema returns the canonical schema `vala init` provisions: one DBSpec per
// brain store, with property names and types matching exactly what the writers
// emit (see brain.go, hunt.go, intel.go, backlog.go). It is the single source of
// truth shared by provisioning and the writers — a property a writer emits must
// appear here or NTN.CreateRow will silently drop it.
func Schema() []DBSpec {
	return []DBSpec{
		{
			Name:  DBEvidence,
			Title: "Vala Evidence",
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
			Title: "Vala Hunts",
			Props: []PropSpec{
				{"hunt_id", "title"},
				{"question", "rich_text"},
				{"hypothesis", "rich_text"},
				{"status", "status"},
				{"mitre", "rich_text"},
				{"behavior", "rich_text"},
				{"data_source", "rich_text"},
				{"findings", "rich_text"},
				{"started_at", "date"},
				{"ended_at", "date"},
			},
			StatusOptions: map[string][]string{"status": {HuntOpen, HuntConfirmed, HuntRefuted, HuntInconclusive}},
		},
		{
			Name:  DBIntel,
			Title: "Vala Intel",
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
			Title: "Vala Detections",
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
			Title: "Vala Backlog",
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
			Title: "Vala Memory",
			Props: []PropSpec{
				{"memory_id", "title"},
				{"fact", "rich_text"},
				{"author", "rich_text"},
				{"created_at", "date"},
			},
			Relations: []RelationSpec{{"hunt", DBHunts}},
		},
	}
}

// DBIDsFromMap assembles a DBIDs from a logical-store-name -> data-source-ID map
// (keyed by the brain.DB* constants) plus the narrative-page parent page ID. It
// is the bridge from provisioning output to the config the brain reads.
func DBIDsFromMap(ds map[string]string, parent string) DBIDs {
	return DBIDs{
		Evidence:   ds[DBEvidence],
		Hunts:      ds[DBHunts],
		Intel:      ds[DBIntel],
		Detections: ds[DBDetections],
		Backlog:    ds[DBBacklog],
		Memory:     ds[DBMemory],
		Parent:     parent,
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

// CreateChildPage creates an empty narrative-parent page titled title under
// parentPageID and returns its ID. The brain writes hunt pages beneath it.
func (n *NTN) CreateChildPage(ctx context.Context, parentPageID, title string) (string, error) {
	out, err := n.runOut(ctx, "pages", "create", "--parent", "page:"+parentPageID, "--content", "# "+title, "--json")
	if err != nil {
		return "", err
	}
	id := extractID(string(out))
	if id == "" {
		return "", fmt.Errorf("create page %q: no id in output", title)
	}
	return id, nil
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
