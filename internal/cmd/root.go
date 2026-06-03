// Package cmd wires the vala CLI together with cobra.
package cmd

import (
	"fmt"
	"os"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/config"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/session"
	"github.com/brittonhayes/vala/internal/tool"
	"github.com/brittonhayes/vala/internal/tools"
	"github.com/brittonhayes/vala/internal/ui"
	"github.com/spf13/cobra"
)

// persisted flag values shared across commands.
var (
	flagModel      string
	flagPermission string
)

// rootCmd starts the interactive REPL by default.
var rootCmd = &cobra.Command{
	Use:   "vala",
	Short: "Agentic security harness for threat hunting, detection, and response",
	Long: `vala is an agentic security harness that orchestrates a Notion-backed brain
to hunt threats, build detections, and work alerts.

Rather than statically searching a SIEM by hand, vala explores: it investigates
a threat question against a hypothesis, stores the hunt and any threat
intelligence it surfaces in Notion as first-class artifacts, connects intel,
hunts, alerts, and detections into one graph, and feeds what it learns back into
detection development.

  vala hunt     explore a threat question and store the hunt
  vala intel    record and link threat intelligence
  vala respond  work an alert through the governed response loop
  vala run      author and validate Sigma detections non-interactively

Run with no arguments to start an interactive session, or use "vala run"
for a single non-interactive task.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		built, err := build()
		if err != nil {
			return err
		}
		sess, err := session.New(session.DefaultDir())
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning: transcript disabled:", err)
		}
		ag := agent.New(built.client, built.registry, built.gate, built.cwd, built.cfg.MaxSteps)
		repl := ui.New(ag, built.gate, sess, built.client.Model())
		return repl.Run(cmd.Context())
	},
}

// Execute is the CLI entry point.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagModel, "model", "", "Anthropic model ID (overrides config)")
	rootCmd.PersistentFlags().StringVar(&flagPermission, "permission", "", "permission mode: ask | allow | deny")
	rootCmd.AddCommand(runCmd, respondCmd, huntCmd, intelCmd, harnessCmd, versionCmd)
}

// built bundles the constructed dependencies for a command.
type built struct {
	cfg      config.Config
	cwd      string
	client   *llm.Client
	registry *tool.Registry
	gate     *permission.Gate
}

// resolveConfig loads config for the current directory and applies persistent
// flag overrides. It does not construct the LLM client, so commands that only
// touch the brain (e.g. `vala intel`) can run without an API key.
func resolveConfig() (config.Config, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return config.Config{}, "", err
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return cfg, cwd, err
	}
	if flagModel != "" {
		cfg.Model = flagModel
	}
	if flagPermission != "" {
		cfg.Permission = flagPermission
	}
	return cfg, cwd, nil
}

// build resolves config + flags and constructs the shared dependencies,
// including the LLM client (which requires an API key).
func build() (*built, error) {
	cfg, cwd, err := resolveConfig()
	if err != nil {
		return nil, err
	}
	client, err := llm.New(cfg)
	if err != nil {
		return nil, err
	}
	registry := tools.Default(cwd)
	gate := permission.New(permission.Parse(cfg.Permission), cfg.Allowlist)

	return &built{cfg: cfg, cwd: cwd, client: client, registry: registry, gate: gate}, nil
}
