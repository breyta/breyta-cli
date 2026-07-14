package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsStatusCmd(app *App) *cobra.Command {
	var source string
	var version string
	var check bool

	cmd := &cobra.Command{
		Use:   "status <flow-slug>",
		Short: "Show compact step-first authoring status for a flow",
		Long: strings.TrimSpace(`
Show entrypoints, MCP exposure, steps, latest isolated run evidence, missing
schemas, stale verification, and connection readiness for one flow source.

Examples:
  breyta flows status my-flow --source draft
  breyta flows status my-flow --source draft --check
  breyta flows status my-flow --source version --version 3
`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows status requires API mode"))
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
				"source":   strings.TrimSpace(source),
			}
			if strings.TrimSpace(version) != "" {
				payload["version"] = strings.TrimSpace(version)
			}
			if check {
				payload["check"] = true
			}

			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.status", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source (draft|active|latest|version)")
	cmd.Flags().StringVar(&version, "version", "", "Specific flow version when --source version")
	cmd.Flags().BoolVar(&check, "check", false, "Fail when status checks are not ready")
	return cmd
}
