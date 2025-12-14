package cli

import (
        "errors"

        "github.com/spf13/cobra"
)

func writeData(cmd *cobra.Command, app *App, meta map[string]any, data any) error {
        out := map[string]any{
                "ok":          true,
                "workspaceId": app.WorkspaceID,
                "meta":        meta,
                "data":        data,
        }
        // Avoid emitting empty meta.
        if meta == nil {
                delete(out, "meta")
        }
        return writeOut(cmd, app, out)
}

func writeFailure(cmd *cobra.Command, app *App, code string, err error, hint string, details any) error {
        if err == nil {
                err = errors.New("unknown error")
        }
        out := map[string]any{
                "ok":          false,
                "workspaceId": app.WorkspaceID,
                "error": map[string]any{
                        "code":    code,
                        "message": err.Error(),
                        "details": details,
                },
                "hint": hint,
        }
        // We still return an error so Cobra exits non-zero.
        _ = writeOut(cmd, app, out)
        return err
}

func writeNotImplemented(cmd *cobra.Command, app *App, hint string) error {
        return writeFailure(cmd, app, "not_implemented", errors.New("not implemented"), hint, nil)
}
