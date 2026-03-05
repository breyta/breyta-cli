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
	var visible bool
	cmd := &cobra.Command{
		Use:   "update <flow-slug> --visible <true|false>",
		Short: "Set marketplace visibility for a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows marketplace update requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.marketplace.update", map[string]any{
				"flowSlug": args[0],
				"visible":  visible,
			})
		},
	}
	cmd.Flags().BoolVar(&visible, "visible", false, "Marketplace visibility state")
	_ = cmd.MarkFlagRequired("visible")
	return cmd
}
