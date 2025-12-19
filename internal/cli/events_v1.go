package cli

import (
        "context"
        "encoding/json"
        "errors"
        "net/http"
        "net/url"
        "strings"

        "github.com/spf13/cobra"
)

func newEventsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "events",
                Short: "Post incoming events/webhooks",
                Long: strings.TrimSpace(`
Events are the public webhook-style entry point for event triggers.

API route:
  POST /<workspace>/events/<event-path>

This endpoint can do two things:
- If <event-path> matches an active wait's webhook path, it completes the wait.
- Otherwise, if <event-path> matches an event trigger's webhook path, it starts a new flow run.

Examples:
  breyta events post demo-hook --payload '{"hello":"world"}'
  breyta events post "webhooks/my-flow/ping" --payload '{"ok":true}'
`),
                // This is intentionally a dev-only command:
                // - It wraps a semi-public webhook endpoint (/events/*).
                // - It's primarily for local/dev testing (agents) to avoid raw curl usage.
                PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
                        if !app.DevMode {
                                return errors.New("events is a dev-only command; re-run with --dev (or set BREYTA_DEV=1)")
                        }
                        return nil
                },
                RunE: func(cmd *cobra.Command, args []string) error {
                        return cmd.Help()
                },
        }

        cmd.AddCommand(newEventsPostCmd(app))
        return cmd
}

func escapePathSegments(p string) string {
        p = strings.TrimSpace(p)
        p = strings.TrimPrefix(p, "/")
        if p == "" {
                return ""
        }
        parts := strings.Split(p, "/")
        escaped := make([]string, 0, len(parts))
        for _, part := range parts {
                if strings.TrimSpace(part) == "" {
                        continue
                }
                escaped = append(escaped, url.PathEscape(part))
        }
        return strings.Join(escaped, "/")
}

func newEventsPostCmd(app *App) *cobra.Command {
        var payload string
        cmd := &cobra.Command{
                Use:   "post <event-path>",
                Short: "POST a JSON event payload to /events/*",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if !isAPIMode(app) {
                                return writeNotImplemented(cmd, app, "events requires --api/BREYTA_API_URL (API mode)")
                        }
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }

                        eventPath := escapePathSegments(args[0])
                        if eventPath == "" {
                                return writeErr(cmd, errors.New("empty event-path"))
                        }

                        body := map[string]any{}
                        if strings.TrimSpace(payload) != "" {
                                var v any
                                if err := json.Unmarshal([]byte(payload), &v); err != nil {
                                        return writeErr(cmd, errors.New("invalid --payload JSON"))
                                }
                                m, ok := v.(map[string]any)
                                if !ok {
                                        return writeErr(cmd, errors.New("--payload must be a JSON object"))
                                }
                                body = m
                        }

                        out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/events/"+eventPath, nil, body)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        cmd.Flags().StringVar(&payload, "payload", "", "JSON object payload to POST (default: {})")
        return cmd
}
