package cli

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsInstallationsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "installations",
		Aliases: []string{"installation", "installs", "install"},
		Short:   "Manage end-user installations of a flow",
		Long: strings.TrimSpace(`
An installation is a per-user instance of an end-user-facing flow.

In the backend, an installation is implemented as a prod flow profile scoped to the current user.
`),
	}
	cmd.AddCommand(newFlowsInstallationsListCmd(app))
	cmd.AddCommand(newFlowsInstallationsCreateCmd(app))
	cmd.AddCommand(newFlowsInstallationsGetCmd(app))
	cmd.AddCommand(newFlowsInstallationsRenameCmd(app))
	cmd.AddCommand(newFlowsInstallationsSetInputsCmd(app))
	cmd.AddCommand(newFlowsInstallationsSetEnabledCmd(app))
	cmd.AddCommand(newFlowsInstallationsEnableCmd(app))
	cmd.AddCommand(newFlowsInstallationsDisableCmd(app))
	cmd.AddCommand(newFlowsInstallationsDeleteCmd(app))
	cmd.AddCommand(newFlowsInstallationsTriggersCmd(app))
	cmd.AddCommand(newFlowsInstallationsUploadCmd(app))
	return cmd
}

func newFlowsInstallationsListCmd(app *App) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List your installations for an end-user flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations list requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if all {
				payload["all"] = true
			}
			return doAPICommand(cmd, app, "flows.installations.list", payload)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "List all installations for the flow (creator-only)")
	return cmd
}

func newFlowsInstallationsCreateCmd(app *App) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create <flow-slug>",
		Short: "Create a new installation (disabled until enabled)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations create requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if strings.TrimSpace(name) != "" {
				payload["name"] = strings.TrimSpace(name)
			}
			return doAPICommand(cmd, app, "flows.installations.create", payload)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Installation name (optional)")
	return cmd
}

func newFlowsInstallationsRenameCmd(app *App) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "rename <profile-id> --name <name>",
		Short: "Rename an installation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations rename requires API mode"))
			}
			if strings.TrimSpace(name) == "" {
				return writeErr(cmd, errors.New("missing --name"))
			}
			return doAPICommand(cmd, app, "flows.installations.rename", map[string]any{
				"profileId": args[0],
				"name":      strings.TrimSpace(name),
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "New installation name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newFlowsInstallationsSetInputsCmd(app *App) *cobra.Command {
	var inputJSON string
	cmd := &cobra.Command{
		Use:   "set-inputs <profile-id> --input '{...}'",
		Short: "Set activation inputs for an installation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations set-inputs requires API mode"))
			}
			if strings.TrimSpace(inputJSON) == "" {
				return writeErr(cmd, errors.New("missing --input"))
			}
			m, err := parseJSONObjectFlag(inputJSON)
			if err != nil {
				return writeErr(cmd, err)
			}
			return doAPICommand(cmd, app, "flows.installations.set_inputs", map[string]any{
				"profileId": args[0],
				"inputs":    m,
			})
		},
	}
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object of activation inputs")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func newFlowsInstallationsSetEnabledCmd(app *App) *cobra.Command {
	var enabled bool
	cmd := &cobra.Command{
		Use:   "set-enabled <profile-id> --enabled",
		Short: "Toggle installation enabled state (pause/resume)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations set-enabled requires API mode"))
			}
			if !cmd.Flags().Changed("enabled") {
				return writeErr(cmd, errors.New("missing --enabled (true|false)"))
			}
			return doAPICommand(cmd, app, "flows.installations.set_enabled", map[string]any{
				"profileId": args[0],
				"enabled":   enabled,
			})
		},
	}
	cmd.Flags().BoolVar(&enabled, "enabled", false, "Enabled state")
	return cmd
}

func newFlowsInstallationsEnableCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <profile-id>",
		Short: "Enable an installation (activate)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations enable requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.installations.set_enabled", map[string]any{
				"profileId": args[0],
				"enabled":   true,
			})
		},
	}
	return cmd
}

func newFlowsInstallationsDisableCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <profile-id>",
		Short: "Disable an installation (pause)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations disable requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.installations.set_enabled", map[string]any{
				"profileId": args[0],
				"enabled":   false,
			})
		},
	}
	return cmd
}

func newFlowsInstallationsDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <profile-id>",
		Aliases: []string{"uninstall"},
		Short:   "Delete an installation (uninstall)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations delete requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.installations.delete", map[string]any{
				"profileId": args[0],
			})
		},
	}
	return cmd
}

func parseJSONObjectFlag(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("input must be a JSON object")
	}
	return m, nil
}
