// Package config loads vala settings, layering (lowest priority first):
// built-in defaults, the user config (~/.config/vala/config.json), the
// project config (./.vala.json), and finally environment variables.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/brittonhayes/vala/internal/brain"
)

// Config holds runtime settings for vala.
type Config struct {
	// Model is the Anthropic model ID used for the agent loop.
	Model string `json:"model"`
	// MaxTokens caps the response size per turn.
	MaxTokens int64 `json:"max_tokens"`
	// Permission is the default permission mode: ask | allow | deny.
	Permission string `json:"permission"`
	// Allowlist names tools that may run without prompting.
	Allowlist []string `json:"allowlist"`
	// DetectionsDir is where Sigma detection rules live in a project.
	DetectionsDir string `json:"detections_dir"`
	// MaxSteps bounds tool-use iterations per user turn (loop guard).
	MaxSteps int `json:"max_steps"`

	// ContextWindow is the model's usable context size in tokens, used to decide
	// when to auto-compact the conversation. 0 disables auto-compaction.
	ContextWindow int64 `json:"context_window"`
	// AutoCompactThreshold is the fraction (0..1) of ContextWindow at which vala
	// optimistically compacts the conversation before continuing. 0 disables it.
	AutoCompactThreshold float64 `json:"auto_compact_threshold"`

	// Notion holds the database IDs the hunt brain writes to. Empty IDs mean
	// the brain runs in local (in-memory) mode.
	Notion brain.DBIDs `json:"notion"`
	// MCP lists the Model Context Protocol servers vala connects to for evidence
	// (e.g. Scanner's security data lake). Each server's tools are discovered at
	// startup and exposed to the agent. Empty means no remote evidence source.
	MCP []MCPServer `json:"mcp"`

	// APIKey is read from the environment, never persisted.
	APIKey string `json:"-"`
}

// MCPServer describes one Model Context Protocol server vala connects to. The
// API key is resolved from APIKeyEnv at load time so secrets stay in the
// environment rather than the config file.
type MCPServer struct {
	// Name namespaces the server's tools inside vala (e.g. "scanner" yields
	// tools like "scanner_execute_query").
	Name string `json:"name"`
	// URL is the server's streamable-HTTP endpoint.
	URL string `json:"url"`
	// APIKeyEnv names the environment variable holding the bearer token.
	APIKeyEnv string `json:"api_key_env"`
	// APIKey is resolved from APIKeyEnv, never persisted.
	APIKey string `json:"-"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Model:         "claude-opus-4-8",
		MaxTokens:     8192,
		Permission:    "ask",
		Allowlist:     nil,
		DetectionsDir: "detections",
		MaxSteps:      50,

		ContextWindow:        200000,
		AutoCompactThreshold: 0.80,
	}
}

// Load resolves the effective configuration for the given working directory.
func Load(cwd string) (Config, error) {
	cfg := Default()

	if home, err := os.UserConfigDir(); err == nil {
		_ = mergeFile(&cfg, filepath.Join(home, "vala", "config.json"))
	}
	if err := mergeFile(&cfg, filepath.Join(cwd, ".vala.json")); err != nil {
		return cfg, err
	}

	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("VALA_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("VALA_PERMISSION"); v != "" {
		cfg.Permission = v
	}
	if v := os.Getenv("VALA_CONTEXT_WINDOW"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.ContextWindow = n
		}
	}
	if v := os.Getenv("VALA_AUTO_COMPACT_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.AutoCompactThreshold = f
		}
	}

	// Convenience: SCANNER_MCP_URL registers Scanner's data lake as a server
	// without a config file, keyed by SCANNER_API_KEY.
	if url := os.Getenv("SCANNER_MCP_URL"); url != "" && !hasMCPServer(cfg.MCP, "scanner") {
		cfg.MCP = append(cfg.MCP, MCPServer{Name: "scanner", URL: url, APIKeyEnv: "SCANNER_API_KEY"})
	}
	// Resolve each server's bearer token from its named environment variable.
	for i := range cfg.MCP {
		if env := cfg.MCP[i].APIKeyEnv; env != "" {
			cfg.MCP[i].APIKey = os.Getenv(env)
		}
	}
	return cfg, nil
}

// hasMCPServer reports whether a server with the given name is already present.
func hasMCPServer(servers []MCPServer, name string) bool {
	for _, s := range servers {
		if s.Name == name {
			return true
		}
	}
	return false
}

// mergeFile overlays a JSON config file onto cfg if the file exists. A missing
// file is not an error; malformed JSON is.
func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, cfg)
}
