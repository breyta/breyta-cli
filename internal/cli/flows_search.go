package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsSearchCmd(app *App) *cobra.Command {
	var catalogScope string
	var provider string
	var limit int
	var from int
	var full bool

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search approved example flows to copy from",
		Long: strings.TrimSpace(`
Search across approved example flows to find reusable patterns to copy from.

By default the search is global (across all workspaces). Use --catalog-scope workspace to
restrict results to the current workspace.

NOTE: Only flows explicitly approved by Breyta for reuse are indexed/searchable here.
These are example definitions to inspect and copy from, not the same thing as public
installable flows in the discover surface. To browse public installables, use
` + "`breyta flows discover list`" + ` or ` + "`breyta flows discover search <query>`" + `.

Omit the query to browse recent approved flows (optionally filtered by --provider
and/or --catalog-scope).
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
			if effectiveScope == "" {
				effectiveScope = "all"
			}
			if effectiveScope != "all" && effectiveScope != "workspace" {
				return writeErr(cmd, errors.New("--catalog-scope must be 'all' or 'workspace'"))
			}
			workspaceID := strings.TrimSpace(app.WorkspaceID)
			if effectiveScope == "workspace" && workspaceID == "" {
				return writeErr(cmd, errors.New("workspace-scoped catalog search requires --workspace or BREYTA_WORKSPACE"))
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

			if workspaceID == "" && effectiveScope == "all" {
				return doGlobalAPICommand(cmd, app, "flows.search", payload)
			}
			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "flows.search", payload)
			}
			return doAPICommand(cmd, app, "flows.search", payload)
		},
	}

	cmd.Flags().StringVar(&catalogScope, "catalog-scope", "all", "Catalog scope: all|workspace")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full indexed definition literal (definitionEdn)")
	return cmd
}
