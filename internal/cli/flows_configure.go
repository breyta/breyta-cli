package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsConfigureCmd(app *App) *cobra.Command {
	var profileArg string
	var setArgs []string
	var scope string
	cmd := &cobra.Command{
		Use:   "configure <flow-slug> [@profile.edn]",
		Short: "Configure workspace default run target",
		Long: strings.TrimSpace(`
Configure bindings/activation inputs for the workspace default run target.

This is the canonical replacement for draft-binding setup in the happy path.
Use "--scope live" to configure the workspace live target.
Use "flows install configure" when you need installation-specific configuration.
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows configure requires API mode"))
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
			scopeChanged := cmd.Flags().Changed("scope")
			resolvedScope := ""
			if scopeChanged {
				var err error
				resolvedScope, err = normalizeInstallScope(scope)
				if err != nil {
					return writeErr(cmd, err)
				}
			}

			body := map[string]any{
				"flowSlug": args[0],
				"inputs":   map[string]any{},
			}
			if strings.TrimSpace(profileArg) != "" {
				payload, err := parseProfileArg(profileArg)
				if err != nil {
					return writeErr(cmd, err)
				}
				// Canonical configure targets workspace-default profile semantics.
				if !scopeChanged && payload.ProfileType != "" && payload.ProfileType != "draft" {
					return writeErr(cmd, errors.New("profile.type must be draft for flows configure"))
				}
				if scopeChanged && payload.ProfileType == "draft" {
					return writeErr(cmd, errors.New("profile.type draft is not valid with --scope live"))
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
			if scopeChanged {
				if resolvedScope != "live" {
					return writeErr(cmd, errors.New("flows configure currently supports --scope live only"))
				}
				return doAPICommand(cmd, app, "profiles.bindings.apply", body)
			}
			return doAPICommand(cmd, app, "flows.configure", body)
		},
	}
	cmd.Flags().StringVar(&profileArg, "profile", "", "Bindings profile (@profile.edn or inline EDN)")
	cmd.Flags().StringArrayVar(&setArgs, "set", nil, "Set binding or activation input (slot.field=value or activation.field=value)")
	cmd.Flags().StringVar(&scope, "scope", "", "Target scope override (live)")
	cmd.AddCommand(newFlowsConfigureShowCmd(app))
	return cmd
}

func newFlowsConfigureShowCmd(app *App) *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Inspect configured bindings for default or live target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows configure show requires API mode"))
			}

			scopeChanged := cmd.Flags().Changed("scope")
			resolvedScope := ""
			if scopeChanged {
				var err error
				resolvedScope, err = normalizeInstallScope(scope)
				if err != nil {
					return writeErr(cmd, err)
				}
				if resolvedScope != "live" {
					return writeErr(cmd, errors.New("flows configure show currently supports --scope live only"))
				}
			}

			profileType := "draft"
			if scopeChanged {
				profileType = "prod"
			}
			return doAPICommand(cmd, app, "profiles.status", map[string]any{
				"flowSlug":    args[0],
				"profileType": profileType,
			})
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Target scope override (live)")
	return cmd
}
