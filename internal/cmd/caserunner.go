package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/config"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/policy"
	"github.com/brittonhayes/vala/internal/respond"
	"github.com/brittonhayes/vala/internal/tool"
)

// caseRunner adapts the governed respond engine to the tools.CaseRunner the
// open_case tool calls. Each case gets a fresh brain store, RunContext, and
// approval ledger inside respond.Engine.RunCase, so the governance machine runs
// exactly as it did for the old `vala respond` command — just reached through a
// tool now instead of a top-level command.
type caseRunner struct {
	cfg      config.Config
	cwd      string
	client   *llm.Client
	gate     *permission.Gate
	policy   *policy.Set
	evidence []tool.Tool
}

// RunCase walks an alert through the governed loop and returns a summary string
// for the agent to relay.
func (cr *caseRunner) RunCase(ctx context.Context, alert brain.Alert, title string) (string, error) {
	store := brainStore(cr.cfg, cr.cwd)
	eng := &respond.Engine{
		Client:        cr.client,
		Gate:          cr.gate,
		Brain:         brain.New(store),
		Policy:        cr.policy,
		Env:           cr.cfg.Env,
		Dir:           cr.cwd,
		Commit:        gitCommit(cr.cwd),
		Webhook:       cr.cfg.SlackWebhook,
		EvidenceTools: cr.evidence,
	}
	res, err := eng.RunCase(ctx, alert, title)
	if err != nil {
		return "", err
	}
	return formatCaseSummary(res, store), nil
}

func formatCaseSummary(res *respond.Result, store brain.Notion) string {
	var b strings.Builder
	fmt.Fprintf(&b, "case %s — reached phase %s\n", res.CaseID, res.PhaseReached)
	fmt.Fprintf(&b, "evidence: %d  actions: %d  executed: %d", len(res.Evidence), len(res.Actions), len(res.Executed))
	if len(res.Violations) > 0 {
		fmt.Fprintf(&b, "  violations: %d", len(res.Violations))
	}
	if mem, ok := store.(*brain.Mem); ok {
		for _, page := range mem.Pages {
			b.WriteString("\n\n--- case page ---\n")
			b.WriteString(page)
		}
	}
	return b.String()
}

// brainStore returns an NTN-backed store when Notion DB IDs are configured,
// otherwise an in-memory store for local runs. Any of the case-brain, hunts, or
// intel databases being set is enough to treat the workspace as configured.
func brainStore(cfg config.Config, cwd string) brain.Notion {
	n := cfg.Notion
	if n.Cases != "" || n.Hunts != "" || n.Intel != "" {
		return &brain.NTN{Dir: cwd, DBs: n}
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
