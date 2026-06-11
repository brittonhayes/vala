package llm

// ModelInfo is the curated metadata vala knows about a model. It is embedded in
// the binary so vala stays self-contained — no network call to a model catalog
// at startup. Unknown models still work; they just fall back to a default
// context window and the provider's protocol defaults.
type ModelInfo struct {
	// ID is the model identifier passed to the provider API.
	ID string
	// ContextWindow is the usable context size in tokens, used to decide when to
	// auto-compact. 0 means unknown.
	ContextWindow int64
}

// catalog maps a provider id to its known models. It is intentionally curated
// rather than exhaustive: it covers the defaults vala ships with and the most
// common models per provider, so context-window-aware auto-compaction works out
// of the box. Operators can point vala at any model id not listed here; it will
// run with defaultContextWindow.
var catalog = map[string][]ModelInfo{
	"anthropic": {
		// Opus 4.8 and Sonnet 4.6 ship a 1M-token window natively (the default,
		// at standard pricing — no context-1m beta header on the 4.x family).
		{ID: "claude-opus-4-8", ContextWindow: 1000000},
		{ID: "claude-opus-4-1", ContextWindow: 200000},
		{ID: "claude-sonnet-4-6", ContextWindow: 1000000},
		{ID: "claude-sonnet-4-5", ContextWindow: 200000},
		{ID: "claude-haiku-4-5", ContextWindow: 200000},
		{ID: "claude-3-5-haiku-latest", ContextWindow: 200000},
	},
	"openai": {
		{ID: "gpt-5", ContextWindow: 400000},
		{ID: "gpt-5-mini", ContextWindow: 400000},
		{ID: "gpt-4.1", ContextWindow: 1047576},
		{ID: "gpt-4o", ContextWindow: 128000},
		{ID: "gpt-4o-mini", ContextWindow: 128000},
		{ID: "o4-mini", ContextWindow: 200000},
	},
	"google": {
		{ID: "gemini-2.5-pro", ContextWindow: 1048576},
		{ID: "gemini-2.5-flash", ContextWindow: 1048576},
		{ID: "gemini-2.0-flash", ContextWindow: 1048576},
	},
	"openrouter": {
		{ID: "anthropic/claude-opus-4-8", ContextWindow: 1000000},
		{ID: "openai/gpt-5", ContextWindow: 400000},
		{ID: "google/gemini-2.5-pro", ContextWindow: 1048576},
	},
	"groq": {
		{ID: "llama-3.3-70b-versatile", ContextWindow: 131072},
		{ID: "openai/gpt-oss-120b", ContextWindow: 131072},
	},
	"deepseek": {
		{ID: "deepseek-chat", ContextWindow: 131072},
		{ID: "deepseek-reasoner", ContextWindow: 131072},
	},
	"xai": {
		{ID: "grok-4", ContextWindow: 256000},
		{ID: "grok-3", ContextWindow: 131072},
	},
}

// defaultContextWindow is used for any model not found in the catalog (notably
// local Ollama/LM Studio models, whose windows vary by quantization). It is
// deliberately conservative so auto-compaction triggers early rather than
// overflowing a smaller local window.
const defaultContextWindow int64 = 128000

// lookupModel returns the catalog entry for a provider/model pair, if known.
func lookupModel(provider, model string) (ModelInfo, bool) {
	for _, m := range catalog[provider] {
		if m.ID == model {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// contextWindowFor returns the known context window for a provider/model, or
// defaultContextWindow when the model is not in the catalog.
func contextWindowFor(provider, model string) int64 {
	if m, ok := lookupModel(provider, model); ok && m.ContextWindow > 0 {
		return m.ContextWindow
	}
	return defaultContextWindow
}

// CatalogModels returns the known model ids for a provider, for display in the
// connect flow. It returns nil for providers with no curated entries (e.g.
// local servers, whose model list is whatever the user has pulled).
func CatalogModels(provider string) []string {
	infos := catalog[provider]
	if len(infos) == 0 {
		return nil
	}
	out := make([]string, 0, len(infos))
	for _, m := range infos {
		out = append(out, m.ID)
	}
	return out
}
