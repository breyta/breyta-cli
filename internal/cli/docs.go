package cli

import "github.com/spf13/cobra"

func newDocsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Browse product docs",
		Long: "Browse product docs from the API.\n\n" +
			"Use `breyta docs find` to search pages, `breyta docs show` to print a page,\n" +
			"and `breyta docs sync` to download all docs for local grep/search.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newDocsFindCmd(app))
	cmd.AddCommand(newDocsShowCmd(app))
	cmd.AddCommand(newDocsSyncCmd(app))
	return cmd
}
