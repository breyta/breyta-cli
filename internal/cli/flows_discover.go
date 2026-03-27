package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsDiscoverCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Manage flow public discover metadata",
		Long: `Public discover is the catalog of installable end-user flows shown in the web app discover surface.

Use ` + "`breyta flows discover list`" + ` or ` + "`breyta flows discover search <query>`" + ` to browse installables.
Use ` + "`breyta flows discover update <slug> --public=true|false`" + ` to control whether your own end-user flow
appears there.

Checklist to make your flow show up in Discover:
1. Add ` + "`:discover {:public true}`" + ` to the flow definition (or run ` + "`breyta flows discover update <slug> --public=true`" + ` after push)
2. Tag the flow with ` + "`end-user`" + `
3. Push the flow
4. Release/promote it so there is an installable live version
5. Verify from another workspace with ` + "`breyta flows discover list`" + ` or ` + "`breyta flows discover search <query>`" + `

This is different from ` + "`breyta flows search`" + `, which only searches approved Breyta-curated examples to
copy from. Approved examples are not the same thing as public installables.`,
	}
	cmd.AddCommand(newFlowsDiscoverListCmd(app))
	cmd.AddCommand(newFlowsDiscoverSearchCmd(app))
	cmd.AddCommand(newFlowsDiscoverUpdateCmd(app))
	return cmd
}

func newFlowsDiscoverListCmd(app *App) *cobra.Command {
	var provider string
	var limit int
	var from int
	var full bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Browse public installable flows for this workspace",
		Long: `Browse public end-user flows that can be installed from the current workspace.

This uses the same public discover/install catalog as the web app.
It is different from ` + "`breyta flows search`" + `, which only returns approved reusable examples.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows discover list requires API mode"))
			}
			payload := map[string]any{
				"limit":              limit,
				"from":               from,
				"includeDefinition":  full,
			}
			if strings.TrimSpace(provider) != "" {
				payload["provider"] = strings.TrimSpace(provider)
			}
			return doAPICommand(cmd, app, "flows.discover.list", payload)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full indexed definition literal (definitionEdn)")
	return cmd
}

func newFlowsDiscoverSearchCmd(app *App) *cobra.Command {
	var provider string
	var limit int
	var from int
	var full bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search public installable flows for this workspace",
		Long: `Search public end-user flows that can be installed from the current workspace.

This uses the public discover/install catalog, not the approved-example catalog behind ` + "`breyta flows search`" + `.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows discover search requires API mode"))
			}
			payload := map[string]any{
				"query":             strings.TrimSpace(args[0]),
				"limit":             limit,
				"from":              from,
				"includeDefinition": full,
			}
			if strings.TrimSpace(provider) != "" {
				payload["provider"] = strings.TrimSpace(provider)
			}
			return doAPICommand(cmd, app, "flows.discover.search", payload)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full indexed definition literal (definitionEdn)")
	return cmd
}

func newFlowsDiscoverUpdateCmd(app *App) *cobra.Command {
	var public bool
	cmd := &cobra.Command{
		Use:   "update <flow-slug> --public <true|false>",
		Short: "Set public discover visibility for a flow",
		Long: `Set whether a flow is visible in public discover/install surfaces.

Requirements for ` + "`--public=true`" + `:
- the flow must be tagged ` + "`end-user`" + `
- the flow must be installable/released for discover surfaces to use it

Typical authoring flow:
1. add ` + "`:discover {:public true}`" + ` in the source file
2. add the ` + "`end-user`" + ` tag
3. ` + "`breyta flows push --file ...`" + `
4. ` + "`breyta flows release <slug>`" + ` (or otherwise promote a live installable version)
5. ` + "`breyta flows discover list`" + ` from another workspace to verify visibility

Use ` + "`breyta flows show <slug> --pretty`" + ` after updating to confirm stored metadata includes
` + "`discover.public`" + `.

Only a privileged workspace member can change this metadata.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows discover update requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.discover.update", map[string]any{
				"flowSlug": args[0],
				"public":   public,
			})
		},
	}
	cmd.Flags().BoolVar(&public, "public", false, "Public discover visibility state")
	_ = cmd.MarkFlagRequired("public")
	return cmd
}
