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
	var allowPublicAccess bool
	cmd := &cobra.Command{
		Use:   "update <flow-slug> --visible <true|false>",
		Short: "Set marketplace visibility for a flow",
		Long: `Set whether a flow is visible in marketplace surfaces.

Marketplace visibility can make the flow accessible to all Breyta users.
Use ` + "`--allow-public-access`" + ` only after the flow author explicitly approves
that distribution and the flow has been verified as installable-ready.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows marketplace update requires API mode"))
			}
			if visible && !allowPublicAccess {
				return writeErr(cmd, publicAccessConfirmationError("setting marketplace visibility"))
			}
			return doAPICommand(cmd, app, "flows.marketplace.update", map[string]any{
				"flowSlug": args[0],
				"visible":  visible,
			})
		},
	}
	cmd.Flags().BoolVar(&visible, "visible", false, "Marketplace visibility state")
	cmd.Flags().BoolVar(&allowPublicAccess, "allow-public-access", false, "Confirm explicit author approval to make this flow accessible to all Breyta users")
	_ = cmd.MarkFlagRequired("visible")
	return cmd
}
