package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/hunt"
	"github.com/brittonhayes/vala/internal/policy"
	"github.com/spf13/cobra"
)

var flagPromote bool

var huntCmd = &cobra.Command{
	Use:   "hunt <question>",
	Short: "Explore a threat question and store the hunt in the brain",
	Long: `Hunt drives a hypothesis-driven threat hunt: vala states a hypothesis for the
question, explores read-only data sources, records each finding with an evidence
pointer, and stores the hunt (with its verdict) in the Notion-backed brain.

Findings and any surfaced threat intelligence become first-class, connected
artifacts. With --promote, a hunt that confirms its hypothesis is authored into a
Sigma detection and linked back to the hunt.

Notion database IDs in config enable real Notion writes; without them the brain
runs in local mode and the hunt page is printed to stdout.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		built, err := build()
		if err != nil {
			return err
		}

		pol, err := policy.Load(built.cwd)
		if err != nil {
			return fmt.Errorf("load policy: %w", err)
		}

		store := brainStore(built.cfg, built.cwd)
		bc := brain.New(store)

		eng := &hunt.Engine{
			Client:  built.client,
			Gate:    built.gate,
			Brain:   bc,
			Policy:  pol,
			Env:     built.cfg.Env,
			Dir:     built.cwd,
			Commit:  gitCommit(built.cwd),
			Promote: flagPromote,
			Events:  respondEvents(),
		}

		question := strings.Join(args, " ")
		res, err := eng.RunHunt(cmd.Context(), question)
		if err != nil {
			return err
		}

		printHuntResult(res)
		if mem, ok := store.(*brain.Mem); ok {
			for _, page := range mem.Pages {
				fmt.Println("\n--- hunt page ---")
				fmt.Println(page)
			}
		}
		return nil
	},
}

func init() {
	huntCmd.Flags().BoolVar(&flagPromote, "promote", false, "author a Sigma detection from a confirmed hunt and link it back")
}

func printHuntResult(r *hunt.Result) {
	fmt.Fprintf(os.Stderr, "\nhunt %s — %s\n", r.HuntID, r.Status)
	fmt.Fprintf(os.Stderr, "findings: %d  detection promoted: %t\n", len(r.Findings), r.DetectionPromoted)
}
