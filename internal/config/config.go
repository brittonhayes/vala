// Package config loads vala settings, layering (lowest priority first):
// built-in defaults, the user config (~/.config/vala/config.json), the
// project config (./.vala.json), and finally environment variables.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

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

	// Env selects the policy environment for governed runs: dev | prod.
	Env string `json:"env"`
	// Notion holds the database IDs the case brain writes to. Empty IDs mean
	// the brain runs in local (in-memory) mode.
	Notion brain.DBIDs `json:"notion"`

	// APIKey is read from the environment, never persisted.
	APIKey string `json:"-"`
	// SlackWebhook is read from SLACK_WEBHOOK_URL, never persisted.
	SlackWebhook string `json:"-"`
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
		Env:           "dev",
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
	if v := os.Getenv("VALA_ENV"); v != "" {
		cfg.Env = v
	}
	if v := os.Getenv("SLACK_WEBHOOK_URL"); v != "" {
		cfg.SlackWebhook = v
	}
	return cfg, nil
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
