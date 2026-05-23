package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsInstallationsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "installations",
		Short: "Manage flow installations",
		Long: strings.TrimSpace(`
Advanced runtime targeting and rollout controls.

Most users should use:
- breyta flows run <flow-slug>

Use install commands only when you need explicit targets, installation-specific
config/triggers, or controlled promotion.
`),
	}
	cmd.AddCommand(newFlowsInstallationsListCmd(app))
	cmd.AddCommand(newFlowsInstallationsStatsCmd(app))
	cmd.AddCommand(newFlowsInstallationsEventsCmd(app))
	cmd.AddCommand(newFlowsInstallationsCreateCmd(app))
	cmd.AddCommand(newFlowsInstallationsGetCmd(app))
	cmd.AddCommand(newFlowsInstallationsRenameCmd(app))
	cmd.AddCommand(newFlowsInstallationsSetInputsCmd(app))
	cmd.AddCommand(newFlowsInstallationsSetEnabledCmd(app))
	cmd.AddCommand(newFlowsInstallationsEnableCmd(app))
	cmd.AddCommand(newFlowsInstallationsDisableCmd(app))
	cmd.AddCommand(newFlowsInstallationsDeleteCmd(app))
	cmd.AddCommand(newFlowsInstallationsTriggersCmd(app))
	cmd.AddCommand(newFlowsInstallationsInterfacesCmd(app))
	cmd.AddCommand(newFlowsInstallationsUploadCmd(app))
	return cmd
}

func newFlowsInstallationsListCmd(app *App) *cobra.Command {
	var all bool
	var sourceWorkspaceID string
	var sourceFlowSlug string
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List installations for a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations list requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if all {
				payload["all"] = true
			}
			if strings.TrimSpace(sourceWorkspaceID) != "" {
				payload["sourceWorkspaceId"] = strings.TrimSpace(sourceWorkspaceID)
			}
			if strings.TrimSpace(sourceFlowSlug) != "" {
				payload["sourceFlowSlug"] = strings.TrimSpace(sourceFlowSlug)
			}
			return doAPICommand(cmd, app, "flows.installations.list", payload)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "List all installations for the flow (creator-only)")
	cmd.Flags().StringVar(&sourceWorkspaceID, "source-workspace-id", "", "Public-install source workspace id for cross-workspace listing")
	cmd.Flags().StringVar(&sourceFlowSlug, "source-flow-slug", "", "Public-install source flow slug override")
	return cmd
}

func newFlowsInstallationsStatsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats <flow-slug>",
		Short: "Show creator install and subscriber stats for a public flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations stats requires API mode"))
			}
			return doAPICommand(cmd, app, "flows.installations.stats", map[string]any{
				"flowSlug": args[0],
			})
		},
	}
	return cmd
}

func newFlowsInstallationsEventsCmd(app *App) *cobra.Command {
	var limit int
	var since string
	cmd := &cobra.Command{
		Use:   "events <flow-slug>",
		Short: "Show recent creator install state events for a public flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations events requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if cmd.Flags().Changed("limit") {
				payload["limit"] = limit
			}
			if strings.TrimSpace(since) != "" {
				payload["since"] = strings.TrimSpace(since)
			}
			return doAPICommand(cmd, app, "flows.installations.events", payload)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum events to return")
	cmd.Flags().StringVar(&since, "since", "", "Only include events since a relative value like 7d or an ISO timestamp")
	return cmd
}

