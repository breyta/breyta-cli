package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var flowGrepMatchSurfaceOrder = []string{"definition", "steps", "tools", "connections", "description"}

var flowGrepMatchSurfaceSet = func() map[string]bool {
	set := make(map[string]bool, len(flowGrepMatchSurfaceOrder))
	for _, surface := range flowGrepMatchSurfaceOrder {
		set[surface] = true
	}
	return set
}()

func searchScopeValue(raw string) (string, error) {
	scope := strings.TrimSpace(strings.ToLower(raw))
	if scope == "" {
		scope = "all"
	}
	if scope != "all" && scope != "workspace" {
		return "", errors.New("--catalog-scope must be 'all' or 'workspace'")
	}
	return scope, nil
}

func grepScopeValue(raw string) (string, error) {
	scope := strings.TrimSpace(strings.ToLower(raw))
	if scope == "" {
		scope = "workspace"
	}
	if scope != "workspace" && scope != "templates" && scope != "all" {
		return "", errors.New("--scope must be 'workspace', 'templates', or 'all'")
	}
	return scope, nil
}

func validFlowSearchTarget(raw string) (string, error) {
	target := strings.TrimSpace(strings.ToLower(raw))
	if target == "" {
		target = "latest"
	}
	if target != "latest" && target != "draft" && target != "live" {
		return "", errors.New("--target must be latest, draft, or live")
	}
	return target, nil
}

func appendFlowSearchFilters(payload map[string]any, provider, stepType, toolName, connection string) {
	if strings.TrimSpace(provider) != "" {
		payload["provider"] = strings.TrimSpace(provider)
	}
	if strings.TrimSpace(stepType) != "" {
		payload["stepType"] = strings.TrimSpace(stepType)
	}
	if strings.TrimSpace(toolName) != "" {
		payload["toolName"] = strings.TrimSpace(toolName)
	}
	if strings.TrimSpace(connection) != "" {
		payload["connection"] = strings.TrimSpace(connection)
	}
}

