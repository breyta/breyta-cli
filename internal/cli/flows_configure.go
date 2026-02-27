package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsConfigureCmd(app *App) *cobra.Command {
	var profileArg string
	var setArgs []string
	var target string
	var version string
	var fromDraft bool
	cmd := &cobra.Command{
		Use:   "configure <flow-slug> [@profile.edn]",
		Short: "Configure workspace default run target",
		Long: strings.TrimSpace(`
Configure bindings/activation inputs for the workspace default run target.

This is the canonical command for workspace-default flow configuration.
Use "--target draft|live" to choose draft (default) or live target.
Use "--target live --from-draft" to promote draft bindings to live in one step.
Use "flows configure check <flow-slug>" to verify required config before running.
Use "flows installations configure" when you need installation-specific configuration.
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows configure requires API mode"))
			}
			targetChanged := cmd.Flags().Changed("target")
			resolvedTarget := ""
			if targetChanged {
				var err error
				resolvedTarget, err = normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
			}
			versionChanged := cmd.Flags().Changed("version")
			versionValue := strings.TrimSpace(version)

			body := map[string]any{
				"flowSlug": args[0],
			}
			if targetChanged {
				body["target"] = resolvedTarget
			}
			if versionChanged && (!targetChanged || resolvedTarget != "live") {
				return writeErr(cmd, errors.New("--version requires --target live"))
			}
			if versionChanged {
				if versionValue == "" {
					return writeErr(cmd, errors.New("--version must be a positive integer or latest"))
				}
				if strings.EqualFold(versionValue, "latest") {
					body["version"] = "latest"
				} else {
					v, err := parsePositiveIntFlag(versionValue)
					if err != nil {
						return writeErr(cmd, err)
					}
					body["version"] = v
				}
			}

			if fromDraft {
				if !targetChanged || resolvedTarget != "live" {
					return writeErr(cmd, errors.New("--from-draft requires --target live"))
				}
				if len(args) == 2 || strings.TrimSpace(profileArg) != "" || len(setArgs) > 0 {
					return writeErr(cmd, errors.New("--from-draft cannot be used with a profile file or --set"))
				}
				body["fromDraft"] = true
				return doAPICommand(cmd, app, "flows.configure", body)
			}

			if len(args) == 2 {
				if strings.TrimSpace(profileArg) != "" {
					return writeErr(cmd, errors.New("provide @profile.edn or --profile, not both"))
				}
				profileArg = args[1]
			}
			if strings.TrimSpace(profileArg) == "" && len(setArgs) == 0 {
				return writeErr(cmd, errors.New("missing profile file or --set (use @profile.edn, --profile, or --set)"))
			}

			body["inputs"] = map[string]any{}
			if strings.TrimSpace(profileArg) != "" {
				payload, err := parseProfileArg(profileArg)
				if err != nil {
					return writeErr(cmd, err)
				}
				targetsLive := targetChanged && resolvedTarget == "live"
				targetsCurrent := !targetsLive
				// Current/default configure targets workspace-default profile semantics.
				if targetsCurrent && payload.ProfileType != "" && payload.ProfileType != "draft" {
					return writeErr(cmd, errors.New("profile.type is not supported for default configure target"))
				}
				if targetsLive && payload.ProfileType == "draft" {
					return writeErr(cmd, errors.New("profile.type is not supported with --target live"))
				}
				body["inputs"] = payload.Inputs
			}
			if len(setArgs) > 0 {
				setInputs, err := parseSetAssignments(setArgs)
				if err != nil {
					return writeErr(cmd, err)
				}
				inputs := body["inputs"].(map[string]any)
				for k, v := range setInputs {
					inputs[k] = v
				}
			}
			return doAPICommand(cmd, app, "flows.configure", body)
		},
	}
	cmd.Flags().StringVar(&profileArg, "profile", "", "Bindings profile (@profile.edn or inline EDN)")
	cmd.Flags().StringArrayVar(&setArgs, "set", nil, "Set binding or activation input (slot.field=value or activation.field=value)")
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live)")
	cmd.Flags().StringVar(&version, "version", "", "Flow version override for live target (positive integer or latest)")
	cmd.Flags().BoolVar(&fromDraft, "from-draft", false, "Promote current draft bindings to live target")
	cmd.AddCommand(newFlowsConfigureShowCmd(app))
	cmd.AddCommand(newFlowsConfigureCheckCmd(app))
	cmd.AddCommand(newFlowsConfigureSuggestCmd(app))
	return cmd
}

func newFlowsConfigureShowCmd(app *App) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Inspect configured bindings for default or live target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows configure show requires API mode"))
			}

			targetChanged := cmd.Flags().Changed("target")
			resolvedTarget := ""
			if targetChanged {
				var err error
				resolvedTarget, err = normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
			}

			profileType := "draft"
			if targetChanged && resolvedTarget == "live" {
				profileType = "prod"
			}
			return doAPICommand(cmd, app, "profiles.status", map[string]any{
				"flowSlug":    args[0],
				"profileType": profileType,
			})
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live)")
	return cmd
}

func newFlowsConfigureCheckCmd(app *App) *cobra.Command {
	var target string
	var version string
	cmd := &cobra.Command{
		Use:   "check <flow-slug>",
		Short: "Check whether required configuration is complete",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows configure check requires API mode"))
			}
			payload := map[string]any{
				"flowSlug": args[0],
			}
			if cmd.Flags().Changed("target") {
				resolvedTarget, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["target"] = resolvedTarget
			}
			if cmd.Flags().Changed("version") {
				versionValue := strings.TrimSpace(version)
				if versionValue == "" {
					return writeErr(cmd, errors.New("--version must be a positive integer or latest"))
				}
				if strings.EqualFold(versionValue, "latest") {
					payload["version"] = "latest"
				} else {
					v, err := parsePositiveIntFlag(versionValue)
					if err != nil {
						return writeErr(cmd, err)
					}
					payload["version"] = v
				}
			}
			return doAPICommand(cmd, app, "flows.configure.check", payload)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live)")
	cmd.Flags().StringVar(&version, "version", "", "Flow version override for live target (positive integer or latest)")
	return cmd
}
