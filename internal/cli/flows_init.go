package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsInitCmd(app *App) *cobra.Command {
	var name string
	var description string
	var empty bool

	cmd := &cobra.Command{
		Use:   "init <flow-slug>",
		Short: "Create an empty draft flow for step-first authoring",
		Long: strings.TrimSpace(`
Create a minimal draft flow that can receive interfaces, schedules, steps,
checks, and later full source edits.

Examples:
  breyta flows init company-profile --empty --name "Company profile"
  breyta flows init company-profile --empty --name "Company profile" --description "Marketing profile builder"
`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows init requires API mode"))
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !empty {
				return writeErr(cmd, errors.New("flows init currently requires --empty"))
			}
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
				"empty":    true,
			}
			if strings.TrimSpace(name) != "" {
				payload["name"] = strings.TrimSpace(name)
			}
			if strings.TrimSpace(description) != "" {
				payload["description"] = strings.TrimSpace(description)
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.init", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().BoolVar(&empty, "empty", false, "Create an empty step-first draft")
	cmd.Flags().StringVar(&name, "name", "", "Flow display name")
	cmd.Flags().StringVar(&description, "description", "", "Flow description")
	return cmd
}
