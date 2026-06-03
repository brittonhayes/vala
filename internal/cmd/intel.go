package cmd

import (
	"fmt"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/spf13/cobra"
)

var intelCmd = &cobra.Command{
	Use:   "intel",
	Short: "Record and link threat intelligence in the brain",
	Long: `Intel manages threat intelligence as first-class brain artifacts: indicators
(IOCs), TTPs (MITRE techniques), actors, and narrative writeups. Recorded intel
can be linked to the hunts, alerts, and detections it relates to.

Notion database IDs in config enable real Notion writes; without them intel is
recorded in local mode and printed.`,
}

var (
	flagIntelKind       string
	flagIntelValue      string
	flagIntelMITRE      string
	flagIntelConfidence string
	flagIntelSource     string
	flagIntelDesc       string
)

var intelAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Record a piece of threat intelligence",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagIntelValue == "" {
			return fmt.Errorf("--value is required")
		}
		if flagIntelKind == "" {
			flagIntelKind = brain.IntelIndicator
		}
		cfg, cwd, err := resolveConfig()
		if err != nil {
			return err
		}
		bc := brain.New(brainStore(cfg, cwd))
		id, err := bc.RecordIntel(cmd.Context(), brain.Intel{
			Kind:        flagIntelKind,
			Value:       flagIntelValue,
			MITRE:       flagIntelMITRE,
			Confidence:  flagIntelConfidence,
			Source:      flagIntelSource,
			Description: flagIntelDesc,
		})
		if err != nil {
			return err
		}
		fmt.Printf("recorded intel %s (%s: %s)\n", id, flagIntelKind, flagIntelValue)
		return nil
	},
}

var flagIntelTo []string
var flagIntelRelation string

var intelLinkCmd = &cobra.Command{
	Use:   "link <row-id>",
	Short: "Link a brain row to other artifacts (intel, hunts, alerts, detections)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(flagIntelTo) == 0 {
			return fmt.Errorf("--to is required (one or more target row IDs)")
		}
		if flagIntelRelation == "" {
			return fmt.Errorf("--relation is required (e.g. hunts, alerts, detections, intel)")
		}
		cfg, cwd, err := resolveConfig()
		if err != nil {
			return err
		}
		bc := brain.New(brainStore(cfg, cwd))
		if err := bc.Link(cmd.Context(), args[0], flagIntelRelation, flagIntelTo...); err != nil {
			return err
		}
		fmt.Printf("linked %s %s -> %v\n", args[0], flagIntelRelation, flagIntelTo)
		return nil
	},
}

func init() {
	intelAddCmd.Flags().StringVar(&flagIntelKind, "kind", "", "intel kind: indicator | ttp | actor | narrative")
	intelAddCmd.Flags().StringVar(&flagIntelValue, "value", "", "the IOC, technique ID, actor name, or title (required)")
	intelAddCmd.Flags().StringVar(&flagIntelMITRE, "mitre", "", "related MITRE ATT&CK technique, e.g. attack.t1562.001")
	intelAddCmd.Flags().StringVar(&flagIntelConfidence, "confidence", "", "confidence: confirmed | probable | hypothesis")
	intelAddCmd.Flags().StringVar(&flagIntelSource, "source", "", "where the intel came from")
	intelAddCmd.Flags().StringVar(&flagIntelDesc, "description", "", "a short description")

	intelLinkCmd.Flags().StringSliceVar(&flagIntelTo, "to", nil, "target row IDs to link to")
	intelLinkCmd.Flags().StringVar(&flagIntelRelation, "relation", "", "relation property to set")

	intelCmd.AddCommand(intelAddCmd, intelLinkCmd)
}
