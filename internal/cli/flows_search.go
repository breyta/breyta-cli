package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsSearchCmd(app *App) *cobra.Command {
	var (
		scope    string
		provider string
		limit    int
		from     int
		full     bool
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search approved reusable flows (API)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Flows search requires API mode (--api or BREYTA_API_URL).")
			}
			q := strings.TrimSpace(strings.Join(args, " "))
			if q == "" {
				return writeErr(cmd, errors.New("invalid query: must be non-empty"))
			}

			payload := map[string]any{"q": q}
			if strings.TrimSpace(scope) != "" && strings.TrimSpace(scope) != "all" {
				payload["scope"] = strings.TrimSpace(scope)
			}
			if strings.TrimSpace(provider) != "" {
				payload["provider"] = strings.TrimSpace(provider)
			}
			if limit > 0 {
				payload["size"] = limit
			}
			if from > 0 {
				payload["from"] = from
			}
			if full {
				payload["includeDefinition"] = true
			}

			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "flows.search", payload)
			}
			return doAPICommand(cmd, app, "flows.search", payload)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "all", "Scope: all|workspace|public")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider/host (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max hits to return")
	cmd.Flags().IntVar(&from, "from", 0, "Offset (pagination)")
	cmd.Flags().BoolVar(&full, "full", false, "Include definition EDN in results")

	return cmd
}
