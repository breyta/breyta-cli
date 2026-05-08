package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsInstallationsInterfacesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "interfaces <installation-id>",
		Short: "List interfaces for an installation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, status, flow, flowSlug, err := fetchInstallationInterfaceMetadata(cmd.Context(), app, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				enrichFlowInterfaceFailure(resp, flowSlug, args[0], "")
				return writeAPIResult(cmd, app, resp, status)
			}
			items := withFlowInterfaceEndpointMetadata(app, flowInterfaceItems(flow, "installation"), flowSlug, args[0], "installation")
			out := map[string]any{
				"ok":          true,
				"workspaceId": workspaceIDFromEnvelope(resp, app.WorkspaceID),
				"meta": pruneEmptyStrings(map[string]any{
					"target": "installation",
					"count":  len(items),
				}),
				"data": pruneEmptyStrings(map[string]any{
					"flowSlug":       flowSlug,
					"target":         "installation",
					"installationId": args[0],
					"items":          items,
				}),
			}
			return writeAPIResult(cmd, app, out, 200)
		},
	}
	return cmd
}

func fetchInstallationInterfaceMetadata(ctx context.Context, app *App, installationID string) (map[string]any, int, map[string]any, string, error) {
	if !isAPIMode(app) {
		return nil, 0, nil, "", errors.New("flows installations interfaces requires --api/BREYTA_API_URL")
	}
	if err := requireAPI(app); err != nil {
		return nil, 0, nil, "", err
	}
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return nil, 0, nil, "", errors.New("installation id is required")
	}
	resp, status, err := runAPICommandWithContext(ctx, app, "flows.installations.get", map[string]any{"profileId": installationID})
	if err != nil {
		return nil, 0, nil, "", err
	}
	if status >= 400 || !isOK(resp) {
		return resp, status, nil, "", nil
	}
	data := mapStringAny(resp["data"])
	flowSlug := firstNonBlankString(data["flowSlug"], data["flow-slug"])
	if flowSlug == "" {
		return nil, 0, nil, "", errors.New("installation response is missing flowSlug")
	}
	payload := map[string]any{
		"flowSlug":           flowSlug,
		"source":             "active",
		"includeFlowLiteral": false,
	}
	if resolvedVersion := firstPositiveInt(data["version"], data["installedVersion"], data["installed-version"]); resolvedVersion > 0 {
		payload["version"] = resolvedVersion
	}
	resp, status, err = runAPICommandWithContext(ctx, app, "flows.get", payload)
	if err != nil {
		return nil, 0, nil, "", err
	}
	flow := mapStringAny(mapStringAny(resp["data"])["flow"])
	return resp, status, flow, flowSlug, nil
}
