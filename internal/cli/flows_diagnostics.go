package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsDoctorCmd(app *App) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "doctor <slug>",
		Short: "Check compact flow authoring readiness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
				"target":   strings.TrimSpace(target),
			}
			return doAPICommand(cmd, app, "flows.doctor", payload)
		},
	}
	cmd.Flags().StringVar(&target, "target", "draft", "Target to inspect: draft|live")
	return cmd
}

func newFlowsPublicCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "public",
		Short: "Inspect public-flow readiness",
	}
	cmd.AddCommand(newFlowsPublicPreflightCmd(app))
	return cmd
}

func newFlowsPublicPreflightCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preflight <slug>",
		Short: "Check public readiness without changing visibility",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
			}
			return doAPICommand(cmd, app, "flows.public.preflight", payload)
		},
	}
	return cmd
}
