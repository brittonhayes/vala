// Package config loads vala settings, layering (lowest priority first):
// built-in defaults, the user config (~/.config/vala/config.json), the
// project config (./.vala.json), and finally environment variables.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
)

// Config holds runtime settings for vala.
type Config struct {
	// Provider is the LLM provider id the agent talks to: a built-in
	// ("anthropic", "openai", "google", "openrouter", "groq", "deepseek",
	// "xai", "ollama", "lmstudio") or a custom one defined under Providers.
	// Empty defaults to "anthropic", preserving pre-multi-provider configs.
	Provider string `json:"provider"`
	// Model is the model ID used for the agent loop, interpreted within the
	// active provider (e.g. "claude-opus-4-8", "gpt-5", "gemini-2.5-pro").
	Model string `json:"model"`
	// Providers holds custom or overridden provider definitions, keyed by id.
	// Use it to point at a local OpenAI-compatible server or a private gateway.
	Providers map[string]ProviderConfig `json:"providers"`
	// MaxTokens caps the response size per turn.
	MaxTokens int64 `json:"max_tokens"`
	// Permission is the default permission mode: ask | allow | deny. Empty means
	// "derive from Maturity" (see MaturityPermission): the maturity level sets the
	// default autonomy, and an explicit permission (config, env, or flag) wins.
	Permission string `json:"permission"`
	// Mode is the active specialization id the harness runs in: a built-in
	// ("hunt", "detect"). Empty defaults to "hunt", which reproduces the classic
	// full hunt-loop behavior. Unlike Maturity (autonomy), Mode is behavioral: it
	// selects the system prompt, the exposed tool subset, and the bundled skills.
	Mode string `json:"mode"`
	// Maturity is the Hunting Maturity Model level (0–4) the harness runs at. It
	// tunes autonomy: it sets the default permission mode and frames the agent's
	// gating in the system prompt. It is NOT a behavioral mode — the loop and
	// tools are identical at every level; only how much runs without approval
	// changes. 0 (initial) investigates only; 1–2 (minimal/procedural) ask before
	// writes; 3–4 (innovative/leading) run autonomously.
	Maturity int `json:"maturity"`
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
	// the brain runs in local mode (file-backed if BrainFile is set, else
	// in-memory).
	Notion brain.DBIDs `json:"notion"`
	// BrainFile, when set and no Notion brain is configured, persists the brain
	// to a JSON file on disk instead of the ephemeral in-memory store — a durable
	// brain with no Notion account. A relative path resolves against the project.
	BrainFile string `json:"brain_file"`
	// MCP lists the Model Context Protocol servers vala connects to for evidence
	// (e.g. Scanner's security data lake). Each server's tools are discovered at
	// startup and exposed to the agent. Empty means no remote evidence source.
	MCP []MCPServer `json:"mcp"`

	// APIKey is read from the environment, never persisted.
	APIKey string `json:"-"`
}

// ProviderConfig describes a custom or overridden LLM provider. For a built-in
// provider, any non-empty field overrides the registry default; for a brand-new
// provider id, it defines the endpoint outright (protocol defaults to the
// OpenAI-compatible wire format, which most servers speak). Secrets are never
// stored here — only the name of the environment variable that holds the key.
type ProviderConfig struct {
	// BaseURL is the API endpoint (required for a custom OpenAI-compatible
	// provider, e.g. a local server or private gateway).
	BaseURL string `json:"base_url"`
	// Protocol is the wire format: "anthropic" or "openai". Empty means openai.
	Protocol string `json:"protocol"`
	// APIKeyEnv names the environment variable holding the provider's API key.
	APIKeyEnv string `json:"api_key_env"`
	// Model is the default model id for this provider when none is configured.
	Model string `json:"model"`
	// Local marks a provider that runs on the operator's machine and needs no
	// API key.
	Local bool `json:"local"`
}

