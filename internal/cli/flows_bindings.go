package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsBindingsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bindings",
		Short: "Manage prod bindings",
	}
	cmd.AddCommand(newFlowsBindingsTemplateCmd(app))
	cmd.AddCommand(newFlowsBindingsApplyCmd(app))
	cmd.AddCommand(newFlowsBindingsShowCmd(app))
	return cmd
}

func newFlowsBindingsApplyCmd(app *App) *cobra.Command {
	var profileArg string
	var setArgs []string
	cmd := &cobra.Command{
		Use:   "apply <flow-slug> [@profile.edn]",
		Short: "Apply prod bindings using a profile file",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows bindings apply requires API mode"))
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
			body := map[string]any{
				"flowSlug": args[0],
				"inputs":   map[string]any{},
			}
			if strings.TrimSpace(profileArg) != "" {
				payload, err := parseProfileArg(profileArg)
				if err != nil {
					return writeErr(cmd, err)
				}
				if payload.ProfileType != "" && payload.ProfileType != "prod" {
					return writeErr(cmd, errors.New("profile.type must be prod for prod bindings"))
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
			return doAPICommand(cmd, app, "profiles.bindings.apply", body)
		},
	}
	cmd.Flags().StringVar(&profileArg, "profile", "", "Bindings profile (@profile.edn or inline EDN)")
	cmd.Flags().StringArrayVar(&setArgs, "set", nil, "Set binding or activation input (slot.field=value or activation.field=value)")
	return cmd
}

func newFlowsBindingsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Inspect prod bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows bindings show requires API mode"))
			}
			return doAPICommand(cmd, app, "profiles.status", map[string]any{
				"flowSlug":    args[0],
				"profileType": "prod",
			})
		},
	}
	return cmd
}
