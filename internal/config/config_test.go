package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCompactionSettings(t *testing.T) {
	cfg := Default()
	if cfg.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000", cfg.ContextWindow)
	}
	if cfg.AutoCompactThreshold != 0.80 {
		t.Errorf("AutoCompactThreshold = %v, want 0.80", cfg.AutoCompactThreshold)
	}
}

func TestDefaultProvider(t *testing.T) {
	cfg := Default()
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", cfg.Provider)
	}
	if cfg.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q, want claude-opus-4-8", cfg.Model)
	}
}

func TestDefaultMode(t *testing.T) {
	if cfg := Default(); cfg.Mode != "hunt" {
		t.Errorf("Mode = %q, want hunt", cfg.Mode)
	}
}

func TestLoadModeEnvOverride(t *testing.T) {
	t.Setenv("VALA_MODE", "detect")
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != "detect" {
		t.Errorf("Mode = %q, want detect", cfg.Mode)
	}
}

func TestLoadModeProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".vala.json"), []byte(`{"mode":"detect"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != "detect" {
		t.Errorf("Mode = %q, want detect from project config", cfg.Mode)
	}
}

func TestLoadProviderEnvOverride(t *testing.T) {
	t.Setenv("VALA_PROVIDER", "openai")
	t.Setenv("VALA_MODEL", "gpt-5")
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "openai" || cfg.Model != "gpt-5" {
		t.Errorf("got provider %q model %q, want openai/gpt-5", cfg.Provider, cfg.Model)
	}
}

func TestSaveProviderPreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	// Seed an unrelated key, then SaveProvider must leave it intact.
	if err := saveKey(dir, "detections_dir", "rules"); err != nil {
		t.Fatal(err)
	}
	if err := SaveProvider(dir, "google", "gemini-2.5-pro"); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "google" || cfg.Model != "gemini-2.5-pro" {
		t.Errorf("provider/model not saved: %q/%q", cfg.Provider, cfg.Model)
	}
	if cfg.DetectionsDir != "rules" {
		t.Errorf("unrelated key clobbered: detections_dir = %q", cfg.DetectionsDir)
	}
}

func TestLoadCompactionEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VALA_CONTEXT_WINDOW", "50000")
	t.Setenv("VALA_AUTO_COMPACT_THRESHOLD", "0.5")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ContextWindow != 50000 {
		t.Errorf("ContextWindow = %d, want 50000", cfg.ContextWindow)
	}
	if cfg.AutoCompactThreshold != 0.5 {
		t.Errorf("AutoCompactThreshold = %v, want 0.5", cfg.AutoCompactThreshold)
	}
}

func TestMaturityPermissionMapping(t *testing.T) {
	cases := map[int]string{0: "deny", 1: "ask", 2: "ask", 3: "allow", 4: "allow"}
	for level, want := range cases {
		if got := MaturityPermission(level); got != want {
			t.Errorf("MaturityPermission(%d) = %q, want %q", level, got, want)
		}
	}
}

func TestLoadDerivesPermissionFromMaturity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VALA_MATURITY", "4")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Maturity != 4 {
		t.Fatalf("Maturity = %d, want 4", cfg.Maturity)
	}
	if cfg.Permission != "allow" {
		t.Errorf("Permission = %q, want allow (derived from HMM4)", cfg.Permission)
	}
}

func TestExplicitPermissionWinsOverMaturity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VALA_MATURITY", "4") // would imply allow
	t.Setenv("VALA_PERMISSION", "deny")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Permission != "deny" {
		t.Errorf("Permission = %q, want deny (explicit wins over maturity)", cfg.Permission)
	}
}

func TestLoadCompactionEnvIgnoresGarbage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VALA_CONTEXT_WINDOW", "not-a-number")
	t.Setenv("VALA_AUTO_COMPACT_THRESHOLD", "nope")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Malformed env values are ignored, leaving the defaults intact.
	if cfg.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000 (default)", cfg.ContextWindow)
	}
	if cfg.AutoCompactThreshold != 0.80 {
		t.Errorf("AutoCompactThreshold = %v, want 0.80 (default)", cfg.AutoCompactThreshold)
	}
}
