package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/brittonhayes/vala/runner"
	"github.com/spf13/cobra"
)

var (
	flagFixtures string
	flagOut      string
	flagBaseline string
)

var harnessCmd = &cobra.Command{
	Use:   "harness",
	Short: "Replay adversarial scenarios and score safety behavior",
	Long: `Harness replays scenario fixtures through vala's governance machine in a
deterministic recorded mode (no LLM) and scores each on approval compliance,
scope creep, evidence-backed claims, injection resistance, and schema validity.

It exits non-zero if any scenario fails or if a regression is detected against a
baseline report, making it suitable as a CI gate.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, _ := os.Getwd()
		fixtures, err := runner.LoadDir(flagFixtures)
		if err != nil {
			return fmt.Errorf("load fixtures: %w", err)
		}
		if len(fixtures) == 0 {
			return fmt.Errorf("no fixtures found under %s", flagFixtures)
		}

		report := runner.RunAll(cmd.Context(), fixtures, gitCommit(cwd))
		fmt.Print(report.Text())

		if flagOut != "" {
			if err := os.WriteFile(flagOut, report.JSON(), 0o644); err != nil {
				return fmt.Errorf("write report: %w", err)
			}
		}

		var regressions []string
		if flagBaseline != "" {
			if raw, err := os.ReadFile(flagBaseline); err == nil {
				var prev runner.Report
				if err := json.Unmarshal(raw, &prev); err == nil {
					regressions = report.Diff(prev)
				}
			}
		}
		for _, r := range regressions {
			fmt.Fprintln(os.Stderr, "REGRESSION:", r)
		}

		if report.Failed > 0 || len(regressions) > 0 {
			return fmt.Errorf("harness failed: %d scenario failure(s), %d regression(s)", report.Failed, len(regressions))
		}
		return nil
	},
}

func init() {
	harnessCmd.Flags().StringVar(&flagFixtures, "fixtures", "tests", "directory of scenario fixtures")
	harnessCmd.Flags().StringVar(&flagOut, "out", "", "write the JSON report to this path")
	harnessCmd.Flags().StringVar(&flagBaseline, "baseline", "", "previous JSON report to diff against for regressions")
}
