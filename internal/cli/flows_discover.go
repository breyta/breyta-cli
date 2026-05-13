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
Use ` + "`breyta flows discover update <slug> --public=true --allow-public-access`" + ` to make your own
end-user flow appear there, or ` + "`--public=false`" + ` to remove it.
Add ` + "`--include-own`" + ` to list/search only when debugging whether your own public flow is indexed.

Checklist to make your flow show up in Discover:
1. Ask the flow author to approve making the flow accessible to all Breyta users
2. Add ` + "`:discover {:public true}`" + ` to the flow definition and push with ` + "`--allow-public-access`" + `, or run ` + "`breyta flows discover update <slug> --public=true --allow-public-access`" + ` after push
3. Tag the flow with ` + "`end-user`" + `
4. Release/promote it so there is an installable live version
5. Verify from another workspace with ` + "`breyta flows discover list`" + ` or ` + "`breyta flows discover search <query>`" + `
6. Open the Discover install dialog and run an installed target when install behavior matters;
   ` + "`/activate`" + ` only proves owner setup, not end-user installability

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
	var includeOwn bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Browse public installable flows for this workspace",
		Long: `Browse public end-user flows that can be installed from the current workspace.

This uses the same public discover/install catalog as the web app.
It excludes flows owned by the current workspace by default because those flows are not installable from itself.
Use ` + "`--include-own`" + ` only to debug whether your own public flow is indexed.
It is different from ` + "`breyta flows search`" + `, which only returns approved reusable examples.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows discover list requires API mode"))
			}
			payload := map[string]any{
				"limit":             limit,
				"from":              from,
				"includeDefinition": full,
			}
			if strings.TrimSpace(provider) != "" {
				payload["provider"] = strings.TrimSpace(provider)
			}
			if includeOwn {
				payload["includeOwn"] = true
			}
			if full {
				return doAPICommand(cmd, app, "flows.discover.list", payload)
			}
			return dispatchFlowAPICommandWithTransform(cmd, app, "flows.discover.list", payload, false, compactTemplateSearchEnvelope)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full indexed definition literal (definitionEdn)")
	cmd.Flags().BoolVar(&includeOwn, "include-own", false, "Include current workspace-owned public flows for debugging indexing")
	return cmd
}

func newFlowsDiscoverSearchCmd(app *App) *cobra.Command {
	var provider string
	var limit int
	var from int
	var full bool
	var includeOwn bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search public installable flows for this workspace",
		Long: `Search public end-user flows that can be installed from the current workspace.

This uses the public discover/install catalog, not the approved-example catalog behind ` + "`breyta flows search`" + `.
It excludes flows owned by the current workspace by default because those flows are not installable from itself.
Use ` + "`--include-own`" + ` only to debug whether your own public flow is indexed.`,
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
			if includeOwn {
				payload["includeOwn"] = true
			}
			if full {
				return doAPICommand(cmd, app, "flows.discover.search", payload)
			}
			return dispatchFlowAPICommandWithTransform(cmd, app, "flows.discover.search", payload, false, compactTemplateSearchEnvelope)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token (e.g. stripe, slack)")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full indexed definition literal (definitionEdn)")
	cmd.Flags().BoolVar(&includeOwn, "include-own", false, "Include current workspace-owned public flows for debugging indexing")
	return cmd
}

func newFlowsDiscoverUpdateCmd(app *App) *cobra.Command {
	var public bool
	var allowPublicAccess bool
	cmd := &cobra.Command{
		Use:   "update <flow-slug> --public <true|false>",
		Short: "Set public discover visibility for a flow",
		Long: `Set whether a flow is visible in public discover/install surfaces.

Requirements for ` + "`--public=true`" + `:
- the flow must be tagged ` + "`end-user`" + `
- the flow must be installable/released for discover surfaces to use it
- the flow author must explicitly approve making it accessible to all Breyta users
- pass ` + "`--allow-public-access`" + ` only after approval and installable-ready verification

Typical authoring flow:
1. ask the flow author whether this should be accessible to all Breyta users
2. add ` + "`:discover {:public true}`" + ` in the source file
3. add the ` + "`end-user`" + ` tag
4. ` + "`breyta flows push --file ... --allow-public-access`" + `
5. ` + "`breyta flows release <slug>`" + ` (or otherwise promote a live installable version)
6. ` + "`breyta flows discover list`" + ` from another workspace to verify visibility

Use ` + "`breyta flows show <slug>`" + ` after updating to confirm stored metadata includes
` + "`discover.public`" + `.

Only a privileged workspace member can change this metadata.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows discover update requires API mode"))
			}
			if public && !allowPublicAccess {
				return writeErr(cmd, publicAccessConfirmationError("setting Discover public visibility"))
			}
			return doAPICommand(cmd, app, "flows.discover.update", map[string]any{
				"flowSlug": args[0],
				"public":   public,
			})
		},
	}
	cmd.Flags().BoolVar(&public, "public", false, "Public discover visibility state")
	cmd.Flags().BoolVar(&allowPublicAccess, "allow-public-access", false, "Confirm explicit author approval to make this flow accessible to all Breyta users")
	_ = cmd.MarkFlagRequired("public")
	return cmd
}
