package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/config"
	"github.com/spf13/cobra"
)

var (
	flagInitParent string
	flagInitForce  bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Provision the Notion-backed brain and write .vala.json",
	Long: `Init provisions vala's Notion "brain" — the hunting databases that hold
evidence, hunts, intel, detections, and the hunting backlog — and writes their
data-source IDs into .vala.json so your work persists instead of living in an
ephemeral in-memory store.

It requires an authenticated Notion CLI (run "ntn login" first). Databases are
created under the page given by --parent (you are prompted if it is omitted).
A second run is idempotent: existing IDs are verified and reused rather than
duplicated. Use --force to re-provision from scratch.

Secrets are never written to .vala.json — only database IDs. Notion auth stays
in the ntn CLI's own credential store.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cwd, err := resolveConfig()
		if err != nil {
			return err
		}
		return provisionBrain(cmd.Context(), cwd, flagInitParent, flagInitForce)
	},
}

func init() {
	initCmd.Flags().StringVar(&flagInitParent, "parent", "", "Notion page ID to create the brain under (prompted if omitted)")
	initCmd.Flags().BoolVar(&flagInitForce, "force", false, "re-provision even if .vala.json already has a brain configured")
}

// provisionBrain runs the full init flow: preflight, idempotency check, create
// the databases and their relations, create the hunt-page parent, and merge the
// resulting data-source IDs into .vala.json. parent may be empty (the operator
// is prompted); force re-provisions even when a brain is already configured.
func provisionBrain(ctx context.Context, cwd, parent string, force bool) error {
	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}
	store := &brain.NTN{Dir: cwd, DBs: cfg.Notion}

	// Preflight: ntn must be installed and logged in. We never log the user in.
	if err := store.Whoami(ctx); err != nil {
		return fmt.Errorf("the Notion CLI is not authenticated.\n  Install it and run `ntn login`, then re-run `vala init`.\n  (%w)", err)
	}

	// Idempotency: if a brain is already configured and not forcing, verify the
	// data sources still resolve and reuse them rather than duplicating.
	if !force && brainConfigured(cfg) {
		return verifyExisting(ctx, store, cfg)
	}

	if parent == "" {
		parent = promptLine("Notion page ID to create the brain under: ")
		if parent == "" {
			return fmt.Errorf("a parent page ID is required; pass --parent <page-id>")
		}
	}

	specs := brain.Schema()

	// Pass 1: create every database; collect logical name -> data-source ID.
	dsByName := make(map[string]string, len(specs))
	for _, s := range specs {
		_, dsID, err := store.CreateDatabase(ctx, parent, s.Title, s.Props, s.StatusOptions)
		if err != nil {
			return fmt.Errorf("create %s database: %w", s.Name, err)
		}
		dsByName[s.Name] = dsID
		fmt.Fprintf(os.Stderr, "✓ created %s\n", s.Title)
	}

	// Pass 2: wire relation properties now that every target exists.
	for _, s := range specs {
		if len(s.Relations) == 0 {
			continue
		}
		rels := make(map[string]string, len(s.Relations))
		for _, r := range s.Relations {
			target, ok := dsByName[r.Target]
			if !ok {
				return fmt.Errorf("%s relation %q targets unknown store %q", s.Name, r.Name, r.Target)
			}
			rels[r.Name] = target
		}
		if err := store.AddRelations(ctx, dsByName[s.Name], rels); err != nil {
			return fmt.Errorf("add relations to %s: %w", s.Name, err)
		}
	}

	// Create the narrative-page parent the brain writes hunt pages beneath.
	pageID, err := store.CreateChildPage(ctx, parent, "Vala Hunt Pages")
	if err != nil {
		return fmt.Errorf("create hunt-page parent: %w", err)
	}

	ids := brain.DBIDsFromMap(dsByName, pageID)
	if err := config.SaveNotion(cwd, ids); err != nil {
		return fmt.Errorf("write .vala.json: %w", err)
	}

	fmt.Fprintln(os.Stderr, "\n✓ Brain provisioned and saved to .vala.json")
	fmt.Fprintln(os.Stderr, "  Next: run `vala` and try — \"queue a hunt: did anyone disable GuardDuty?\"")
	return nil
}

// verifyExisting checks that the configured data sources still resolve and
// reports the result, leaving .vala.json untouched. It is the idempotent path:
// re-running init on a configured project repairs nothing destructively and
// points the operator at --force if something is actually broken.
func verifyExisting(ctx context.Context, store *brain.NTN, cfg config.Config) error {
	checks := []struct {
		name string
		id   string
	}{
		{"evidence", cfg.Notion.Evidence},
		{"hunts", cfg.Notion.Hunts},
		{"intel", cfg.Notion.Intel},
		{"detections", cfg.Notion.Detections},
		{"backlog", cfg.Notion.Backlog},
	}
	var missing []string
	for _, c := range checks {
		if c.id == "" {
			missing = append(missing, c.name+" (unset)")
			continue
		}
		if !store.DataSourceExists(ctx, c.id) {
			missing = append(missing, c.name+" (unreachable)")
		}
	}
	if len(missing) == 0 {
		fmt.Fprintln(os.Stderr, "✓ Brain already configured; all data sources resolve. Nothing to do.")
		return nil
	}
	fmt.Fprintf(os.Stderr, "Brain is configured but some stores are missing or unreachable:\n")
	for _, m := range missing {
		fmt.Fprintf(os.Stderr, "  - %s\n", m)
	}
	return fmt.Errorf("re-run with --force to re-provision the brain")
}

// promptLine reads a single trimmed line from stdin (empty on EOF/error).
func promptLine(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	line, err := readLine()
	if err != nil {
		return ""
	}
	return line
}