// MCPServer describes one Model Context Protocol server vala connects to. The
// transport selects how it is reached: "http" (the default) dials URL with the
// bearer token from APIKeyEnv; "stdio" launches Command/Args as a local
// subprocess. Secrets are never stored here — only the names of the environment
// variables that hold them, resolved at load time.
type MCPServer struct {
	// Name namespaces the server's tools inside vala (e.g. "scanner" yields
	// tools like "scanner_execute_query").
	Name string `json:"name"`
	// Transport is "http" (default when empty) or "stdio".
	Transport string `json:"transport,omitempty"`

	// URL is the server's streamable-HTTP endpoint (http transport).
	URL string `json:"url,omitempty"`
	// APIKeyEnv names the environment variable holding the bearer token (http
	// transport).
	APIKeyEnv string `json:"api_key_env,omitempty"`
	// OAuth marks an HTTP server that authorizes via the MCP OAuth flow (browser
	// sign-in + dynamic client registration) rather than a static bearer token.
	// Wiz's remote MCP server works this way. Tokens are cached out of band, never
	// in this file.
	OAuth bool `json:"oauth,omitempty"`

	// Command is the executable to launch (stdio transport).
	Command string `json:"command,omitempty"`
	// Args are the command's arguments (stdio transport).
	Args []string `json:"args,omitempty"`
	// EnvPassthrough names environment variables to forward to the subprocess
	// (stdio transport). Values are read from the operator's environment at load
	// time so secrets stay out of the config file.
	EnvPassthrough []string `json:"env,omitempty"`

	// APIKey is resolved from APIKeyEnv, never persisted.
	APIKey string `json:"-"`
	// Env holds the resolved name->value pairs for EnvPassthrough, never
	// persisted.
	Env map[string]string `json:"-"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Provider:      "anthropic",
		Model:         "claude-opus-4-8",
		Mode:          "hunt",
		MaxTokens:     8192,
		Permission:    "", // derived from Maturity unless set explicitly
		Maturity:      1,  // minimal: ask before writes
		Allowlist:     nil,
		DetectionsDir: "detections",
		MaxSteps:      50,

		ContextWindow:        200000,
		AutoCompactThreshold: 0.80,
	}
}

// MaturityPermission maps a Hunting Maturity Model level to the default
// permission mode it implies: HMM0 investigates read-only (deny writes), HMM1–2
// ask before each write, and HMM3–4 run autonomously (allow). It is the default
// only — an explicitly configured permission always wins.
func MaturityPermission(level int) string {
	switch {
	case level <= 0:
		return "deny"
	case level >= 3:
		return "allow"
	default:
		return "ask"
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
	if v := os.Getenv("VALA_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if v := os.Getenv("VALA_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("VALA_PERMISSION"); v != "" {
		cfg.Permission = v
	}
	if v := os.Getenv("VALA_MODE"); v != "" {
		cfg.Mode = v
	}
	if v := os.Getenv("VALA_MATURITY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Maturity = n
		}
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
	// Resolve each server's secrets from the environment so they stay out of the
	// config file: the bearer token from APIKeyEnv (http) and any passthrough
	// variables forwarded to a stdio subprocess.
	for i := range cfg.MCP {
		if env := cfg.MCP[i].APIKeyEnv; env != "" {
			cfg.MCP[i].APIKey = os.Getenv(env)
		}
		for _, name := range cfg.MCP[i].EnvPassthrough {
			if v, ok := os.LookupEnv(name); ok {
				if cfg.MCP[i].Env == nil {
					cfg.MCP[i].Env = make(map[string]string)
				}
				cfg.MCP[i].Env[name] = v
			}
		}
	}

	// Derive the default permission from the maturity level when no explicit
	// permission was set anywhere (config file or VALA_PERMISSION). An explicit
	// permission — including the --permission flag applied after Load — always
	// wins, since it leaves Permission non-empty.
	if cfg.Permission == "" {
		cfg.Permission = MaturityPermission(cfg.Maturity)
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

// NotionSearchServerName is the conventional MCP server name vala treats as the
// hunt brain's search backend rather than an evidence source: its search tool
// powers recall, and it is deliberately NOT exposed to the agent as a freelance
// tool so recall stays the single curated read surface over the brain.
const NotionSearchServerName = "notion"

// NotionSearchServer returns the configured MCP server that backs brain search
// (the one named "notion"), if any.
func (c Config) NotionSearchServer() (MCPServer, bool) {
	for _, s := range c.MCP {
		if strings.EqualFold(s.Name, NotionSearchServerName) {
			return s, true
		}
	}
	return MCPServer{}, false
}

// IsNotionSearchServer reports whether srv is the brain's search backend, so the
// evidence-connection path can skip it (it is wired into recall instead).
func (c Config) IsNotionSearchServer(srv MCPServer) bool {
	return strings.EqualFold(srv.Name, NotionSearchServerName)
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
