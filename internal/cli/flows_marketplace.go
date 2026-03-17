package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newFlowsMarketplaceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Legacy alias for discover visibility commands",
	}
	cmd.AddCommand(newFlowsVisibilityUpdateCmd(app, "marketplace", "flows.discover.update"))
	return hideDevOnlyCommandTree(cmd, app)
}

func newFlowsDiscoverCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Manage discover visibility for installable flows",
	}
	cmd.AddCommand(newFlowsVisibilityUpdateCmd(app, "discover", "flows.discover.update"))
	return cmd
}

func newFlowsVisibilityUpdateCmd(app *App, surface string, apiCommand string) *cobra.Command {
	var visible bool
	cmd := &cobra.Command{
		Use:   "update <flow-slug> --visible <true|false>",
		Short: "Set discover visibility for a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows "+surface+" update requires API mode"))
			}
			return doAPICommand(cmd, app, apiCommand, map[string]any{
				"flowSlug": args[0],
				"visible":  visible,
			})
		},
	}
	cmd.Flags().BoolVar(&visible, "visible", false, "Discover visibility state")
	_ = cmd.MarkFlagRequired("visible")
	return cmd
}
