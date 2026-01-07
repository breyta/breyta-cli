package cli

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "io"
        "net/http"
        "os"
        "strings"
        "time"

        "breyta-cli/internal/api"

        "github.com/spf13/cobra"
)

func requireAPIBase(app *App) error {
        if strings.TrimSpace(app.APIURL) == "" {
                return errors.New("missing --api or BREYTA_API_URL")
        }
        return nil
}

func authClient(app *App) api.Client {
        return api.Client{
                BaseURL:     app.APIURL,
                WorkspaceID: app.WorkspaceID,
                Token:       app.Token,
        }
}

func shellExportTokenLine(token string) string {
        // Firebase ID tokens are base64url-ish and should not contain single quotes.
        // Still, be defensive to avoid producing unsafe shell output.
        if strings.Contains(token, "'") {
                return ""
        }
        return "export BREYTA_TOKEN='" + token + "'"
}

func newAuthCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "auth", Short: "Authenticate"}
        cmd.AddCommand(newAuthWhoamiCmd(app))
        cmd.AddCommand(newAuthLoginCmd(app))
        cmd.AddCommand(newAuthLogoutCmd(app))
        return cmd
}

func newAuthWhoamiCmd(app *App) *cobra.Command {
        return &cobra.Command{
                Use:   "whoami",
                Short: "Show identity for the current token",
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
                        defer cancel()

                        out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/auth/verify", nil, nil)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeData(cmd, app, map[string]any{"httpStatus": status}, map[string]any{"verify": out})
                },
        }
}

func newAuthLoginCmd(app *App) *cobra.Command {
        var email string
        var password string
        var passwordStdin bool
        var printMode string

        cmd := &cobra.Command{
                Use:   "login",
                Short: "Login via flows-api and print a token",
                Long: strings.TrimSpace(`
Login by exchanging email+password for a Firebase ID token via flows-api (/api/auth/token).

This does NOT persist credentials. Use the output to set BREYTA_TOKEN.
`),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPIBase(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        email = strings.TrimSpace(email)
                        if email == "" {
                                return writeErr(cmd, errors.New("missing --email"))
                        }

                        if passwordStdin {
                                b, err := io.ReadAll(cmd.InOrStdin())
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                password = strings.TrimSpace(string(b))
                        }
                        if strings.TrimSpace(password) == "" {
                                return writeErr(cmd, errors.New("missing --password (or use --password-stdin)"))
                        }

                        ctx, cancel := context.WithTimeout(cmd.Context(), 25*time.Second)
                        defer cancel()

                        client := authClient(app)
                        client.Token = ""

                        out, status, err := client.DoRootREST(ctx, http.MethodPost, "/api/auth/token", nil, map[string]any{
                                "email":    email,
                                "password": password,
                        })
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        // Clear password as soon as we can (best-effort).
                        password = ""

                        m, ok := out.(map[string]any)
                        if !ok {
                                return writeFailure(cmd, app, "auth_login_unexpected_response", fmt.Errorf("unexpected response (status=%d)", status), "Expected JSON object from /api/auth/token", out)
                        }
                        if success, _ := m["success"].(bool); !success {
                                msg, _ := m["error"].(string)
                                if strings.TrimSpace(msg) == "" {
                                        msg = "login failed"
                                }
                                return writeFailure(cmd, app, "auth_login_failed", fmt.Errorf("%s (status=%d)", msg, status), "Check email/password and server config (FIREBASE_WEB_API_KEY, Email/Password provider enabled).", m)
                        }
                        token, _ := m["token"].(string)
                        token = strings.TrimSpace(token)
                        if token == "" {
                                return writeFailure(cmd, app, "auth_login_missing_token", fmt.Errorf("missing token in response (status=%d)", status), "Server returned success but no token.", m)
                        }

                        switch printMode {
                        case "token":
                                fmt.Fprintln(cmd.OutOrStdout(), token)
                                return nil
                        case "export":
                                line := shellExportTokenLine(token)
                                if line == "" {
                                        return writeFailure(cmd, app, "auth_login_shell_export_unsafe", errors.New("cannot render safe shell export"), "Token contained unexpected characters; use --print token or --format json.", nil)
                                }
                                fmt.Fprintln(cmd.OutOrStdout(), line)
                                return nil
                        default:
                                meta := map[string]any{
                                        "httpStatus": status,
                                }
                                if line := shellExportTokenLine(token); line != "" {
                                        meta["export"] = line
                                        meta["hint"] = "Set BREYTA_TOKEN in your shell, then run breyta commands in API mode."
                                }
                                return writeData(cmd, app, meta, map[string]any{"token": token, "uid": m["uid"], "expiresIn": m["expiresIn"]})
                        }
                },
        }

        cmd.Flags().StringVar(&email, "email", envOr("BREYTA_EMAIL", ""), "Email address")
        cmd.Flags().StringVar(&password, "password", envOr("BREYTA_PASSWORD", ""), "Password (use --password-stdin to avoid shell history)")
        cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read password from stdin")
        cmd.Flags().StringVar(&printMode, "print", envOr("BREYTA_AUTH_PRINT", "json"), "Output mode: json|token|export")

        _ = cmd.MarkFlagRequired("email")
        return cmd
}

func newAuthLogoutCmd(app *App) *cobra.Command {
        return &cobra.Command{
                Use:   "logout",
                Short: "Logout (local-only)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        // There is no durable local auth store yet; we just guide the user.
                        meta := map[string]any{
                                "hint": "Unset BREYTA_TOKEN (and any shell exports) to 'log out' locally.",
                        }
                        return writeData(cmd, app, meta, map[string]any{"tokenPresent": strings.TrimSpace(app.Token) != ""})
                },
        }
}

// Ensure any accidental debug output does not leak sensitive values in JSON mode.
// This is a no-op unless someone sets BREYTA_AUTH_DEBUG=1 while developing.
func maybeAuthDebug(v any) {
        if os.Getenv("BREYTA_AUTH_DEBUG") != "1" {
                return
        }
        _ = json.NewEncoder(io.Discard).Encode(v)
}
