package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsExamplesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "examples",
		Short: "Extract primitive examples from approved flows",
	}
	cmd.AddCommand(newFlowsExamplesStepCmd(app))
	return cmd
}

func newFlowsExamplesStepCmd(app *App) *cobra.Command {
	var catalogScope string
	var limit int
	var full bool

	cmd := &cobra.Command{
		Use:   "step <type> [query]",
		Short: "Extract matching step snippets from approved reusable flows",
		Long: strings.TrimSpace(`
Extract primitive-level examples from approved reusable flows.

By default, snippets exclude the current workspace so private/workspace examples are only used
when explicitly requested with --catalog-scope workspace. Returned snippets include the matching
step config plus referenced requires, templates, and functions.
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows examples step requires API mode"))
			}
			stepType := strings.TrimSpace(args[0])
			if stepType == "" {
				return writeErr(cmd, errors.New("missing step type"))
			}
			query := ""
			if len(args) > 1 {
				query = strings.TrimSpace(args[1])
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
				return writeErr(cmd, errors.New("workspace-scoped examples require --workspace or BREYTA_WORKSPACE"))
			}
			payload := map[string]any{
				"stepType": stepType,
				"scope":    effectiveScope,
				"limit":    limit,
				"full":     full,
			}
			if query != "" {
				payload["query"] = query
			}
			if workspaceID == "" && effectiveScope == "all" {
				return doGlobalAPICommand(cmd, app, "flows.examples.step", payload)
			}
			return doAPICommand(cmd, app, "flows.examples.step", payload)
		},
	}
	cmd.Flags().StringVar(&catalogScope, "catalog-scope", "all", "Catalog scope: all|workspace")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max snippets to return")
	cmd.Flags().BoolVar(&full, "full", false, "Include source definition EDN on returned snippets")
	return cmd
}
