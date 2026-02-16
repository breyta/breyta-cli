package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsConfigureCmd(app *App) *cobra.Command {
	var profileArg string
	var setArgs []string
	cmd := &cobra.Command{
		Use:   "configure <flow-slug> [@profile.edn]",
		Short: "Configure workspace default run target",
		Long: strings.TrimSpace(`
Configure bindings/activation inputs for the workspace default run target.

This is the canonical replacement for draft-binding setup in the happy path.
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
				if payload.ProfileType != "" && payload.ProfileType != "draft" {
					return writeErr(cmd, errors.New("profile.type must be draft for flows configure"))
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
	return cmd
}
