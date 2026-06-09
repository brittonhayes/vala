// Package cmd wires the vala CLI together with cobra.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/config"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/mcp"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/session"
	"github.com/brittonhayes/vala/internal/tool"
	"github.com/brittonhayes/vala/internal/tools"
	"github.com/brittonhayes/vala/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// persisted flag values shared across commands.
var (
	flagModel      string
	flagPermission string
)

// rootCmd starts the interactive REPL by default.
var rootCmd = &cobra.Command{
	Use:   "vala",
	Short: "Agentic security harness for threat hunting and detection",
	Long: `vala is a single agentic security harness: one interactive session with a
toolbox the agent composes to hunt threats, record and link threat
intelligence, and author and validate detections — documenting it all in a
Notion-backed brain.

There is one surface and one set of tools. Workflows are not separate commands;
they are things you ask the agent to do, and it reaches for the right
primitives: open_hunt to investigate a question, record_intel/link_artifacts to
build the intel graph, and the detection-authoring tools to write Sigma rules.

Run with no arguments to start an interactive session, or use "vala run" for a
single non-interactive task.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, cwd, err := resolveConfig()
		if err != nil {
			return err
		}
		// First-run brain check: when no Notion brain is configured, warn that
		// the session is ephemeral and (in a TTY) offer to run `vala init`.
		interactive := term.IsTerminal(int(os.Stdin.Fd()))
		if err := firstRunNotice(cmd.Context(), cfg, cwd, interactive); err != nil {
			return err
		}
		built, err := build()
		if err != nil {
			return err
		}
		sess, err := session.New(session.DefaultDir())
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning: transcript disabled:", err)
		}
		ag := agent.New(built.client, built.registry, built.gate, built.cwd, built.cfg.MaxSteps)
		repl := ui.New(ag, built.gate, sess, built.client.Model(), built.cfg.ContextWindow, built.cfg.AutoCompactThreshold)
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
	rootCmd.PersistentFlags().BoolVar(&flagNoInitPrompt, "no-init-prompt", false, "suppress the first-run notice when no Notion brain is configured")
	rootCmd.PersistentFlags().BoolVar(&flagRequireBrain, "require-brain", false, "fail instead of falling back to the ephemeral in-memory brain")
	rootCmd.AddCommand(runCmd, versionCmd, initCmd)
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
// flag overrides. It does not construct the LLM client, so callers that only
// touch the brain can run without an API key.
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
// including the LLM client (which requires an API key), and assembles the one
// unified toolbox the harness runs with.
func build() (*built, error) {
	cfg, cwd, err := resolveConfig()
	if err != nil {
		return nil, err
	}
	client, err := llm.New(cfg)
	if err != nil {
		return nil, err
	}
	gate := permission.New(permission.Parse(cfg.Permission), cfg.Allowlist)

	// Connect configured MCP servers (e.g. Scanner) and discover their evidence
	// tools so they show up during a hunt.
	evidence := connectMCP(cfg)

	// The session RunContext the hunt/intel tools write through. open_hunt sets
	// its active hunt at runtime.
	rc := tools.NewRunContext(brain.New(brainStore(cfg, cwd)))
	registry := tools.Toolbox(cwd, rc, evidence...)

	return &built{cfg: cfg, cwd: cwd, client: client, registry: registry, gate: gate}, nil
}

// brainStore returns an NTN-backed store when Notion DB IDs are configured,
// otherwise an in-memory store for local runs.
func brainStore(cfg config.Config, cwd string) brain.Notion {
	if brainConfigured(cfg) {
		return &brain.NTN{Dir: cwd, DBs: cfg.Notion}
	}
	return brain.NewMem()
}

// connectMCP dials every configured MCP server, discovers its tools, and returns
// them as vala tools. A server that fails to connect is logged and skipped —
// vala keeps running, just without that evidence source. The sessions live for
// the process lifetime.
func connectMCP(cfg config.Config) []tool.Tool {
	var evidence []tool.Tool
	for _, srv := range cfg.MCP {
		sess, err := mcp.Connect(context.Background(), mcp.ServerConfig{Name: srv.Name, URL: srv.URL, APIKey: srv.APIKey})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: mcp server %q unavailable: %v\n", srv.Name, err)
			continue
		}
		ts, _, err := tools.MCPToolsFrom(context.Background(), sess)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: mcp server %q: discover tools: %v\n", srv.Name, err)
			_ = sess.Close()
			continue
		}
		evidence = append(evidence, ts...)
	}
	return evidence
}
