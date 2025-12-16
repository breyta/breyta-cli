package cli

import (
        "context"
        "errors"
        "fmt"
        "strings"

        "breyta-cli/internal/api"

        "github.com/spf13/cobra"
)

func isAPIMode(app *App) bool {
        return strings.TrimSpace(app.APIURL) != ""
}

func requireAPI(app *App) error {
        if strings.TrimSpace(app.APIURL) == "" {
                return errors.New("missing --api or BREYTA_API_URL")
        }
        // In mock-auth mode, any non-blank token works, but we still require callers
        // to be explicit about auth being in play.
        if strings.TrimSpace(app.Token) == "" {
                return errors.New("missing --token or BREYTA_TOKEN")
        }
        return nil
}

func apiClient(app *App) api.Client {
        return api.Client{
                BaseURL:     app.APIURL,
                WorkspaceID: app.WorkspaceID,
                Token:       app.Token,
        }
}

func writeAPIResult(cmd *cobra.Command, app *App, v map[string]any, status int) error {
        _ = writeOut(cmd, app, v)

        // Determine exit code: non-2xx OR ok=false => exit non-zero.
        if status >= 400 {
                return fmt.Errorf("api error (status=%d)", status)
        }
        if okAny, ok := v["ok"]; ok {
                if okb, ok := okAny.(bool); ok && !okb {
                        // Surface the server-provided message if present.
                        if errAny, ok := v["error"]; ok {
                                if em, ok := errAny.(map[string]any); ok {
                                        if msg, _ := em["message"].(string); strings.TrimSpace(msg) != "" {
                                                return errors.New(msg)
                                        }
                                }
                        }
                        return errors.New("command failed")
                }
        }
        return nil
}

func doAPICommand(cmd *cobra.Command, app *App, command string, args map[string]any) error {
        if err := requireAPI(app); err != nil {
                return writeErr(cmd, err)
        }
        client := apiClient(app)
        out, status, err := client.DoCommand(context.Background(), command, args)
        if err != nil {
                return writeErr(cmd, err)
        }
        return writeAPIResult(cmd, app, out, status)
}