func hasFlowSearchFilter(provider, stepType, toolName, connection, flowSlug string) bool {
	for _, v := range []string{provider, stepType, toolName, connection, flowSlug} {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func hasGrepSelector(pattern string, ors []string, provider, stepType, toolName, connection, flowSlug string) bool {
	if strings.TrimSpace(pattern) != "" || hasFlowSearchFilter(provider, stepType, toolName, connection, flowSlug) {
		return true
	}
	for _, v := range ors {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func addPatternPayload(payload map[string]any, pattern string, ors []string) {
	if strings.TrimSpace(pattern) != "" {
		payload["query"] = strings.TrimSpace(pattern)
	}
	clean := make([]string, 0, len(ors))
	for _, v := range ors {
		if strings.TrimSpace(v) != "" {
			clean = append(clean, strings.TrimSpace(v))
		}
	}
	if len(clean) > 0 {
		payload["patterns"] = clean
	}
}

func normalizedMatchSurfaces(surfaces []string) ([]string, error) {
	clean := make([]string, 0, len(surfaces))
	seen := map[string]bool{}
	for _, raw := range surfaces {
		for _, part := range strings.Split(raw, ",") {
			surface := strings.TrimSpace(strings.ToLower(part))
			if surface == "" || seen[surface] {
				continue
			}
			if !flowGrepMatchSurfaceSet[surface] {
				return nil, fmt.Errorf("--surface must be one of %s (got %q)", strings.Join(flowGrepMatchSurfaceOrder, ", "), surface)
			}
			seen[surface] = true
			clean = append(clean, surface)
		}
	}
	return clean, nil
}

func appendMatchSurfacesPayload(payload map[string]any, surfaces []string) error {
	clean, err := normalizedMatchSurfaces(surfaces)
	if err != nil {
		return err
	}
	setMatchSurfacesPayload(payload, clean)
	return nil
}

func setMatchSurfacesPayload(payload map[string]any, clean []string) {
	if len(clean) > 0 {
		payload["matchSurfaces"] = clean
	}
}

func dispatchFlowAPICommand(cmd *cobra.Command, app *App, command string, payload map[string]any, allowGlobal bool) error {
	if allowGlobal && strings.TrimSpace(app.WorkspaceID) == "" {
		return doGlobalAPICommand(cmd, app, command, payload)
	}
	if useDoAPICommandFn {
		return doAPICommandFn(cmd, app, command, payload)
	}
	return doAPICommand(cmd, app, command, payload)
}

func dispatchFlowAPICommandWithTransform(cmd *cobra.Command, app *App, command string, payload map[string]any, allowGlobal bool, transform func(map[string]any)) error {
	if transform == nil {
		return dispatchFlowAPICommand(cmd, app, command, payload, allowGlobal)
	}
	if useDoAPICommandFn {
		return doAPICommandFn(cmd, app, command, payload)
	}
	if allowGlobal && strings.TrimSpace(app.WorkspaceID) == "" {
		if err := requireAPI(app); err != nil {
			return writeErr(cmd, err)
		}
		client := apiClient(app)
		out, status, err := client.DoGlobalCommand(context.Background(), command, payload)
		if err != nil {
			return writeErr(cmd, err)
		}
		trackCommandTelemetry(app, command, payload, status, status < 400 && isOK(out))
		transform(out)
		if err := writeAPIResult(cmd, app, out, status); err != nil {
			return writeErr(cmd, err)
		}
		return nil
	}
	out, status, err := runAPICommand(app, command, payload)
	if err != nil {
		return writeErr(cmd, err)
	}
	transform(out)
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
}

func dispatchGlobalFlowAPICommandWithTransform(cmd *cobra.Command, app *App, command string, payload map[string]any, transform func(map[string]any)) error {
	if err := requireAPI(app); err != nil {
		return writeErr(cmd, err)
	}
	client := apiClient(app)
	out, status, err := client.DoGlobalCommand(context.Background(), command, payload)
	if err != nil {
		return writeErr(cmd, err)
	}
	trackCommandTelemetry(app, command, payload, status, status < 400 && isOK(out))
	if transform != nil {
		transform(out)
	}
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
}

func newFlowsSearchCmd(app *App) *cobra.Command {
	var catalogScope string
	var provider string
	var stepType string
	var toolName string
	var connection string
	var flowSlug string
	var target string
	var limit int
	var from int
	var full bool
	var rawDefinition bool
	var includeArchived bool

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search workspace flows by metadata",
		Long: strings.TrimSpace(`
Search actual flows in the current workspace by name, description, tags, connections,
step metadata, providers, and indexed summaries.

Use ` + "`breyta flows grep`" + ` when you need to search source/definition content such as
tool envelopes, uploaded-resource fields, prompts, or step configuration literals.

Compatibility: approved reusable-template search has moved to
` + "`breyta flows templates search`" + `. For one release, ` + "`breyta flows search --catalog-scope ...`" + `,
` + "`breyta flows search --full`" + `, and no-workspace ` + "`breyta flows search`" + ` still use the old
approved-template search surface. ` + "`--full`" + ` includes a bounded source preview by default; use
` + "`--raw-definition`" + ` only when an inline full definition is required.
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

			workspaceID := strings.TrimSpace(app.WorkspaceID)
			if rawDefinition && !full {
				return writeErr(cmd, errors.New("--raw-definition requires --full"))
			}
			legacyTemplateSearch := workspaceID == "" || cmd.Flags().Changed("catalog-scope") || full
			if legacyTemplateSearch {
				if strings.TrimSpace(flowSlug) != "" {
					return writeErr(cmd, errors.New("--flow only applies to workspace search; use `breyta flows search` with a workspace, or remove --flow for template search"))
				}
				effectiveScope, err := searchScopeValue(catalogScope)
				if err != nil {
					return writeErr(cmd, err)
				}
				if effectiveScope == "workspace" && workspaceID == "" {
					return writeErr(cmd, errors.New("workspace-scoped template search requires --workspace or BREYTA_WORKSPACE"))
				}
				payload := map[string]any{
					"scope":             effectiveScope,
					"limit":             limit,
					"from":              from,
					"includeDefinition": full,
					"rawDefinition":     rawDefinition,
				}
				if query != "" {
					payload["query"] = query
				}
				appendFlowSearchFilters(payload, provider, stepType, toolName, connection)
				if full {
					return dispatchFlowAPICommand(cmd, app, "flows.search", payload, workspaceID == "" && effectiveScope == "all")
				}
				return dispatchFlowAPICommandWithTransform(cmd, app, "flows.search", payload, workspaceID == "" && effectiveScope == "all", compactTemplateSearchEnvelope)
			}

			if query == "" && !hasFlowSearchFilter(provider, stepType, toolName, connection, flowSlug) {
				return writeErr(cmd, errors.New("provide a query or filter such as --step-type, --tool-name, --provider, --connection, or --flow"))
			}
			effectiveTarget, err := validFlowSearchTarget(target)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"target":          effectiveTarget,
				"limit":           limit,
				"from":            from,
				"includeArchived": includeArchived,
			}
			if query != "" {
				payload["query"] = query
			}
			appendFlowSearchFilters(payload, provider, stepType, toolName, connection)
			if strings.TrimSpace(flowSlug) != "" {
				payload["flowSlug"] = strings.TrimSpace(flowSlug)
			}
			return dispatchFlowAPICommand(cmd, app, "flows.workspace.search", payload, false)
		},
	}

	cmd.Flags().StringVar(&catalogScope, "catalog-scope", "all", "Deprecated compatibility: approved template scope all|workspace")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token (e.g. openai, google, slack)")
	cmd.Flags().StringVar(&stepType, "step-type", "", "Filter by primitive step type (e.g. http, llm, agent, search)")
	cmd.Flags().StringVar(&toolName, "tool-name", "", "Filter by indexed tool-call name (e.g. web_search)")
	cmd.Flags().StringVar(&connection, "connection", "", "Filter by connection slot/provider token")
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Limit workspace search to one known flow slug")
	cmd.Flags().StringVar(&target, "target", "latest", "Workspace source target: latest|draft|live")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Deprecated compatibility: include bounded approved template source preview")
	cmd.Flags().BoolVar(&rawDefinition, "raw-definition", false, "With --full, include the raw full source definition inline; verbose")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived workspace flows")
	return cmd
}

func newFlowsGrepCmd(app *App) *cobra.Command {
	var scope string
	var ors []string
	var provider string
	var stepType string
	var toolName string
	var connection string
	var matchSurfaces []string
	var flowSlug string
	var target string
	var limit int
	var from int
	var includeArchived bool
	var full bool
	var rawDefinition bool

	cmd := &cobra.Command{
		Use:   "grep [pattern]",
		Short: "Search workspace or template flow source",
		Long: strings.TrimSpace(`
Search flow definitions/source with literal case-insensitive matching. This is the
power search for "show me flows that configure X" questions.

By default it searches actual workspace flows. Use ` + "`--scope templates`" + ` for approved
reusable templates or ` + "`--scope all`" + ` to combine workspace and template hits. Use
repeatable ` + "`--or <pattern>`" + ` for spelling variations. Grep does not do hidden
synonym expansion.

Results are compact by default. Add ` + "`--full`" + ` for a bounded source preview, or
` + "`--full --raw-definition`" + ` for the raw definition inline.
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows grep requires API mode"))
			}
			pattern := ""
			if len(args) > 0 {
				pattern = strings.TrimSpace(args[0])
			}
			if !hasGrepSelector(pattern, ors, provider, stepType, toolName, connection, flowSlug) {
				return writeErr(cmd, errors.New("provide a grep pattern, --or pattern, or a filter such as --step-type, --tool-name, --provider, --connection, or --flow"))
			}
			effectiveScope, err := grepScopeValue(scope)
			if err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(flowSlug) != "" && effectiveScope != "workspace" {
				return writeErr(cmd, errors.New("--flow only applies to workspace grep; use --scope workspace or remove --flow for template/all grep"))
			}
			effectiveTarget, err := validFlowSearchTarget(target)
			if err != nil {
				return writeErr(cmd, err)
			}
			if (effectiveScope == "workspace" || effectiveScope == "all") && strings.TrimSpace(app.WorkspaceID) == "" {
				return writeErr(cmd, errors.New("workspace grep requires --workspace or BREYTA_WORKSPACE; use --scope templates without workspace context"))
			}
			if rawDefinition && !full {
				return writeErr(cmd, errors.New("--raw-definition requires --full"))
			}
			normalizedSurfaces, err := normalizedMatchSurfaces(matchSurfaces)
			if err != nil {
				return writeErr(cmd, err)
			}

			workspacePayload := map[string]any{
				"definitionSearch":  true,
				"target":            effectiveTarget,
				"limit":             limit,
				"from":              from,
				"includeArchived":   includeArchived,
				"includeDefinition": full,
				"rawDefinition":     rawDefinition,
			}
			addPatternPayload(workspacePayload, pattern, ors)
			appendFlowSearchFilters(workspacePayload, provider, stepType, toolName, connection)
			setMatchSurfacesPayload(workspacePayload, normalizedSurfaces)
			if strings.TrimSpace(flowSlug) != "" {
				workspacePayload["flowSlug"] = strings.TrimSpace(flowSlug)
			}

			templatePayload := map[string]any{
				"definitionSearch":  true,
				"scope":             "all",
				"surface":           "templates",
				"limit":             limit,
				"from":              from,
				"includeDefinition": full,
				"rawDefinition":     rawDefinition,
			}
			addPatternPayload(templatePayload, pattern, ors)
			appendFlowSearchFilters(templatePayload, provider, stepType, toolName, connection)
			setMatchSurfacesPayload(templatePayload, normalizedSurfaces)

			switch effectiveScope {
			case "workspace":
				return dispatchFlowAPICommand(cmd, app, "flows.workspace.search", workspacePayload, false)
			case "templates":
				// --full requests a source preview, so skip the compacting
				// transform that would strip the definition back out.
				if full {
					return dispatchFlowAPICommand(cmd, app, "flows.search", templatePayload, strings.TrimSpace(app.WorkspaceID) == "")
				}
				return dispatchFlowAPICommandWithTransform(cmd, app, "flows.search", templatePayload, strings.TrimSpace(app.WorkspaceID) == "", compactTemplateSearchEnvelope)
			default:
				return runCombinedFlowGrep(cmd, app, workspacePayload, templatePayload, limit, full)
			}
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "workspace", "Search scope: workspace|templates|all")
	cmd.Flags().StringArrayVar(&ors, "or", nil, "Additional literal pattern alternative; repeat for variations")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token")
	cmd.Flags().StringVar(&stepType, "step-type", "", "Filter by primitive step type")
	cmd.Flags().StringVar(&toolName, "tool-name", "", "Filter by indexed tool-call name")
	cmd.Flags().StringVar(&connection, "connection", "", "Filter by connection slot/provider token")
	cmd.Flags().StringArrayVar(&matchSurfaces, "surface", nil, "Limit literal matches to a surface: definition|steps|tools|connections|description (repeatable or comma-separated)")
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Limit workspace search to one known flow slug")
	cmd.Flags().StringVar(&target, "target", "latest", "Workspace source target: latest|draft|live")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max results per scope (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived workspace flows")
	cmd.Flags().BoolVar(&full, "full", false, "Include bounded source definition preview for matched flows")
	cmd.Flags().BoolVar(&rawDefinition, "raw-definition", false, "With --full, include raw source definition inline; verbose")
	return cmd
}

func runCombinedFlowGrep(cmd *cobra.Command, app *App, workspacePayload, templatePayload map[string]any, limit int, full bool) error {
	workspaceOut, workspaceStatus, err := runAPICommand(app, "flows.workspace.search", workspacePayload)
	if err != nil {
		return writeErr(cmd, err)
	}
	if workspaceStatus >= 400 || !isOK(workspaceOut) {
		return writeAPIResult(cmd, app, workspaceOut, workspaceStatus)
	}
	templateOut, templateStatus, err := runAPICommand(app, "flows.search", templatePayload)
	if err != nil {
		return writeErr(cmd, err)
	}
	if templateStatus >= 400 || !isOK(templateOut) {
		return writeAPIResult(cmd, app, templateOut, templateStatus)
	}
	// --full requests a source preview, so keep the template definitions
	// instead of compacting them back out (matches the templates scope path).
	if !full {
		compactTemplateSearchEnvelope(templateOut)
	}

	hits := append(resultHits(workspaceOut), resultHits(templateOut)...)
	meta := map[string]any{
		"scope":         "all",
		"limit":         limit,
		"workspaceHits": len(resultHits(workspaceOut)),
		"templateHits":  len(resultHits(templateOut)),
		"nextCommands": []string{
			"breyta flows grep <pattern> --scope workspace",
			"breyta flows grep <pattern> --scope templates",
		},
	}
	if matchSurfaces, ok := workspacePayload["matchSurfaces"]; ok {
		meta["matchSurfaces"] = matchSurfaces
	}
	return writeData(cmd, app, meta, map[string]any{
		"result": map[string]any{
			"hits":      hits,
			"totalHits": len(hits),
		},
	})
}

func resultHits(out map[string]any) []any {
	data, _ := out["data"].(map[string]any)
	result, _ := data["result"].(map[string]any)
	raw, _ := result["hits"].([]any)
	return raw
}

func newFlowsTemplatesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "Search approved reusable flow templates",
		Long: strings.TrimSpace(`
Search approved reusable templates. This is the copy-from surface formerly reached
through ` + "`breyta flows search`" + `.

Use ` + "`breyta flows search`" + ` for actual workspace flow metadata and ` + "`breyta flows grep`" + `
for source/content search.
`),
	}
	cmd.AddCommand(newFlowsTemplatesSearchCmd(app))
	cmd.AddCommand(newFlowsTemplatesGrepCmd(app))
	cmd.AddCommand(newFlowsTemplatesDuplicateCmd(app))
	examples := newFlowsExamplesCmd(app)
	examples.Short = "Extract primitive examples from approved templates"
	cmd.AddCommand(examples)
	return cmd
}

func newFlowsTemplatesDuplicateCmd(app *App) *cobra.Command {
	var targetSlug string
	var name string
	var description string
	var catalogScope string
	var outFormat string
	var replace bool

	cmd := &cobra.Command{
		Use:     "duplicate <template-slug>",
		Aliases: []string{"copy", "import"},
		Short:   "Duplicate an approved template into this workspace",
		Long: strings.TrimSpace(`
Duplicate an approved reusable template into the current workspace as a workspace-owned
draft. Use this before custom authoring when a template closely matches the requested
outcome: copy the known-good base, prove one green draft run, then make narrow edits.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows templates duplicate requires API mode"))
			}
			if err := validateJSONOnlyFormat(outFormat, "flows templates duplicate"); err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(app.WorkspaceID) == "" {
				return writeErr(cmd, errors.New("flows templates duplicate requires --workspace or BREYTA_WORKSPACE"))
			}
			effectiveScope, err := searchScopeValue(catalogScope)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"templateSlug": strings.TrimSpace(args[0]),
				"scope":        effectiveScope,
			}
			if strings.TrimSpace(targetSlug) != "" {
				payload["targetSlug"] = strings.TrimSpace(targetSlug)
			}
			if strings.TrimSpace(name) != "" {
				payload["name"] = strings.TrimSpace(name)
			}
			if strings.TrimSpace(description) != "" {
				payload["description"] = strings.TrimSpace(description)
			}
			if replace {
				payload["replace"] = true
			}
			return dispatchFlowAPICommand(cmd, app, "flows.templates.duplicate", payload, false)
		},
	}

	cmd.Flags().StringVar(&targetSlug, "slug", "", "Target workspace flow slug; defaults to <template-slug>-copy")
	cmd.Flags().StringVar(&name, "name", "", "Override copied flow name")
	cmd.Flags().StringVar(&description, "description", "", "Override copied flow description")
	cmd.Flags().StringVar(&catalogScope, "catalog-scope", "all", "Template catalog scope: all|workspace")
	cmd.Flags().BoolVar(&replace, "replace", false, "Replace the existing target draft when --slug already exists")
	cmd.Flags().StringVar(&outFormat, "format", "json", "Output format (json)")
	return cmd
}

func newFlowsTemplatesSearchCmd(app *App) *cobra.Command {
	var catalogScope string
	var provider string
	var stepType string
	var toolName string
	var connection string
	var outFormat string
	var limit int
	var from int
	var full bool
	var rawDefinition bool

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search approved reusable templates",
		Long: strings.TrimSpace(`
Search across approved reusable flow templates to find patterns to copy from.

Only flows explicitly approved by Breyta for reuse are indexed here. Public
installable flows live under ` + "`breyta flows discover search`" + `.
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows templates search requires API mode"))
			}
			if err := validateJSONOnlyFormat(outFormat, "flows templates search"); err != nil {
				return writeErr(cmd, err)
			}
			query := ""
			if len(args) > 0 {
				query = strings.TrimSpace(args[0])
			}
			effectiveScope, err := searchScopeValue(catalogScope)
			if err != nil {
				return writeErr(cmd, err)
			}
			if effectiveScope == "workspace" && strings.TrimSpace(app.WorkspaceID) == "" {
				return writeErr(cmd, errors.New("workspace-scoped template search requires --workspace or BREYTA_WORKSPACE"))
			}
			if rawDefinition && !full {
				return writeErr(cmd, errors.New("--raw-definition requires --full"))
			}
			payload := map[string]any{
				"scope":             effectiveScope,
				"surface":           "templates",
				"limit":             limit,
				"from":              from,
				"includeDefinition": full,
				"rawDefinition":     rawDefinition,
			}
			if query != "" {
				payload["query"] = query
			}
			appendFlowSearchFilters(payload, provider, stepType, toolName, connection)
			if effectiveScope == "all" {
				if full {
					return dispatchGlobalFlowAPICommandWithTransform(cmd, app, "flows.search", payload, nil)
				}
				return dispatchGlobalFlowAPICommandWithTransform(cmd, app, "flows.search", payload, compactTemplateSearchEnvelope)
			}
			if full {
				return dispatchFlowAPICommand(cmd, app, "flows.search", payload, false)
			}
			return dispatchFlowAPICommandWithTransform(cmd, app, "flows.search", payload, false, compactTemplateSearchEnvelope)
		},
	}

	cmd.Flags().StringVar(&catalogScope, "catalog-scope", "all", "Template catalog scope: all|workspace")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token")
	cmd.Flags().StringVar(&stepType, "step-type", "", "Filter by primitive step type")
	cmd.Flags().StringVar(&toolName, "tool-name", "", "Filter by indexed tool-call name")
	cmd.Flags().StringVar(&connection, "connection", "", "Filter by connection slot/provider token")
	cmd.Flags().StringVar(&outFormat, "format", "json", "Output format (json)")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include bounded indexed template source preview")
	cmd.Flags().BoolVar(&rawDefinition, "raw-definition", false, "With --full, include the raw full source definition inline; verbose")
	return cmd
}

func newFlowsTemplatesGrepCmd(app *App) *cobra.Command {
	var catalogScope string
	var ors []string
	var provider string
	var stepType string
	var toolName string
	var connection string
	var matchSurfaces []string
	var outFormat string
	var limit int
	var from int
	var full bool
	var rawDefinition bool

	cmd := &cobra.Command{
		Use:   "grep [pattern]",
		Short: "Search approved template source",
		Long: strings.TrimSpace(`
Search approved reusable template definitions with literal case-insensitive matching.
Use repeatable ` + "`--or <pattern>`" + ` for spelling variations. Grep does not do
hidden semantic or synonym expansion.
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows templates grep requires API mode"))
			}
			if err := validateJSONOnlyFormat(outFormat, "flows templates grep"); err != nil {
				return writeErr(cmd, err)
			}
			pattern := ""
			if len(args) > 0 {
				pattern = strings.TrimSpace(args[0])
			}
			if !hasGrepSelector(pattern, ors, provider, stepType, toolName, connection, "") {
				return writeErr(cmd, errors.New("provide a grep pattern, --or pattern, or a filter such as --step-type, --tool-name, --provider, or --connection"))
			}
			effectiveScope, err := searchScopeValue(catalogScope)
			if err != nil {
				return writeErr(cmd, err)
			}
			if effectiveScope == "workspace" && strings.TrimSpace(app.WorkspaceID) == "" {
				return writeErr(cmd, errors.New("workspace-scoped template grep requires --workspace or BREYTA_WORKSPACE"))
			}
			if rawDefinition && !full {
				return writeErr(cmd, errors.New("--raw-definition requires --full"))
			}
			payload := map[string]any{
				"definitionSearch":  true,
				"scope":             effectiveScope,
				"surface":           "templates",
				"limit":             limit,
				"from":              from,
				"includeDefinition": full,
				"rawDefinition":     rawDefinition,
			}
			addPatternPayload(payload, pattern, ors)
			appendFlowSearchFilters(payload, provider, stepType, toolName, connection)
			if err := appendMatchSurfacesPayload(payload, matchSurfaces); err != nil {
				return writeErr(cmd, err)
			}
			if effectiveScope == "all" {
				if full {
					return dispatchGlobalFlowAPICommandWithTransform(cmd, app, "flows.search", payload, nil)
				}
				return dispatchGlobalFlowAPICommandWithTransform(cmd, app, "flows.search", payload, compactTemplateSearchEnvelope)
			}
			if full {
				return dispatchFlowAPICommand(cmd, app, "flows.search", payload, false)
			}
			return dispatchFlowAPICommandWithTransform(cmd, app, "flows.search", payload, false, compactTemplateSearchEnvelope)
		},
	}

	cmd.Flags().StringVar(&catalogScope, "catalog-scope", "all", "Template catalog scope: all|workspace")
	cmd.Flags().StringArrayVar(&ors, "or", nil, "Additional literal pattern alternative; repeat for variations")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider token")
	cmd.Flags().StringVar(&stepType, "step-type", "", "Filter by primitive step type")
	cmd.Flags().StringVar(&toolName, "tool-name", "", "Filter by indexed tool-call name")
	cmd.Flags().StringVar(&connection, "connection", "", "Filter by connection slot/provider token")
	cmd.Flags().StringArrayVar(&matchSurfaces, "surface", nil, "Limit literal matches to a surface: definition|steps|tools|connections|description (repeatable or comma-separated)")
	cmd.Flags().StringVar(&outFormat, "format", "json", "Output format (json)")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&full, "full", false, "Include bounded source definition preview for matched templates")
	cmd.Flags().BoolVar(&rawDefinition, "raw-definition", false, "With --full, include raw source definition EDN inline; verbose")
	return cmd
}
