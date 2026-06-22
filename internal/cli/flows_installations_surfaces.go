package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsInstallationsSurfacesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "surfaces <installation-id>",
		Short: "List installer-facing email addresses and endpoints",
		Long: strings.TrimSpace(`
List the durable surfaces for an installed app.

This includes generated inbound email addresses, HTTP endpoints, webhook paths,
and MCP transport metadata when the installed flow exposes them.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations surfaces requires API mode"))
			}
			installationID := strings.TrimSpace(args[0])
			if installationID == "" {
				return writeErr(cmd, errors.New("installation id is required"))
			}
			return doAPICommand(cmd, app, "flows.installations.surfaces.list", map[string]any{
				"profileId": installationID,
			})
		},
	}
	return cmd
}
