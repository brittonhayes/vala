// Package cmd wires the vala CLI together with cobra.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/config"
	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/mcp"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/policy"
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
	Long: `vala is a single agentic security harness: one interactive session with a
toolbox the agent composes to hunt threats, record and link threat
intelligence, author and validate detections, and work alerts — documenting it
all in a Notion-backed brain.

There is one surface and one set of tools. Workflows are not separate commands;
they are things you ask the agent to do, and it reaches for the right
primitives: open_hunt to investigate a question, record_intel/link_artifacts to
build the intel graph, the detection-authoring tools to write Sigma rules, and
open_case to work an alert through the governed response loop.

Run with no arguments to start an interactive session, or use "vala run" for a
single non-interactive task.`,
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
	rootCmd.AddCommand(runCmd, harnessCmd, versionCmd)
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

	pol, err := policy.Load(cwd)
	if err != nil {
		return nil, fmt.Errorf("load policy: %w", err)
	}

	// Connect configured MCP servers (e.g. Scanner) and discover their evidence
	// tools, classifying the read-only ones so they show up during a hunt.
	evidence := connectMCP(cfg, pol)

	// The session RunContext the hunt/intel tools write through. open_hunt sets
	// its active hunt at runtime; the ledger is unused on this single-phase path
	// (governed actions run inside their own context within open_case).
	rc := tools.NewRunContext(cfg.Env, "", brain.New(brainStore(cfg, cwd)), governance.NewLedger(), pol)
	cr := &caseRunner{cfg: cfg, cwd: cwd, client: client, gate: gate, policy: pol, evidence: evidence}
	registry := tools.Toolbox(cwd, rc, cfg.SlackWebhook, cr, evidence...)

	return &built{cfg: cfg, cwd: cwd, client: client, registry: registry, gate: gate}, nil
}

// connectMCP dials every configured MCP server, discovers its tools, and returns
// them as vala tools. Read-only tools are promoted into the policy's read class
// so the agent can use them during investigation and hunts. A server that fails
// to connect is logged and skipped — vala keeps running, just without that
// evidence source. The sessions live for the process lifetime.
func connectMCP(cfg config.Config, pol *policy.Set) []tool.Tool {
	var evidence []tool.Tool
	for _, srv := range cfg.MCP {
		sess, err := mcp.Connect(context.Background(), mcp.ServerConfig{Name: srv.Name, URL: srv.URL, APIKey: srv.APIKey})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: mcp server %q unavailable: %v\n", srv.Name, err)
			continue
		}
		ts, readOnly, err := tools.MCPToolsFrom(context.Background(), sess)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: mcp server %q: discover tools: %v\n", srv.Name, err)
			_ = sess.Close()
			continue
		}
		pol.ClassifyRead(readOnly...)
		evidence = append(evidence, ts...)
	}
	return evidence
}
