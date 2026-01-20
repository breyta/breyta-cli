package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newAppsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apps",
		Short: "End-user app flows (tagged :end-user)",
	}
	cmd.AddCommand(newAppsListCmd(app))
	cmd.AddCommand(newAppsInstancesCmd(app))
	cmd.AddCommand(newAppsRunsCmd(app))
	return cmd
}

func isEndUserTag(tag string) bool {
	t := strings.TrimSpace(tag)
	t = strings.TrimPrefix(t, ":")
	t = strings.ToLower(t)
	return t == "end-user" || t == "end_user"
}

func isEndUserFlowItem(it map[string]any) bool {
	if it == nil {
		return false
	}
	tagsAny, ok := it["tags"]
	if !ok || tagsAny == nil {
		return false
	}
	switch v := tagsAny.(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && isEndUserTag(s) {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if isEndUserTag(s) {
				return true
			}
		}
	}
	return false
}

func newAppsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List end-user apps (flows with :end-user tag)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps list requires API mode"))
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}

			client := apiClient(app)
			resp, status, err := client.DoCommand(context.Background(), "flows.list", map[string]any{})
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 {
				return writeAPIResult(cmd, app, resp, status)
			}

			data, _ := resp["data"].(map[string]any)
			itemsAny, _ := data["items"].([]any)
			items := make([]any, 0, len(itemsAny))
			for _, x := range itemsAny {
				if it, ok := x.(map[string]any); ok && isEndUserFlowItem(it) {
					items = append(items, it)
				}
			}

			meta := map[string]any{
				"total":   len(itemsAny),
				"matched": len(items),
				"hint":    "Use `breyta apps instances create <flow-slug>` to subscribe, then `breyta apps instances enable <profile-id>` to activate.",
			}
			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}
	return cmd
}

func newAppsInstancesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instances",
		Short: "Manage end-user subscription instances (profiles)",
	}
	cmd.AddCommand(newAppsInstancesListCmd(app))
	cmd.AddCommand(newAppsInstancesCreateCmd(app))
	cmd.AddCommand(newAppsInstancesRenameCmd(app))
	cmd.AddCommand(newAppsInstancesSetEnabledCmd(app))
	cmd.AddCommand(newAppsInstancesEnableCmd(app))
	cmd.AddCommand(newAppsInstancesDisableCmd(app))
	return cmd
}

func newAppsInstancesListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List your instances for an end-user flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps instances list requires API mode"))
			}
			return doAPICommand(cmd, app, "apps.instances.list", map[string]any{"flowSlug": args[0]})
		},
	}
	return cmd
}

func newAppsInstancesCreateCmd(app *App) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create <flow-slug>",
		Short: "Subscribe to an end-user flow (creates a disabled instance)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps instances create requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if strings.TrimSpace(name) != "" {
				payload["name"] = strings.TrimSpace(name)
			}
			return doAPICommand(cmd, app, "apps.instances.create", payload)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Instance name (optional)")
	return cmd
}

func newAppsInstancesRenameCmd(app *App) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "rename <profile-id> --name <name>",
		Short: "Rename an instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps instances rename requires API mode"))
			}
			if strings.TrimSpace(name) == "" {
				return writeErr(cmd, errors.New("missing --name"))
			}
			return doAPICommand(cmd, app, "apps.instances.rename", map[string]any{
				"profileId": args[0],
				"name":      strings.TrimSpace(name),
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "New instance name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newAppsInstancesSetEnabledCmd(app *App) *cobra.Command {
	var enabled bool
	cmd := &cobra.Command{
		Use:   "set-enabled <profile-id> --enabled",
		Short: "Toggle an instance enabled state (pause/resume)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps instances set-enabled requires API mode"))
			}
			if !cmd.Flags().Changed("enabled") {
				return writeErr(cmd, errors.New("missing --enabled (true|false)"))
			}
			return doAPICommand(cmd, app, "apps.instances.set_enabled", map[string]any{
				"profileId": args[0],
				"enabled":   enabled,
			})
		},
	}
	cmd.Flags().BoolVar(&enabled, "enabled", false, "Enabled state")
	return cmd
}

func newAppsInstancesEnableCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <profile-id>",
		Short: "Enable an instance (activate)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps instances enable requires API mode"))
			}
			return doAPICommand(cmd, app, "apps.instances.set_enabled", map[string]any{
				"profileId": args[0],
				"enabled":   true,
			})
		},
	}
	return cmd
}

func newAppsInstancesDisableCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <profile-id>",
		Short: "Disable an instance (pause)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps instances disable requires API mode"))
			}
			return doAPICommand(cmd, app, "apps.instances.set_enabled", map[string]any{
				"profileId": args[0],
				"enabled":   false,
			})
		},
	}
	return cmd
}

func newAppsRunsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "Runs for end-user apps (instances)",
	}
	cmd.AddCommand(newAppsRunsListCmd(app))
	cmd.AddCommand(newAppsRunsStartCmd(app))
	return cmd
}

func newAppsRunsListCmd(app *App) *cobra.Command {
	var profileID string
	var limit int
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List runs for an app instance (by profile-id)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps runs list requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if strings.TrimSpace(profileID) != "" {
				payload["profileId"] = strings.TrimSpace(profileID)
			}
			if limit > 0 {
				payload["limit"] = limit
			}
			return doAPICommand(cmd, app, "runs.list", payload)
		},
	}
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Filter by instance profile id")
	cmd.Flags().IntVar(&limit, "limit", 25, "Limit results")
	return cmd
}

func newAppsRunsStartCmd(app *App) *cobra.Command {
	var (
		profileID string
		inputJSON string
	)
	cmd := &cobra.Command{
		Use:   "start <flow-slug> --profile-id <profile-id> [--input '{...}']",
		Short: "Start a run for an app instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("apps runs start requires API mode"))
			}
			if strings.TrimSpace(profileID) == "" {
				return writeErr(cmd, errors.New("missing --profile-id"))
			}
			payload := map[string]any{
				"flowSlug":  args[0],
				"profileId": strings.TrimSpace(profileID),
				"source":    "active",
			}
			if strings.TrimSpace(inputJSON) != "" {
				m, err := parseJSONObjectFlag(inputJSON)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["input"] = m
			}
			return doAPICommand(cmd, app, "runs.start", payload)
		},
	}
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Instance profile id")
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object input")
	_ = cmd.MarkFlagRequired("profile-id")
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
