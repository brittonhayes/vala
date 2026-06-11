package cmd

import (
	"testing"

	"github.com/brittonhayes/vala/internal/config"
)

func TestEvidenceConfigured(t *testing.T) {
	if evidenceConfigured(config.Config{}) {
		t.Error("no MCP servers should report not configured")
	}
	cfg := config.Config{MCP: []config.MCPServer{{Name: "scanner"}}}
	if !evidenceConfigured(cfg) {
		t.Error("a configured MCP server should report configured")
	}
}

func TestProviderConfiguredFromEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	if !providerConfigured(config.Config{Provider: "anthropic"}) {
		t.Error("an API key in the environment should satisfy provider detection")
	}
}

func TestProviderConfiguredLocalNeedsNoKey(t *testing.T) {
	if !providerConfigured(config.Config{Provider: "ollama"}) {
		t.Error("a local provider needs no key and should report configured")
	}
}

func TestSetupCompleteRequiresAllThree(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	// Provider satisfied, but no brain and no evidence.
	if setupComplete(config.Config{Provider: "anthropic"}) {
		t.Error("setup should be incomplete without a brain and evidence")
	}
	cfg := config.Config{
		Provider:  "anthropic",
		BrainFile: ".vala/brain.json",
		MCP:       []config.MCPServer{{Name: "scanner"}},
	}
	if !setupComplete(cfg) {
		t.Error("provider + brain + evidence should be complete")
	}
}
