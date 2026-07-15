package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

const (
	commandProactiveAgentInitiativeContinue = "proactive_agent.initiative.continue"
	commandProactiveAgentInitiativePark     = "proactive_agent.initiative.park"
)

func newAgentInitiativeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "initiative",
		Short: "Manage the current proactive initiative",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAgentInitiativeContinueCmd(app))
	cmd.AddCommand(newAgentInitiativeParkCmd(app))
	return cmd
}

func newAgentInitiativeContinueCmd(app *App) *cobra.Command {
	var summary string
	var sourceEventID string

	cmd := &cobra.Command{
		Use:   "continue --summary <work-summary>",
		Short: "Continue a user-directed proactive priority",
		Long: strings.TrimSpace(`
Record a concise work priority for the signed-in user's background proactive
agent in the selected workspace. The priority takes precedence over generic
checks, but does not approve publishing, spend, account connections, or other
gated actions.
`),
		Example: strings.TrimSpace(`
breyta proactive-agent initiative continue --summary "Continue the Meta ads work and prepare the next creative test."
breyta proactive-agent initiative continue --summary "Continue the Meta ads work." --source-event-id "chat-event-42"
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.APIKeyExplicit || looksLikeServiceAccountAPIKey(app.Token) {
				return writeErr(cmd, errors.New("proactive-agent initiative continue requires signed-in user authentication; unset BREYTA_API_KEY or omit --api-key, then run `breyta auth login`"))
			}

			summary = strings.TrimSpace(summary)
			if summary == "" {
				return writeErr(cmd, errors.New("missing --summary"))
			}

			payload := map[string]any{"summary": summary}
			if sourceEventID = strings.TrimSpace(sourceEventID); sourceEventID != "" {
				payload["sourceEventId"] = sourceEventID
			}
			return doAPICommand(cmd, app, commandProactiveAgentInitiativeContinue, payload)
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "Concise work priority for the proactive agent")
	cmd.Flags().StringVar(&sourceEventID, "source-event-id", "", "Optional source chat or event id for idempotency")
	_ = cmd.MarkFlagRequired("summary")
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
			if app.APIKeyExplicit || looksLikeServiceAccountAPIKey(app.Token) {
				return writeErr(cmd, errors.New("proactive-agent initiative park requires signed-in user authentication; unset BREYTA_API_KEY or omit --api-key, then run `breyta auth login`"))
			}
			return doAPICommand(cmd, app, commandProactiveAgentInitiativePark, map[string]any{})
		},
	}
}
