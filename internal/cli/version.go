package cli

import (
	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeData(cmd, app, nil, map[string]any{
				"version":    buildinfo.DisplayVersion(),
				"rawVersion": buildinfo.Version,
				"commit":     buildinfo.Commit,
				"date":       buildinfo.Date,
			})
		},
	}
}
