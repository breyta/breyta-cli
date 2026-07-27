package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsDiscoverCmd(app *App) *cobra.Command {
	return newDiscoverCmdWithPath(app, "breyta flows discover")
}

func newDiscoverCmd(app *App) *cobra.Command {
	return newDiscoverCmdWithPath(app, "breyta discover")
}

func newDiscoverCmdWithPath(app *App, commandPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Manage flow public discover metadata",
		Long: `Public discover is the catalog of installable public flows shown in the web app discover surface.

Use ` + "`" + commandPath + " list`" + ` or ` + "`" + commandPath + " search <query>`" + ` to browse installables.
Use ` + "`" + commandPath + " show <workspace-id>/<flow-slug>`" + ` to inspect one public app's full listing
(publish copy, pricing, connections, versions); it works across workspaces, unlike ` + "`breyta flows show`" + `.
Use ` + "`breyta flows public publish <slug>`" + ` or ` + "`breyta flows public delist <slug>`" + ` to change all public surfaces together.
Use ` + "`" + commandPath + " update <slug> --public=true|false`" + ` only when you need to control the Discover flag directly.
Add ` + "`--include-own`" + ` to list/search only when debugging whether your own public flow is indexed.

Checklist to make your flow show up in Discover:
1. Add ` + "`:discover {:public true}`" + ` to the flow definition (or run ` + "`" + commandPath + " update <slug> --public=true`" + ` after push)
2. Push the flow
3. Release/promote it so there is an installable live version
4. Verify from another workspace with ` + "`" + commandPath + " list`" + ` or ` + "`" + commandPath + " search <query>`" + `
5. Open the Discover install dialog and run an installed target when install behavior matters;
   ` + "`/activate`" + ` only proves owner setup, not end-user installability

This is different from ` + "`breyta flows search`" + `, which only searches approved Breyta-curated examples to
copy from. Approved examples are not the same thing as public installables.`,
	}
	cmd.AddCommand(newFlowsDiscoverListCmd(app))
	cmd.AddCommand(newFlowsDiscoverSearchCmd(app))
	cmd.AddCommand(newFlowsDiscoverShowCmdWithPath(app, commandPath))
	cmd.AddCommand(newFlowsDiscoverUpdateCmd(app, commandPath))
	return cmd
}

// splitDiscoverAppRef splits "<workspace-id>/<flow-slug>", "<workspace-id>:<flow-slug>",
// or the catalog id "<workspace-id>__<flow-slug>" into its parts. Slash wins
// because it is the one separator forbidden inside workspace ids, so
// path-style refs stay unambiguous even when the id contains ":" or "__".
func splitDiscoverAppRef(ref string) (string, string, bool) {
	ref = strings.TrimSpace(ref)
	for _, sep := range []string{"/", ":", "__"} {
		if idx := strings.Index(ref, sep); idx > 0 {
			ws := strings.TrimSpace(ref[:idx])
			slug := strings.TrimSpace(ref[idx+len(sep):])
			if ws != "" && slug != "" {
				return ws, slug, true
			}
		}
	}
	return "", "", false
}

func newFlowsDiscoverShowCmd(app *App) *cobra.Command {
	return newFlowsDiscoverShowCmdWithPath(app, "breyta flows discover")
}

