package cmd

import (
	"testing"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/config"
)

// TestBrainStoreSelectsBackend pins the backend-selection precedence: Notion
// wins when configured, a brain file gives a durable store, and an unconfigured
// brain falls back to ephemeral memory.
func TestBrainStoreSelectsBackend(t *testing.T) {
	cwd := t.TempDir()

	if _, ok := brainStore(config.Config{}, cwd).(*brain.Mem); !ok {
		t.Fatalf("unconfigured brain should be *brain.Mem")
	}
	if _, ok := brainStore(config.Config{BrainFile: "brain.json"}, cwd).(*brain.File); !ok {
		t.Fatalf("brain_file should select *brain.File")
	}
	cfg := config.Config{BrainFile: "brain.json", Notion: brain.DBIDs{Hunts: "ds"}}
	if _, ok := brainStore(cfg, cwd).(*brain.NTN); !ok {
		t.Fatalf("configured Notion should win and select *brain.NTN")
	}
}
