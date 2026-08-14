package cli

import "github.com/spf13/cobra"

func configureVisibility(_ *cobra.Command, app *App) {
	if app != nil {
		app.visibilityConfigured = true
	}
}

func hideDevOnlyCommandTree(cmd *cobra.Command, _ *App) *cobra.Command {
	if cmd != nil {
		cmd.Hidden = true
	}
	return cmd
}