func newFlowsDiscoverShowCmdWithPath(app *App, commandPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <workspace-id>/<flow-slug>",
		Short: "Show the full public listing for one installable public app",
		Long: `Show the full public Discover listing for one installable public app:
publish copy, pricing, connections, versions, install counts, and ratings.

This works across workspaces because it reads only public catalog data.
Use it to evaluate a hit from ` + "`" + commandPath + " search`" + ` in depth;
` + "`breyta flows show`" + ` cannot inspect flows in workspaces you are not a
member of and fails with access denied.

The app ref accepts the id shown in Discover hits ("<workspace-id>:<flow-slug>"),
"<workspace-id>/<flow-slug>", or two separate arguments.`,
		Example: strings.TrimSpace(`
` + commandPath + ` show BN2zoJYkBlO1DlDwQJoS/lead-research
` + commandPath + ` show BN2zoJYkBlO1DlDwQJoS:lead-research
` + commandPath + ` show BN2zoJYkBlO1DlDwQJoS lead-research
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, discoverRequiresAPIModeError(cmd))
			}
			var workspaceID, flowSlug string
			if len(args) == 2 {
				workspaceID = strings.TrimSpace(args[0])
				flowSlug = strings.TrimSpace(args[1])
			} else {
				var ok bool
				workspaceID, flowSlug, ok = splitDiscoverAppRef(args[0])
				if !ok {
					return writeErr(cmd, errors.New("app ref must be <workspace-id>/<flow-slug> (or <workspace-id>:<flow-slug> from a Discover hit)"))
				}
			}
			if workspaceID == "" || flowSlug == "" {
				return writeErr(cmd, errors.New("app ref must include both workspace id and flow slug"))
			}
			return doAPICommand(cmd, app, "flows.discover.get", map[string]any{
				"sourceWorkspaceId": workspaceID,
				"flowSlug":          flowSlug,
			})
		},
	}
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
				return writeErr(cmd, discoverRequiresAPIModeError(cmd))
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
				return writeErr(cmd, discoverRequiresAPIModeError(cmd))
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

func newFlowsDiscoverUpdateCmd(app *App, commandPath string) *cobra.Command {
	var public string
	cmd := &cobra.Command{
		Use:   "update <flow-slug> --public <true|false>",
		Short: "Set public discover visibility for a flow",
		Long: `Set whether a flow is visible in public discover/install surfaces.

This is the lower-level Discover flag. To publish or delist a flow across
marketplace, Discover, and the public app page together, use
` + "`breyta flows public publish <flow-slug>`" + ` or
` + "`breyta flows public delist <flow-slug>`" + `.

Requirements for ` + "`--public=true`" + `:
- the flow must have explicit public Discover visibility
- the flow must be installable/released for discover surfaces to use it

Typical authoring flow:
1. add ` + "`:discover {:public true}`" + ` in the source file
2. ` + "`breyta flows push --file ...`" + `
3. ` + "`breyta flows release <slug>`" + ` (or otherwise promote a live installable version)
4. ` + "`" + commandPath + " list`" + ` from another workspace to verify visibility
5. Open the marketing app page at ` + "`https://breyta.ai/apps/<flow-slug>`" + `

Use ` + "`breyta flows show <slug>`" + ` after updating to confirm stored metadata includes
` + "`discover.public`" + `.

Only a privileged workspace member can change this metadata.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, discoverRequiresAPIModeError(cmd))
			}
			flowSlug, publicValue, err := parseFlowSlugAndCLITrueFalseFlag("public", public, args, cmd.Flags().Changed("public"))
			if err != nil {
				return writeErr(cmd, err)
			}
			return doAPICommand(cmd, app, "flows.discover.update", map[string]any{
				"flowSlug": flowSlug,
				"public":   publicValue,
			})
		},
	}
	cmd.Flags().StringVar(&public, "public", "", "Public discover visibility state (true|false)")
	if f := cmd.Flags().Lookup("public"); f != nil {
		f.NoOptDefVal = cliBareTrueValue
	}
	_ = cmd.MarkFlagRequired("public")
	return cmd
}

func discoverRequiresAPIModeError(cmd *cobra.Command) error {
	path := ""
	if cmd != nil {
		path = strings.TrimSpace(cmd.CommandPath())
		if root := cmd.Root(); root != nil && strings.TrimSpace(root.Name()) != "" {
			path = strings.TrimSpace(strings.TrimPrefix(path, strings.TrimSpace(root.Name())))
		}
	}
	if path == "" {
		path = "discover"
	}
	return errors.New(path + " requires API mode")
}
