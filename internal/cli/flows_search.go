package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsSearchCmd(app *App) *cobra.Command {
	var scope string
	var provider string
	var limit int
	var from int
	var full bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search approved flows for reuse patterns",
		Long: strings.TrimSpace(`
Search across approved flows to find reusable examples.

By default the search is global (across all workspaces). Use --scope=workspace to
restrict results to the current workspace.

NOTE: Only flows explicitly approved for reuse are indexed/searchable.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows search requires API mode"))
			}

			query := strings.TrimSpace(args[0])
			if query == "" {
				return writeErr(cmd, errors.New("query is required"))
			}

			scope = strings.TrimSpace(strings.ToLower(scope))
			if scope == "" {
				scope = "all"
			}
			if scope != "all" && scope != "workspace" {
				return writeErr(cmd, errors.New("--scope must be 'all' or 'workspace'"))
			}

			payload := map[string]any{
				"query":             query,
				"scope":             scope,
				"limit":             limit,
				"from":              from,
				"includeDefinition": full,
			}
			if strings.TrimSpace(provider) != "" {
				payload["provider"] = strings.TrimSpace(provider)
			}

			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "flows.search", payload)
			}
			return doAPICommand(cmd, app, "flows.search", payload)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "all", "Search scope: all|workspace")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full indexed definition literal (definitionEdn)")
	return cmd
}
