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

// SaveBrainFile records the local brain-file path in .vala.json (the
// "brain_file" key) so a file-backed brain persists across runs without a Notion
// account, preserving every other key.
func SaveBrainFile(cwd, brainFile string) error {
	return saveKey(cwd, "brain_file", brainFile)
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
