package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newFlowsMarketplaceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Manage flow marketplace metadata",
	}
	cmd.AddCommand(newFlowsMarketplaceUpdateCmd(app))
	return cmd
}

func newFlowsMarketplaceUpdateCmd(app *App) *cobra.Command {
	var visible string
	cmd := &cobra.Command{
		Use:   "update <flow-slug> --visible <true|false>",
		Short: "Set marketplace visibility for a flow",
		Long: `Set the lower-level marketplace visibility flag for a flow.

To publish or delist a flow across all public surfaces at once, use
` + "`breyta flows public publish <flow-slug>`" + ` or
` + "`breyta flows public delist <flow-slug>`" + `.`,
		Example: "Publish queues acceptance; use --wait or flows public status to observe it. Pushed visibility metadata cannot bypass review.",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows marketplace update requires API mode"))
			}
			flowSlug, visibleValue, err := parseFlowSlugAndCLITrueFalseFlag("visible", visible, args, cmd.Flags().Changed("visible"))
			if err != nil {
				return writeErr(cmd, err)
			}
			return doAPICommand(cmd, app, "flows.marketplace.update", map[string]any{
				"flowSlug": flowSlug,
				"visible":  visibleValue,
			})
		},
	}
	cmd.Flags().StringVar(&visible, "visible", "", "Marketplace visibility state (true|false)")
	if f := cmd.Flags().Lookup("visible"); f != nil {
		f.NoOptDefVal = cliBareTrueValue
	}
	_ = cmd.MarkFlagRequired("visible")
	return cmd
}