func newFlowsInstallationsCreateCmd(app *App) *cobra.Command {
	var name string
	var sourceWorkspaceID string
	var sourceFlowSlug string
	var enable bool
	var localPrivateTest bool
	cmd := &cobra.Command{
		Use:   "create <flow-slug>",
		Short: "Create a new installation",
		Long: strings.TrimSpace(`
Create an installation for a flow that has an active live version.

For a public flow returned by ` + "`breyta flows discover list`" + ` or
` + "`breyta flows discover search <query>`" + `, pass the listed flow slug:
- breyta flows installations create <flow-slug> --name "Smoke install"

If more than one public source uses the same slug, pass the source fields from
Discover:
- breyta flows installations create <flow-slug> --source-workspace-id <workspace-id> --source-flow-slug <flow-slug>

Before creating an installation for your own workspace flow, check the source with:
- breyta flows release-check <flow-slug>
- breyta flows release <flow-slug>

Creation auto-enables zero-setup installations. Installations that still need
setup are created disabled until configured. Use --enable only when you want to
state the enable intent explicitly.

For paid public apps, use the Discover/public app checkout surface first when
purchase, subscription, prepaid runs, or trial entry is required. After checkout
or trial entry, create/configure/enable the installation here, inspect callable
surfaces with ` + "`breyta flows installations interfaces <installation-id>`" + `, then call
installed interfaces with ` + "`breyta flows interfaces call ... --installation-id <installation-id>`" + `.
If creation returns a checkout action, open that URL and retry after checkout.

Local private cross-workspace tests require --local-private-test plus
--source-workspace-id, --source-flow-slug, and an active source version.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations create requires API mode"))
			}
			payload := map[string]any{"flowSlug": args[0]}
			if strings.TrimSpace(name) != "" {
				payload["name"] = strings.TrimSpace(name)
			}
			if strings.TrimSpace(sourceWorkspaceID) != "" {
				payload["sourceWorkspaceId"] = strings.TrimSpace(sourceWorkspaceID)
			}
			if strings.TrimSpace(sourceFlowSlug) != "" {
				payload["sourceFlowSlug"] = strings.TrimSpace(sourceFlowSlug)
			}
			if enable {
				payload["enabled"] = true
			}
			if localPrivateTest {
				payload["localPrivateTest"] = true
			}
			return doAPICommand(cmd, app, "flows.installations.create", payload)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Installation name (optional)")
	cmd.Flags().StringVar(&sourceWorkspaceID, "source-workspace-id", "", "Source workspace id for cross-workspace installs")
	cmd.Flags().StringVar(&sourceFlowSlug, "source-flow-slug", "", "Source flow slug override for cross-workspace installs")
	cmd.Flags().BoolVar(&enable, "enable", false, "Explicitly request enabled state after create; zero-setup installs enable by default")
	cmd.Flags().BoolVar(&localPrivateTest, "local-private-test", false, "Local dev only: test a private cross-workspace source flow; requires source ids and an active source version")
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
	var bindingsJSON string
	var schedulesJSON string
	var scheduleEnableItems []string
	var scheduleDisableItems []string
	var scheduleResetItems []string
	var setItems []string
	cmd := &cobra.Command{
		Use:     "configure <installation-id> [--input '{...}'] [--bindings '{...}'] [--schedules '{...}'] [--schedule-enable <trigger-id>] [--schedule-disable <trigger-id>] [--set activation.<field>=... --set <slot>.conn=... --set <slot>.root=...]",
		Aliases: []string{"set-inputs"},
		Short:   "Configure installation setup inputs, schedules, and installer-owned bindings",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations configure requires API mode"))
			}
			if strings.TrimSpace(inputJSON) == "" && strings.TrimSpace(bindingsJSON) == "" && strings.TrimSpace(schedulesJSON) == "" && len(scheduleEnableItems) == 0 && len(scheduleDisableItems) == 0 && len(scheduleResetItems) == 0 && len(setItems) == 0 {
				return writeErr(cmd, errors.New("missing configuration updates (use --input, --bindings, --schedules, --schedule-enable, --schedule-disable, --schedule-reset, or --set)"))
			}
			inputs, err := parseJSONObjectFlag(inputJSON)
			if err != nil {
				return writeErr(cmd, err)
			}
			bindings, err := parseJSONObjectFlag(bindingsJSON)
			if err != nil {
				return writeErr(cmd, err)
			}
			schedules, err := parseJSONObjectFlag(schedulesJSON)
			if err != nil {
				return writeErr(cmd, err)
			}
			schedules, err = mergeScheduleEnabledFlags(schedules, scheduleEnableItems, scheduleDisableItems)
			if err != nil {
				return writeErr(cmd, err)
			}
			setPayload, err := parseInstallationSetAssignments(setItems)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"profileId": args[0],
				"inputs":    mergeJSONObjectFlags(inputs, setPayload.Inputs),
			}
			if mergedBindings := mergeJSONObjectFlags(bindings, setPayload.Bindings); len(mergedBindings) > 0 {
				payload["bindings"] = mergedBindings
			}
			if len(schedules) > 0 || strings.TrimSpace(schedulesJSON) != "" {
				payload["schedules"] = schedules
			}
			if len(scheduleResetItems) > 0 {
				payload["scheduleResets"] = scheduleResetMap(scheduleResetItems)
			}
			return doAPICommand(cmd, app, "flows.installations.set_inputs", payload)
		},
	}
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object of activation inputs")
	cmd.Flags().StringVar(&bindingsJSON, "bindings", "", "JSON object of installer-owned binding updates")
	cmd.Flags().StringVar(&schedulesJSON, "schedules", "", "JSON object of installation schedule settings")
	cmd.Flags().StringArrayVar(&scheduleEnableItems, "schedule-enable", nil, "Enable one installation schedule trigger")
	cmd.Flags().StringArrayVar(&scheduleDisableItems, "schedule-disable", nil, "Disable one installation schedule trigger")
	cmd.Flags().StringArrayVar(&scheduleResetItems, "schedule-reset", nil, "Reset a schedule trigger override to the flow default")
	cmd.Flags().StringArrayVar(&setItems, "set", nil, "Set installation setup values, e.g. activation.folder=https://... or archive.root=customer-a")
	return cmd
}

func normalizeInstallTarget(target string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(target))
	if s == "" {
		return "draft", nil
	}
	switch s {
	case "draft", "live":
		return s, nil
	default:
		return "", errors.New("invalid --target (expected draft or live)")
	}
}

func normalizePromoteScope(scope string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(scope))
	if s == "" {
		return "", nil
	}
	switch s {
	case "all", "live":
		return s, nil
	default:
		return "", errors.New("invalid --scope (expected all or live)")
	}
}

func newFlowsPromoteCmd(app *App) *cobra.Command {
	var version string
	var scope string
	var policy string
	cmd := &cobra.Command{
		Use:   "promote <flow-slug>",
		Short: "Promote a released version to live and all track-latest installations in this workspace",
		Long: strings.TrimSpace(`
Promote a released version to the live target in the current workspace.
By default, this also updates all track-latest end-user installations for the flow.

Most users run workspace-draft by default with:
- breyta flows run <flow-slug>

After promote, verify the live runtime explicitly with flows show and a live smoke run.
`),
		Example: strings.TrimSpace(`
breyta flows promote order-ingest
breyta flows promote order-ingest --version 42
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows promote requires API mode"))
			}
			resolvedScope, err := normalizePromoteScope(scope)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"flowSlug": args[0],
				"target":   "live",
			}
			if strings.TrimSpace(version) != "" && strings.TrimSpace(version) != "latest" {
				v, err := parsePositiveIntFlag(version)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["version"] = v
			}
			if resolvedScope != "" {
				payload["scope"] = resolvedScope
			}
			policy = strings.ToLower(strings.TrimSpace(policy))
			if policy != "" {
				if policy != "track-latest" && policy != "pinned" {
					return writeErr(cmd, errors.New("invalid --policy (expected pinned or track-latest)"))
				}
				payload["policy"] = policy
			}
			return doAPICommand(cmd, app, "flows.promote", payload)
		},
	}
	cmd.Flags().StringVar(&version, "version", "latest", "Release version to promote (or latest)")
	cmd.Flags().StringVar(&scope, "scope", "", "Advanced: promotion scope override (all|live). Default all")
	cmd.Flags().StringVar(&policy, "policy", "", "Advanced: installation policy override (pinned|track-latest)")
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
				return writeErr(cmd, errors.New("flows installations set-enabled requires API mode"))
			}
			if !cmd.Flags().Changed("enabled") {
				return writeErr(cmd, errors.New("missing --enabled (true|false)"))
			}
			command := "flows.installations.set_enabled"
			if enabled {
				command = "flows.installations.set_enabled"
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
		Use:   "disable <installation-id>",
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
		Use:     "delete <installation-id>",
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

func scheduleResetMap(items []string) map[string]any {
	out := map[string]any{}
	for _, item := range items {
		key := strings.TrimSpace(strings.TrimPrefix(item, ":"))
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func mergeScheduleEnabledFlags(schedules map[string]any, enableItems []string, disableItems []string) (map[string]any, error) {
	out := mergeJSONObjectFlags(map[string]any{}, schedules)
	seen := map[string]bool{}
	for _, item := range enableItems {
		key := strings.TrimSpace(strings.TrimPrefix(item, ":"))
		if key == "" {
			continue
		}
		seen[key] = true
		if err := setScheduleEnabled(out, key, true); err != nil {
			return nil, err
		}
	}
	for _, item := range disableItems {
		key := strings.TrimSpace(strings.TrimPrefix(item, ":"))
		if key == "" {
			continue
		}
		if seen[key] {
			return nil, fmt.Errorf("schedule %q cannot be both enabled and disabled", key)
		}
		if err := setScheduleEnabled(out, key, false); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func setScheduleEnabled(schedules map[string]any, key string, enabled bool) error {
	existing, ok := schedules[key]
	if !ok {
		schedules[key] = map[string]any{"enabled": enabled}
		return nil
	}
	setting, ok := existing.(map[string]any)
	if !ok {
		return fmt.Errorf("schedule %q must be a JSON object", key)
	}
	setting["enabled"] = enabled
	return nil
}

func mergeJSONObjectFlags(base map[string]any, overlay map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overlay {
		existing, hasExisting := out[key]
		existingMap, existingOK := existing.(map[string]any)
		incomingMap, incomingOK := value.(map[string]any)
		if hasExisting && existingOK && incomingOK {
			out[key] = mergeJSONObjectFlags(existingMap, incomingMap)
			continue
		}
		out[key] = value
	}
	return out
}

type installationSetPayload struct {
	Inputs   map[string]any
	Bindings map[string]any
}

func parseInstallationSetAssignments(items []string) (*installationSetPayload, error) {
	payload := &installationSetPayload{
		Inputs:   map[string]any{},
		Bindings: map[string]any{},
	}
	for _, item := range items {
		raw := strings.TrimSpace(item)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --set %q (expected key=value)", raw)
		}
		key := strings.TrimSpace(parts[0])
		valRaw := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid --set %q (empty key)", raw)
		}
		val, err := parseSetValue(valRaw)
		if err != nil {
			return nil, err
		}
		if isPlaceholder(val) {
			continue
		}
		if strings.HasPrefix(key, "activation.") {
			field := strings.TrimSpace(strings.TrimPrefix(key, "activation."))
			if field == "" {
				return nil, fmt.Errorf("invalid --set %q (missing activation field)", raw)
			}
			payload.Inputs[field] = val
			continue
		}
		slot, field, ok := strings.Cut(key, ".")
		if !ok {
			return nil, fmt.Errorf("invalid --set %q (use activation.<field> or <slot>.<field>)", raw)
		}
		slot = strings.TrimSpace(slot)
		field = strings.TrimSpace(field)
		if slot == "" || field == "" {
			return nil, fmt.Errorf("invalid --set %q (empty slot or field)", raw)
		}
		normalizedField := strings.ToLower(field)
		slotBinding, _ := payload.Bindings[slot].(map[string]any)
		if slotBinding == nil {
			slotBinding = map[string]any{}
			payload.Bindings[slot] = slotBinding
		}
		switch normalizedField {
		case "conn", "connection", "connection-id", "connectionid", "id":
			slotBinding["connectionId"] = val
		case "root", "prefix":
			config, _ := slotBinding["config"].(map[string]any)
			if config == nil {
				config = map[string]any{}
				slotBinding["config"] = config
			}
			if normalizedField == "prefix" {
				config["prefix"] = val
			} else {
				config["root"] = val
			}
		default:
			return nil, fmt.Errorf("invalid --set %q (unsupported installation binding field %q; use conn, root, or prefix)", raw, field)
		}
	}
	return payload, nil
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
