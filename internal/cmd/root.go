// Package cmd wires the vala CLI together with cobra.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
		// First-run onboarding: in a TTY, launch the curated wizard whenever a
		// surface (provider, brain, or evidence) is unconfigured so the operator is
		// guided to a working tool. Non-interactive sessions fall back to a stderr
		// summary so automation is never blocked.
		if term.IsTerminal(int(os.Stdin.Fd())) {
			proceed, err := maybeRunSetup(cmd.Context(), cfg, cwd, false)
			if err != nil {
				return err
			}
			if !proceed {
				return nil
			}
		} else if err := firstRunNotice(cfg, cwd); err != nil {
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
		ag := agent.New(built.client, built.registry, built.gate, built.cwd, built.cfg.MaxSteps,
			sessionContext(cmd.Context(), built.cwd, built.rc.Brain))
		repl := ui.New(ag, built.gate, sess, modelLabel(built.client), contextWindow(built.client, built.cfg), built.cfg.AutoCompactThreshold)
		repl.Evidence = built.evidence
		// Wire /connect: rebuild a provider from the latest stored credentials so
		// the operator can connect or switch providers without leaving the session.
		repl.Connect = func(provider, model string) (llm.Provider, error) {
			return reconnect(built.cfg, provider, model)
		}
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
	client   llm.Provider
	registry *tool.Registry
	gate     *permission.Gate
	rc       *tools.RunContext
	// evidence reports how each configured MCP source connected, so the session
	// can show the operator what is (and is not) available to hunt in.
	evidence []mcp.EvidenceStatus
	// connectErr is set (and client left nil) when no provider credential is
	// available yet, so the interactive REPL can launch and offer /connect while
	// unattended commands can fail closed.
	connectErr error
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
	// A missing provider credential is not fatal: the interactive REPL launches
	// anyway so the operator can run /connect. Any other construction error
	// (malformed config, unknown provider) still fails.
	client, err := llm.New(cfg)
	var connectErr error
	if err != nil {
		if !errors.Is(err, llm.ErrNoCredentials) {
			return nil, err
		}
		connectErr = err
		client = nil
	}
	gate := permission.New(permission.Parse(cfg.Permission), cfg.Allowlist)

	// Connect configured MCP servers (e.g. Scanner) and discover their evidence
	// tools so they show up during a hunt.
	evidence, evidenceStatus := connectMCP(cfg)

	// The session RunContext the hunt/intel tools write through. open_hunt sets
	// its active hunt at runtime; Author stamps shared memories with this operator.
	rc := tools.NewRunContext(brain.New(brainStore(cfg, cwd)))
	rc.Author = resolveAuthor()
	registry := tools.Toolbox(cwd, rc, evidence...)

	return &built{cfg: cfg, cwd: cwd, client: client, registry: registry, gate: gate, rc: rc, evidence: evidenceStatus, connectErr: connectErr}, nil
}

// modelLabel renders the active provider and model for the session banner,
// e.g. "anthropic · claude-opus-4-8", or a connect hint when no provider is
// wired up yet.
func modelLabel(p llm.Provider) string {
	if p == nil {
		return "not connected · /connect to choose a provider"
	}
	return p.Provider() + " · " + p.Model()
}

// contextWindow returns the token budget that drives auto-compaction. It prefers
// the provider's known window for the active model (from the embedded catalog)
// and falls back to the configured value for models vala does not recognize (or
// when no provider is connected yet).
func contextWindow(p llm.Provider, cfg config.Config) int64 {
	if p != nil {
		if w := p.ContextWindow(); w > 0 {
			return w
		}
	}
	return cfg.ContextWindow
}

// reconnect builds a fresh provider for an in-session /connect switch. It reads
// the latest stored credentials, so a key saved moments earlier is picked up.
func reconnect(base config.Config, provider, model string) (llm.Provider, error) {
	c := base
	c.Provider = provider
	c.Model = model
	return llm.New(c)
}

// brainStore selects the brain backend: an NTN-backed store when Notion DB IDs
// are configured, a durable file-backed store when a brain file is set, and an
// ephemeral in-memory store otherwise. A file that fails to open degrades to
// in-memory with a warning rather than blocking the session.
func brainStore(cfg config.Config, cwd string) brain.Notion {
	switch {
	case brainConfigured(cfg):
		return &brain.NTN{Dir: cwd, DBs: cfg.Notion}
	case cfg.BrainFile != "":
		path := cfg.BrainFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		f, err := brain.NewFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ could not open brain file %s: %v — using ephemeral memory\n", path, err)
			return brain.NewMem()
		}
		return f
	default:
		return brain.NewMem()
	}
}

// connectMCP dials every configured MCP server, discovers its tools, and returns
// them as vala tools alongside a per-server status report. A server that fails
// to connect is recorded in the status (not fatal) so the session can show the
// operator what is and isn't connected — vala keeps running, just without that
// evidence source. The sessions live for the process lifetime.
func connectMCP(cfg config.Config) ([]tool.Tool, []mcp.EvidenceStatus) {
	var evidence []tool.Tool
	var report []mcp.EvidenceStatus
	for _, srv := range cfg.MCP {
		ts, status := tools.ConnectEvidence(context.Background(), serverConfig(srv))
		evidence = append(evidence, ts...)
		report = append(report, status)
	}
	return evidence, report
}

// serverConfig maps a persisted MCP server entry to the mcp package's transport
// config, carrying the secrets resolved from the environment at load time.
func serverConfig(srv config.MCPServer) mcp.ServerConfig {
	return mcp.ServerConfig{
		Name:      srv.Name,
		Transport: srv.Transport,
		URL:       srv.URL,
		APIKey:    srv.APIKey,
		OAuth:     srv.OAuth,
		Command:   srv.Command,
		Args:      srv.Args,
		Env:       srv.Env,
	}
}
