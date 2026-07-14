package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsConnectionsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connections",
		Short: "Inspect flow connection readiness",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFlowsConnectionsStatusCmd(app))
	cmd.AddCommand(newFlowsConnectionsSuggestCmd(app))
	cmd.AddCommand(newFlowsConnectionsSetupCmd(app))
	cmd.AddCommand(newFlowsConnectionsTestCmd(app))
	return cmd
}

func newFlowsConnectionsAPICommand(
	app *App,
	name string,
	apiCommand string,
	short string,
	long string,
) *cobra.Command {
	var source string
	var version string
	var step string

	cmd := &cobra.Command{
		Use:   name + " <flow-slug>",
		Short: short,
		Long:  strings.TrimSpace(long),
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows connections "+name+" requires API mode"))
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
			if strings.TrimSpace(step) != "" {
				payload["step"] = strings.TrimSpace(step)
			}

			out, status, err := apiClient(app).DoCommand(cmd.Context(), apiCommand, payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source (draft|active|latest|version)")
	cmd.Flags().StringVar(&version, "version", "", "Specific flow version when --source version")
	cmd.Flags().StringVar(&step, "step", "", "Limit readiness to one flow-local step id")
	return cmd
}

func newFlowsConnectionsStatusCmd(app *App) *cobra.Command {
	return newFlowsConnectionsAPICommand(
		app,
		"status",
		"flows.connections.status",
		"Show compact connection readiness for a flow",
		`
Show ready, missing, invalid, and unhealthy connection slots for a flow source.

Examples:
  breyta flows connections status my-flow --source draft
  breyta flows connections status my-flow --source draft --step tools/search
  breyta flows connections status my-flow --source active
  breyta flows connections status my-flow --source version --version 3
`,
	)
}

func newFlowsConnectionsSuggestCmd(app *App) *cobra.Command {
	return newFlowsConnectionsAPICommand(
		app,
		"suggest",
		"flows.connections.suggest",
		"Suggest flow connection bindings",
		`
Suggest flow connection bindings from existing workspace connections.

Examples:
  breyta flows connections suggest my-flow --source draft
  breyta flows connections suggest my-flow --source active
`,
	)
}

func newFlowsConnectionsSetupCmd(app *App) *cobra.Command {
	return newFlowsConnectionsAPICommand(
		app,
		"setup",
		"flows.connections.setup",
		"Show connection setup actions for a flow",
		`
Show compact setup actions for missing, invalid, and unhealthy flow connection slots.

Examples:
  breyta flows connections setup my-flow --source draft
  breyta flows connections setup my-flow --source version --version 3
`,
	)
}

func newFlowsConnectionsTestCmd(app *App) *cobra.Command {
	return newFlowsConnectionsAPICommand(
		app,
		"test",
		"flows.connections.test",
		"Test flow-bound connections",
		`
Test only the connections bound by a flow source.

Examples:
  breyta flows connections test my-flow --source draft
  breyta flows connections test my-flow --source active
`,
	)
}
