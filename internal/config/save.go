package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/brittonhayes/vala/internal/brain"
)

// SaveNotion merges the provisioned Notion data-source IDs into the project's
// .vala.json, setting only the "notion" key and preserving every other key
// (model, mcp, detections_dir, …) byte-for-byte.
func SaveNotion(cwd string, ids brain.DBIDs) error {
	return saveKey(cwd, "notion", ids)
}

// SaveProvider records the chosen provider id and model in the project's
// .vala.json so the next launch uses them, preserving every other key. An empty
// model leaves the existing "model" key untouched (the provider's default
// applies). Secrets are never written here — only the provider id and model.
func SaveProvider(cwd, provider, model string) error {
	if err := saveKey(cwd, "provider", provider); err != nil {
		return err
	}
	if model != "" {
		return saveKey(cwd, "model", model)
	}
	return nil
}

// SaveBrainFile records the local brain-file path in .vala.json (the
// "brain_file" key) so a file-backed brain persists across runs without a Notion
// account, preserving every other key.
func SaveBrainFile(cwd, brainFile string) error {
	return saveKey(cwd, "brain_file", brainFile)
}

// SaveMCP upserts an MCP server into the project's .vala.json "mcp" array by
// name — replacing an existing entry with the same Name or appending a new one —
// while preserving every other key. Secrets are never written: the MCPServer's
// resolved APIKey/Env fields are tagged json:"-", so only the env-var names are
// persisted.
func SaveMCP(cwd string, server MCPServer) error {
	servers, err := loadMCP(cwd)
	if err != nil {
		return err
	}
	replaced := false
	for i := range servers {
		if servers[i].Name == server.Name {
			servers[i] = server
			replaced = true
			break
		}
	}
	if !replaced {
		servers = append(servers, server)
	}
	return saveKey(cwd, "mcp", servers)
}

// loadMCP reads just the "mcp" array from the project's .vala.json, returning an
// empty slice when the file or key is absent.
func loadMCP(cwd string) ([]MCPServer, error) {
	path := filepath.Join(cwd, ".vala.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc struct {
		MCP []MCPServer `json:"mcp"`
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
	}
	return doc.MCP, nil
}

// saveKey sets a single top-level key in the project's .vala.json while
// preserving every other key byte-for-byte. The file is created if absent and
// pretty-printed. Secrets are never written here — they stay in the environment,
// as the config comments require.
func saveKey(cwd, key string, value any) error {
	path := filepath.Join(cwd, ".vala.json")

	// Decode into ordered-agnostic raw messages so unrelated keys round-trip
	// unchanged regardless of whether config knows about them.
	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &raw); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	v, err := json.Marshal(value)
	if err != nil {
		return err
	}
	raw[key] = v

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
