package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsInstallationsSurfacesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "surfaces <installation-id>",
		Short: "List installer-facing email addresses and endpoints",
		Long: strings.TrimSpace(`
List the durable surfaces for an installed app.

This includes generated inbound email addresses, HTTP endpoints, webhook paths,
and MCP transport metadata when the installed flow exposes them.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations surfaces requires API mode"))
			}
			installationID := strings.TrimSpace(args[0])
			if installationID == "" {
				return writeErr(cmd, errors.New("installation id is required"))
			}
			if resp, status, err := runAPICommandWithContext(cmd.Context(), app, "flows.installations.get", map[string]any{"profileId": installationID}); err == nil && status < 400 && isOK(resp) {
				if out, ok := installationSurfacesEnvelopeFromGet(app, resp, installationID); ok {
					return writeAPIResult(cmd, app, out, 200)
				}
			}
			return doAPICommand(cmd, app, "flows.installations.surfaces.list", map[string]any{
				"profileId": installationID,
			})
		},
	}
	return cmd
}

func installationSurfacesEnvelopeFromGet(app *App, resp map[string]any, installationID string) (map[string]any, bool) {
	data := mapStringAny(resp["data"])
	surfaces := mapStringAny(data["surfaces"])
	items, ok := surfaces["items"].([]any)
	if !ok {
		return nil, false
	}
	profileID := firstNonBlankString(data["profileId"], data["profile-id"], data["installationId"], data["installation-id"], installationID)
	flowSlug := firstNonBlankString(data["sourceFlowSlug"], data["source-flow-slug"], data["flowSlug"], data["flow-slug"])
	outData := map[string]any{
		"profileId":      profileID,
		"installationId": profileID,
		"flowSlug":       flowSlug,
		"items":          items,
		"nextCursor":     surfaces["nextCursor"],
		"hasMore":        false,
	}
	if hasMore, ok := surfaces["hasMore"].(bool); ok {
		outData["hasMore"] = hasMore
	}
	if nextCursor := firstNonBlankString(surfaces["nextCursor"], surfaces["next-cursor"]); nextCursor != "" {
		outData["nextCursor"] = nextCursor
	}
	return map[string]any{
		"ok":          true,
		"workspaceId": workspaceIDFromEnvelope(resp, app.WorkspaceID),
		"data":        pruneEmptyStrings(outData),
	}, true
}
