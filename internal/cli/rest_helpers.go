package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func writeREST(cmd *cobra.Command, app *App, status int, data any) error {
	ok := status < 400
	if !ok {
		if m, _ := data.(map[string]any); m != nil {
			if errAny, exists := m["error"]; exists {
				_ = writeOut(cmd, app, map[string]any{
					"ok":          false,
					"workspaceId": app.WorkspaceID,
					"error":       errAny,
					"meta":        map[string]any{"status": status},
					"data":        m,
				})
				return errors.New("api error")
			}
		}
		_ = writeOut(cmd, app, map[string]any{
			"ok":          false,
			"workspaceId": app.WorkspaceID,
			"data":        data,
			"meta":        map[string]any{"status": status},
		})
		return errors.New("api error")
	}
	return writeOut(cmd, app, map[string]any{
		"ok":          true,
		"workspaceId": app.WorkspaceID,
		"data":        data,
		"meta":        map[string]any{"status": status},
	})
}
