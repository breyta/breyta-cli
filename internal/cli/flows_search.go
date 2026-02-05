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
		Use:   "search [query]",
		Short: "Search for reusable flows (approved + workspace)",
		Long: strings.TrimSpace(`
Search flows via Elasticsearch-backed keyword search.

Default scope is "all" (approved reusable flows + your current workspace flows).
Use --scope workspace to search only your workspace, or --scope public to search only approved reusable flows.
`),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows search requires API mode"))
			}

			q := strings.TrimSpace(strings.Join(args, " "))
			apiArgs := map[string]any{
				"q":     q,
				"scope": strings.TrimSpace(scope),
				"from":  from,
				"size":  limit,
			}
			if strings.TrimSpace(provider) != "" {
				apiArgs["provider"] = strings.TrimSpace(provider)
			}
			if full {
				apiArgs["includeDefinition"] = true
			}

			return doAPICommandFn(cmd, app, "flows.search", apiArgs)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "all", "Search scope: all|workspace|public")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider/host (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max results (server caps at 100)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination")
	cmd.Flags().BoolVar(&full, "full", false, "Include flowLiteral definition in results")
	return cmd
}

