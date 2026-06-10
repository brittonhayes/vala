package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/session"
	"github.com/spf13/cobra"
)

var flagYes bool

var runCmd = &cobra.Command{
	Use:   "run [prompt...]",
	Short: "Run a single non-interactive task",
	Long: `Run executes one task and exits. Useful for scripting and automation.

By default, non-read-only tools are denied in this mode (there is no operator
to prompt). Pass --yes to auto-approve every tool call for an unattended run.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Non-interactive: warn on stderr when the brain is ephemeral and keep
		// going (automation must not block) unless --require-brain is set.
		if cfg, cwd, err := resolveConfig(); err != nil {
			return err
		} else if err := firstRunNotice(cmd.Context(), cfg, cwd, false); err != nil {
			return err
		}
		built, err := build()
		if err != nil {
			return err
		}
		// Non-interactive: no prompter. --yes forces allow; otherwise honor the
		// configured mode but fail closed (deny) when it would need to ask.
		if flagYes {
			built.gate.Mode = permission.ModeAllow
		} else if built.gate.Mode == permission.ModeAsk {
			built.gate.Mode = permission.ModeDeny
			fmt.Fprintln(os.Stderr, "note: no TTY to prompt; non-read-only tools will be denied (use --yes to allow)")
		}

		sess, _ := session.New(session.DefaultDir())
		ag := agent.New(built.client, built.registry, built.gate, built.cwd, built.cfg.MaxSteps,
			sessionContext(cmd.Context(), built.cwd, built.rc.Brain))

		prompt := strings.Join(args, " ")
		sess.Add(session.Entry{Kind: session.KindUser, Content: prompt})

		_, runErr := ag.Run(cmd.Context(), nil, prompt, printEvents(sess))
		return runErr
	},
}

func init() {
	runCmd.Flags().BoolVar(&flagYes, "yes", false, "auto-approve all tool calls (unattended)")
}

// printEvents renders the loop to stdout/stderr for one-shot runs and records
// the transcript.
func printEvents(sess *session.Session) agent.Events {
	return agent.Events{
		OnAssistantText: func(text string) {
			fmt.Println(strings.TrimSpace(text))
			sess.Add(session.Entry{Kind: session.KindAssistant, Content: text})
		},
		OnToolCall: func(name, summary string) {
			fmt.Fprintf(os.Stderr, "⚙ %s %s\n", name, summary)
			sess.Add(session.Entry{Kind: session.KindToolCall, Tool: name, Content: summary})
		},
		OnToolResult: func(name, content string, isErr bool) {
			sess.Add(session.Entry{Kind: session.KindToolResult, Tool: name, Content: content, IsError: isErr})
		},
		OnPermissionDenied: func(name, summary string) {
			fmt.Fprintf(os.Stderr, "✗ denied: %s\n", name)
		},
	}
}
