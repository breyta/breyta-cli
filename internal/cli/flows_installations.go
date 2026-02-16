package cli

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsInstallationsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Manage flow installations",
		Long: strings.TrimSpace(`
Advanced runtime targeting and rollout controls.

Most users should use:
- breyta flows run <flow-slug>

Use install commands only when you need explicit scopes, installation-specific
config/triggers, or controlled promotion.
`),
	}
	cmd.AddCommand(newFlowsInstallationsListCmd(app))
	cmd.AddCommand(newFlowsInstallationsCreateCmd(app))
	cmd.AddCommand(newFlowsInstallationsGetCmd(app))
	cmd.AddCommand(newFlowsInstallationsRenameCmd(app))
	cmd.AddCommand(newFlowsInstallationsSetInputsCmd(app))
	cmd.AddCommand(newFlowsInstallationsPromoteCmd(app))
	cmd.AddCommand(newFlowsInstallationsSetEnabledCmd(app))
	cmd.AddCommand(newFlowsInstallationsEnableCmd(app))
	cmd.AddCommand(newFlowsInstallationsDisableCmd(app))
	cmd.AddCommand(newFlowsInstallationsDeleteCmd(app))
	cmd.AddCommand(newFlowsInstallationsTriggersCmd(app))
	cmd.AddCommand(newFlowsInstallationsUploadCmd(app))
	return cmd
}

func newFlowsInstallationsLegacyCmd(app *App) *cobra.Command {
	cmd := newFlowsInstallationsCmd(app)
	cmd.Use = "installations"
	cmd.Aliases = []string{"installation", "installs"}
	cmd.Short = "Manage flow installations (legacy alias)"
	cmd.Hidden = true
	return cmd
}

func newFlowsInstallationsListCmd(app *App) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List installations for a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows install list requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if all {
				payload["all"] = true
			}
			return doAPICommand(cmd, app, "flows.install.list", payload)
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
				return writeErr(cmd, errors.New("flows install create requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if strings.TrimSpace(name) != "" {
				payload["name"] = strings.TrimSpace(name)
			}
			return doAPICommand(cmd, app, "flows.install.create", payload)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Installation name (optional)")
	return cmd
}

func newFlowsInstallationsRenameCmd(app *App) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "rename <installation-id> --name <name>",
		Short: "Rename an installation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows install rename requires API mode"))
			}
			if strings.TrimSpace(name) == "" {
				return writeErr(cmd, errors.New("missing --name"))
			}
			return doAPICommand(cmd, app, "flows.install.rename", map[string]any{
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
		Use:     "configure <installation-id> --input '{...}'",
		Aliases: []string{"set-inputs"},
		Short:   "Configure installation inputs",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows install configure requires API mode"))
			}
			if strings.TrimSpace(inputJSON) == "" {
				return writeErr(cmd, errors.New("missing --input"))
			}
			m, err := parseJSONObjectFlag(inputJSON)
			if err != nil {
				return writeErr(cmd, err)
			}
			return doAPICommand(cmd, app, "flows.install.configure", map[string]any{
				"profileId": args[0],
				"inputs":    m,
			})
		},
	}
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object of activation inputs")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func normalizeInstallScope(scope string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(scope))
	if s == "" {
		return "live", nil
	}
	switch s {
	case "live":
		return s, nil
	default:
		return "", errors.New("invalid --scope (expected live)")
	}
}

func normalizeInstallPolicy(policy string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(policy))
	if p == "" {
		return "", nil
	}
	switch p {
	case "pinned", "track-latest":
		return p, nil
	default:
		return "", errors.New("invalid --policy (expected pinned or track-latest)")
	}
}

func newFlowsInstallationsPromoteCmd(app *App) *cobra.Command {
	var version string
	var scope string
	var policy string
	cmd := &cobra.Command{
		Use:   "promote <flow-slug>",
		Short: "Advanced: promote a released version to an installation scope",
		Long: strings.TrimSpace(`
Advanced rollout command.

Default path:
- breyta flows run <flow-slug>

Use promote when you need explicit scoped targets (for example live)
or pinned rollout control.
`),
		Example: strings.TrimSpace(`
breyta flows install promote order-ingest --scope live
breyta flows install promote order-ingest --scope live --version 42
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows install promote requires API mode"))
			}
			resolvedScope, err := normalizeInstallScope(scope)
			if err != nil {
				return writeErr(cmd, err)
			}
			resolvedPolicy, err := normalizeInstallPolicy(policy)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"flowSlug": args[0],
				"scope":    resolvedScope,
			}
			if strings.TrimSpace(version) != "" && strings.TrimSpace(version) != "latest" {
				v, err := parsePositiveIntFlag(version)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["version"] = v
			}
			if resolvedPolicy != "" {
				payload["policy"] = resolvedPolicy
			}
			if resolvedScope != "live" {
				return writeErr(cmd, errors.New("flows install promote currently supports --scope live only"))
			}
			return doAPICommand(cmd, app, "flows.install.promote", payload)
		},
	}
	cmd.Flags().StringVar(&version, "version", "latest", "Release version to promote (or latest)")
	cmd.Flags().StringVar(&scope, "scope", "live", "Advanced: installation scope (live only)")
	cmd.Flags().StringVar(&policy, "policy", "", "Advanced: install policy override (pinned|track-latest)")
	return cmd
}

func newFlowsInstallationsSetEnabledCmd(app *App) *cobra.Command {
	var enabled bool
	cmd := &cobra.Command{
		Use:   "set-enabled <installation-id> --enabled",
		Short: "Toggle installation enabled state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows install set-enabled requires API mode"))
			}
			if !cmd.Flags().Changed("enabled") {
				return writeErr(cmd, errors.New("missing --enabled (true|false)"))
			}
			command := "flows.install.disable"
			if enabled {
				command = "flows.install.enable"
			}
			return doAPICommand(cmd, app, command, map[string]any{
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
		Use:   "enable <installation-id>",
		Short: "Enable an installation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows install enable requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.install.enable", map[string]any{
				"profileId": args[0],
				"enabled":   true,
			})
		},
	}
	return cmd
}

func newFlowsInstallationsDisableCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <installation-id>",
		Short: "Disable an installation (pause)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows install disable requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.install.disable", map[string]any{
				"profileId": args[0],
				"enabled":   false,
			})
		},
	}
	return cmd
}

func newFlowsInstallationsDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <installation-id>",
		Aliases: []string{"uninstall"},
		Short:   "Delete an installation (uninstall)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows install delete requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.install.delete", map[string]any{
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

func parsePositiveIntFlag(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, errors.New("missing numeric value")
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, errors.New("version must be a positive integer or latest")
	}
	return n, nil
}
