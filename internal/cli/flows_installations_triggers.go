package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newFlowsInstallationsTriggersCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triggers <installation-id>",
		Short: "List installation triggers (e.g. upload endpoints)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations triggers requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.installations.triggers.list", map[string]any{
				"profileId": args[0],
			})
		},
	}
	return cmd
}
