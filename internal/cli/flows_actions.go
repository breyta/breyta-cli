package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsActivateCmd(app *App) *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:    "activate <flow-slug>",
		Short:  "Enable the prod profile for a flow",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows activate requires API mode"))
			}
			body := map[string]any{
				"flowSlug": args[0],
				"version":  strings.TrimSpace(version),
			}
			return doAPICommand(cmd, app, "profiles.activate", body)
		},
	}
	cmd.Flags().StringVar(&version, "version", "latest", "Flow version to activate (number or latest)")
	return cmd
}

func newFlowsDraftBindingsURLCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "draft-bindings-url <flow-slug>",
		Short: "Print the draft setup URL for working-copy runs",
		Long: strings.TrimSpace(`
Working-copy runs use a user-scoped draft profile. Bind credentials here:
- Draft setup: http://localhost:8090/<workspace>/flows/<slug>/draft-bindings
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := strings.TrimRight(app.APIURL, "/")
			if strings.TrimSpace(base) == "" {
				base = "http://localhost:8090"
			}
			url := fmt.Sprintf("%s/%s/flows/%s/draft-bindings", base, app.WorkspaceID, args[0])
			return writeData(cmd, app, nil, map[string]any{
				"workspaceId":      app.WorkspaceID,
				"flowSlug":         args[0],
				"draftBindingsUrl": url,
			})
		},
	}
	return cmd
}

func newFlowsSpineCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "spine <flow-slug>",
		Short:  "Show a flow spine (textual structure)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Local/mock-only command. In API mode use `breyta flows doctor <flow-slug>` for readiness/structure or `breyta flows show <flow-slug> --full` for source.")
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "spine": f.Spine})
		},
	}
	return cmd
}
