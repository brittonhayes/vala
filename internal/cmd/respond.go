package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/policy"
	"github.com/brittonhayes/vala/internal/respond"
	"github.com/spf13/cobra"
)

var flagApproveAll bool

var respondCmd = &cobra.Command{
	Use:   "respond <alert.json>",
	Short: "Work an alert through the governed detection & response loop",
	Long: `Respond ingests an alert and drives it through vala's phase-separated
governance loop (plan -> evidence -> propose -> approval -> execute -> report),
writing a Case, Evidence, Actions, and a narrative page to the case brain.

The alert file is JSON: {"alert_id":"...","source":"...","severity":"...","raw":"..."}.

Notion database IDs in config enable real Notion writes; without them the brain
runs in local mode and the case page is printed to stdout.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		built, err := build()
		if err != nil {
			return err
		}

		raw, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read alert: %w", err)
		}
		var alert brain.Alert
		if err := json.Unmarshal(raw, &alert); err != nil {
			return fmt.Errorf("parse alert: %w", err)
		}

		pol, err := policy.Load(built.cwd)
		if err != nil {
			return fmt.Errorf("load policy: %w", err)
		}

		store := brainStore(built)
		bc := brain.New(store)

		eng := &respond.Engine{
			Client:  built.client,
			Gate:    built.gate,
			Brain:   bc,
			Policy:  pol,
			Env:     built.cfg.Env,
			Dir:     built.cwd,
			Commit:  gitCommit(built.cwd),
			Webhook: built.cfg.SlackWebhook,
			Events:  respondEvents(),
		}
		if flagApproveAll {
			eng.Approver = func(governance.ProposedAction) bool { return true }
		}

		title := alert.AlertID
		if title == "" {
			title = "incident-" + alert.Source
		}
		res, err := eng.RunCase(cmd.Context(), alert, title)
		if err != nil {
			return err
		}

		printResult(res)
		if mem, ok := store.(*brain.Mem); ok {
			for _, page := range mem.Pages {
				fmt.Println("\n--- case page ---")
				fmt.Println(page)
			}
		}
		return nil
	},
}

func init() {
	respondCmd.Flags().BoolVar(&flagApproveAll, "approve-all", false, "approve every proposed action (otherwise only policy auto-approvals)")
}

// brainStore returns an NTN-backed store when Notion DB IDs are configured,
// otherwise an in-memory store for local runs.
func brainStore(b *built) brain.Notion {
	if b.cfg.Notion.Cases != "" {
		return &brain.NTN{Dir: b.cwd, DBs: b.cfg.Notion}
	}
	return brain.NewMem()
}

func gitCommit(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func respondEvents() agent.Events {
	return agent.Events{
		OnAssistantText: func(text string) { fmt.Println(strings.TrimSpace(text)) },
		OnToolCall:      func(name, summary string) { fmt.Fprintf(os.Stderr, "⚙ %s %s\n", name, summary) },
		OnPermissionDenied: func(name, summary string) {
			fmt.Fprintf(os.Stderr, "✗ denied: %s\n", name)
		},
	}
}

func printResult(r *respond.Result) {
	fmt.Fprintf(os.Stderr, "\ncase %s — reached phase %s\n", r.CaseID, r.PhaseReached)
	fmt.Fprintf(os.Stderr, "evidence: %d  actions: %d  executed: %d\n", len(r.Evidence), len(r.Actions), len(r.Executed))
}
