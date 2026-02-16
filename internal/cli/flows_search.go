package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsSearchCmd(app *App) *cobra.Command {
	var catalogScope string
	var legacyScope string
	var provider string
	var limit int
	var from int
	var full bool

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search/browse approved flows for reuse patterns",
		Long: strings.TrimSpace(`
Search across approved flows to find reusable examples.

By default the search is global (across all workspaces). Use --scope=workspace to
restrict results to the current workspace.

NOTE: Only flows explicitly approved for reuse are indexed/searchable.

Omit the query to browse recent approved flows (optionally filtered by --provider
and/or --scope).
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows search requires API mode"))
			}

			query := ""
			if len(args) > 0 {
				query = strings.TrimSpace(args[0])
			}

			effectiveScope := strings.TrimSpace(strings.ToLower(catalogScope))
			legacyScope = strings.TrimSpace(strings.ToLower(legacyScope))
			if effectiveScope == "" && legacyScope != "" {
				effectiveScope = legacyScope
			}
			if effectiveScope == "" {
				effectiveScope = "all"
			}
			if effectiveScope != "all" && effectiveScope != "workspace" {
				return writeErr(cmd, errors.New("--catalog-scope must be 'all' or 'workspace'"))
			}

			payload := map[string]any{
				"scope":             effectiveScope,
				"limit":             limit,
				"from":              from,
				"includeDefinition": full,
			}
			if query != "" {
				payload["query"] = query
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

	cmd.Flags().StringVar(&catalogScope, "catalog-scope", "all", "Catalog scope: all|workspace")
	cmd.Flags().StringVar(&legacyScope, "scope", "", "Deprecated alias for --catalog-scope")
	_ = cmd.Flags().MarkHidden("scope")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full indexed definition literal (definitionEdn)")
	return cmd
}
