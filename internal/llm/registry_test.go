package llm

import "testing"

func TestProvidersOrderedRecommendedFirst(t *testing.T) {
	ps := Providers()
	if len(ps) != len(builtins) {
		t.Fatalf("Providers() returned %d, want %d", len(ps), len(builtins))
	}
	// Anthropic, OpenAI, Google lead the list as the recommended providers.
	want := []string{"anthropic", "openai", "google"}
	for i, id := range want {
		if ps[i].ID != id {
			t.Errorf("Providers()[%d] = %q, want %q", i, ps[i].ID, id)
		}
	}
}

func TestBuiltinLookup(t *testing.T) {
	if _, ok := Builtin("anthropic"); !ok {
		t.Fatal("anthropic should be a built-in provider")
	}
	if _, ok := Builtin("nope"); ok {
		t.Fatal("unknown provider should not resolve")
	}
}

func TestContextWindowFor(t *testing.T) {
	// Opus 4.8 ships a 1M-token window natively (the catalog's source of truth).
	if got := contextWindowFor("anthropic", "claude-opus-4-8"); got != 1000000 {
		t.Errorf("known model window = %d, want 1000000", got)
	}
	// An unknown model falls back to the conservative default.
	if got := contextWindowFor("ollama", "some-random-local-model"); got != defaultContextWindow {
		t.Errorf("unknown model window = %d, want %d", got, defaultContextWindow)
	}
}

func TestCatalogModels(t *testing.T) {
	if models := CatalogModels("anthropic"); len(models) == 0 {
		t.Fatal("expected curated models for anthropic")
	}
	if models := CatalogModels("lmstudio"); models != nil {
		t.Fatalf("expected nil models for a local provider, got %v", models)
	}
}
