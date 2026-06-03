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
	Short: "Agentic security detection & response harness",
	Long: `Vala is an agentic harness for security detection & response work.

It drives an LLM agent that can investigate, author and validate Sigma
detection rules, run shell/file tools, and document findings in Notion via the
ntn CLI.

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
	rootCmd.AddCommand(runCmd, respondCmd, harnessCmd, versionCmd)
}

// built bundles the constructed dependencies for a command.
type built struct {
	cfg      config.Config
	cwd      string
	client   *llm.Client
	registry *tool.Registry
	gate     *permission.Gate
}

// build resolves config + flags and constructs the shared dependencies.
func build() (*built, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return nil, err
	}
	if flagModel != "" {
		cfg.Model = flagModel
	}
	if flagPermission != "" {
		cfg.Permission = flagPermission
	}

	client, err := llm.New(cfg)
	if err != nil {
		return nil, err
	}
	registry := tools.Default(cwd)
	gate := permission.New(permission.Parse(cfg.Permission), cfg.Allowlist)

	return &built{cfg: cfg, cwd: cwd, client: client, registry: registry, gate: gate}, nil
}
