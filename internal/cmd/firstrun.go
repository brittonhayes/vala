package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brittonhayes/vala/internal/config"
)

// brain-related flags shared by the REPL and `vala run`.
var (
	flagNoInitPrompt bool
	flagRequireBrain bool
)

// brainConfigured reports whether the project has a live Notion brain wired up.
// It is the single predicate brainStore uses to choose its backend and the
// first-run notice uses to decide whether to warn: any of the hunts, intel, or
// evidence data sources being set means writes persist to Notion.
func brainConfigured(cfg config.Config) bool {
	n := cfg.Notion
	return n.Hunts != "" || n.Intel != "" || n.Evidence != ""
}

const ephemeralNotice = `⚠ No Notion brain is configured — running in ephemeral in-memory mode.
  Hunts, intel, evidence, and detections will NOT persist; they vanish on exit.
  Run ` + "`vala init`" + ` once to provision a persistent Notion-backed brain.`

// firstRunNotice warns when the brain is unconfigured (ephemeral in-memory).
// In an interactive session it offers to run `vala init` now; otherwise it
// prints the warning to stderr and continues so automation is never blocked —
// unless --require-brain is set, which turns the unconfigured state into an
// error. The notice is suppressed by --no-init-prompt or a prior dismissal
// recorded for this project so a deliberate in-memory user is not nagged.
func firstRunNotice(ctx context.Context, cfg config.Config, cwd string, interactive bool) error {
	if brainConfigured(cfg) {
		return nil
	}
	if flagRequireBrain {
		return fmt.Errorf("no Notion brain is configured; run `vala init` (or unset --require-brain)")
	}
	if flagNoInitPrompt || initPromptDismissed(cwd) {
		return nil
	}

	fmt.Fprintln(os.Stderr, ephemeralNotice)
	if !interactive {
		// Non-interactive (e.g. `vala run`): warn and continue, no prompt.
		return nil
	}

	if promptYesNo("Run `vala init` now to set up a persistent brain? [y/N] ") {
		return provisionBrain(ctx, cwd, "", false)
	}
	// Declined: remember it so we don't prompt on every launch.
	dismissInitPrompt(cwd)
	return nil
}

// promptYesNo reads a single line from stdin and reports whether it is an
// affirmative (y / yes, case-insensitive). Anything else — including EOF — is no.
func promptYesNo(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	line, err := readLine()
	if err != nil {
		return false
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// stdin is shared so sequential prompts don't drop input buffered by an earlier
// bufio.Reader.
var stdin = bufio.NewReader(os.Stdin)

// readLine reads one trimmed line from stdin.
func readLine() (string, error) {
	line, err := stdin.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// state is the small per-user marker file vala keeps in the OS config dir. It
// records which project directories have opted out of the first-run prompt so a
// deliberate in-memory user is not asked again.
type state struct {
	DismissedInitPrompt []string `json:"dismissed_init_prompt"`
}

func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vala", "state.json"), nil
}

func loadState() state {
	var s state
	path, err := statePath()
	if err != nil {
		return s
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

// initPromptDismissed reports whether the operator has opted out of the
// first-run prompt for this project directory.
func initPromptDismissed(cwd string) bool {
	abs, _ := filepath.Abs(cwd)
	for _, p := range loadState().DismissedInitPrompt {
		if p == abs {
			return true
		}
	}
	return false
}

// dismissInitPrompt records that this project directory has opted out of the
// first-run prompt. It is best-effort: a write failure just means the operator
// may be asked again next launch.
func dismissInitPrompt(cwd string) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return
	}
	s := loadState()
	for _, p := range s.DismissedInitPrompt {
		if p == abs {
			return
		}
	}
	s.DismissedInitPrompt = append(s.DismissedInitPrompt, abs)

	path, err := statePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if data, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}
