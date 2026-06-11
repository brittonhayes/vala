package cmd

import (
	"bufio"
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

// firstRunNotice is the non-interactive setup check used by `vala run`. It
// enforces --require-brain and, when a surface is unconfigured, prints a concise
// stderr summary and continues so automation is never blocked. The interactive
// REPL uses the onboarding wizard (maybeRunSetup) instead.
func firstRunNotice(cfg config.Config, cwd string) error {
	if flagRequireBrain && !(brainConfigured(cfg) || cfg.BrainFile != "") {
		return fmt.Errorf("no brain is configured; run `vala init` (or unset --require-brain)")
	}
	if setupComplete(cfg) || flagNoInitPrompt || initPromptDismissed(cwd) {
		return nil
	}
	var gaps []string
	if !providerConfigured(cfg) {
		gaps = append(gaps, "no model provider connected (run `vala connect`)")
	}
	if !(brainConfigured(cfg) || cfg.BrainFile != "") {
		gaps = append(gaps, "no brain — findings will not persist (run `vala init`)")
	}
	if !evidenceConfigured(cfg) {
		gaps = append(gaps, "no evidence sources — nothing to hunt in (run `vala setup`)")
	}
	fmt.Fprintln(os.Stderr, "⚠ vala is not fully set up:")
	for _, g := range gaps {
		fmt.Fprintln(os.Stderr, "  - "+g)
	}
	return nil
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
