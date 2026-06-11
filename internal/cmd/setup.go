package cmd

import (
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"os"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run the guided onboarding (provider, brain, evidence sources)",
	Long: `Setup opens vala's onboarding wizard: connect a model provider, set up the
brain, and connect the security-tool MCP evidence sources the agent hunts in —
Scanner, Wiz, or any MCP server. It runs automatically on first launch when a
surface is unconfigured; run it anytime to add a source or change a choice.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, cwd, err := resolveConfig()
		if err != nil {
			return err
		}
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return firstRunNotice(cfg, cwd)
		}
		// force: show every step even when already configured so the operator can
		// add an evidence source to a set-up project.
		_, err = maybeRunSetup(cmd.Context(), cfg, cwd, true)
		return err
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
