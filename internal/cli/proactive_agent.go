package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

const commandProactiveAgentInitiativePark = "proactive_agent.initiative.park"

func newAgentInitiativeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "initiative",
		Short: "Manage the current proactive initiative",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAgentInitiativeParkCmd(app))
	return cmd
}

func newAgentInitiativeParkCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "park",
		Short: "Park the current proactive initiative",
		Long: strings.TrimSpace(`
Set aside the signed-in user's current proactive initiative in the selected
workspace. Drafts and completed work are preserved, while outstanding prompts
are suppressed until the user explicitly raises that theme again.

Calling this when no initiative is active succeeds without changing anything.
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doAPICommand(cmd, app, commandProactiveAgentInitiativePark, map[string]any{})
		},
	}
}
