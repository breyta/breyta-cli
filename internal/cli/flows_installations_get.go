package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newFlowsInstallationsGetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <profile-id>",
		Aliases: []string{"show"},
		Short:   "Get installation details",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations get requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.installations.get", map[string]any{
				"profileId": args[0],
			})
		},
	}
	return cmd
}

