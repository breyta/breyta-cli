package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsWorkspaceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Search private workspace flows for reuse",
		Long: strings.TrimSpace(`
Search actual flows in the current workspace for compact reuse evidence.

This is different from ` + "`breyta flows search`" + `, which searches approved reusable example
flows. Workspace search is for private/local patterns already authored in your workspace.
It returns compact metadata and matching step evidence, not full flow definitions.
`),
	}
	cmd.AddCommand(newFlowsWorkspaceSearchCmd(app))
	cmd.AddCommand(newFlowsWorkspaceExamplesCmd(app))
	return cmd
}

func newFlowsWorkspaceSearchCmd(app *App) *cobra.Command {
	var stepType string
	var flowSlug string
	var target string
	var limit int
	var from int
	var includeArchived bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search actual workspace flows without listing every flow",
		Long: strings.TrimSpace(`
Search private flows in the current workspace by name, description, tags, connections,
requires, steps, templates, and functions.

Use this before broad flow lists when looking for local examples, such as:
` + "`breyta flows workspace search \"gmail\" --step-type http --limit 5`" + `
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows workspace search requires API mode"))
			}
			if strings.TrimSpace(app.WorkspaceID) == "" {
				return writeErr(cmd, errors.New("workspace flow search requires --workspace or BREYTA_WORKSPACE"))
			}
			query := strings.TrimSpace(args[0])
			if query == "" {
				return writeErr(cmd, errors.New("missing query"))
			}
			effectiveTarget := strings.TrimSpace(strings.ToLower(target))
			if effectiveTarget == "" {
				effectiveTarget = "latest"
			}
			if effectiveTarget != "latest" && effectiveTarget != "draft" && effectiveTarget != "live" {
				return writeErr(cmd, errors.New("--target must be latest, draft, or live"))
			}

			payload := map[string]any{
				"query":           query,
				"target":          effectiveTarget,
				"limit":           limit,
				"from":            from,
				"includeArchived": includeArchived,
			}
			if strings.TrimSpace(stepType) != "" {
				payload["stepType"] = strings.TrimSpace(stepType)
			}
			if strings.TrimSpace(flowSlug) != "" {
				payload["flowSlug"] = strings.TrimSpace(flowSlug)
			}
			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "flows.workspace.search", payload)
			}
			return doAPICommand(cmd, app, "flows.workspace.search", payload)
		},
	}

	cmd.Flags().StringVar(&stepType, "step-type", "", "Filter by primitive step type (e.g. http, llm, search)")
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Limit search to one known workspace flow slug")
	cmd.Flags().StringVar(&target, "target", "latest", "Flow source target: latest|draft|live")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (1..100 recommended)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination (>= 0)")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived workspace flows")
	return cmd
}

func newFlowsWorkspaceExamplesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "examples",
		Short: "Extract snippets from private workspace flows",
	}
	cmd.AddCommand(newFlowsWorkspaceExamplesStepCmd(app))
	return cmd
}

func newFlowsWorkspaceExamplesStepCmd(app *App) *cobra.Command {
	var flowSlug string
	var target string
	var limit int
	var full bool
	var includeArchived bool

	cmd := &cobra.Command{
		Use:   "step <type> <query>",
		Short: "Extract matching step snippets from private workspace flows",
		Long: strings.TrimSpace(`
Extract primitive-level examples from actual flows in the current workspace.

Returned snippets include the matching step config plus referenced requires, templates,
and functions. Even with --full, this command expands only matched snippet context;
pull a full flow explicitly with ` + "`breyta flows pull <slug>`" + ` when architecture-level reuse is needed.
`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows workspace examples step requires API mode"))
			}
			if strings.TrimSpace(app.WorkspaceID) == "" {
				return writeErr(cmd, errors.New("workspace examples require --workspace or BREYTA_WORKSPACE"))
			}
			stepType := strings.TrimSpace(args[0])
			query := strings.TrimSpace(args[1])
			if stepType == "" {
				return writeErr(cmd, errors.New("missing step type"))
			}
			if query == "" {
				return writeErr(cmd, errors.New("missing query"))
			}
			effectiveTarget := strings.TrimSpace(strings.ToLower(target))
			if effectiveTarget == "" {
				effectiveTarget = "latest"
			}
			if effectiveTarget != "latest" && effectiveTarget != "draft" && effectiveTarget != "live" {
				return writeErr(cmd, errors.New("--target must be latest, draft, or live"))
			}

			payload := map[string]any{
				"stepType":        stepType,
				"query":           query,
				"target":          effectiveTarget,
				"limit":           limit,
				"full":            full,
				"includeArchived": includeArchived,
			}
			if strings.TrimSpace(flowSlug) != "" {
				payload["flowSlug"] = strings.TrimSpace(flowSlug)
			}
			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "flows.workspace.examples.step", payload)
			}
			return doAPICommand(cmd, app, "flows.workspace.examples.step", payload)
		},
	}

	cmd.Flags().StringVar(&flowSlug, "flow", "", "Limit extraction to one known workspace flow slug")
	cmd.Flags().StringVar(&target, "target", "latest", "Flow source target: latest|draft|live")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max snippets to return")
	cmd.Flags().BoolVar(&full, "full", false, "Include expanded matched-snippet context without dumping full flows")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived workspace flows")
	return cmd
}
