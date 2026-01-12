package cli

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
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

Tip:
  To discover webhook event URLs for a flow, use:
    breyta triggers webhook-url --flow <slug>
`),
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
        var webhookTriggerID string
        cmd := &cobra.Command{
                Use:   "post [event-path]",
                Short: "POST a JSON event payload to /events/*",
                Args:  cobra.MaximumNArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if !isAPIMode(app) {
                                return writeNotImplemented(cmd, app, "events requires --api/BREYTA_API_URL (API mode)")
                        }
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }

                        eventPathRaw := ""
                        if strings.TrimSpace(webhookTriggerID) != "" {
                                // Resolve trigger -> webhook path.
                                client := apiClient(app)
                                triggerID := strings.TrimSpace(webhookTriggerID)
                                outAny, status, err := client.DoREST(context.Background(), http.MethodGet, "/api/triggers/"+url.PathEscape(triggerID), nil, nil)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                if status >= 400 {
                                        return writeREST(cmd, app, status, outAny)
                                }

                                out, _ := outAny.(map[string]any)
                                triggerAny := out["trigger"]
                                trigger, _ := triggerAny.(map[string]any)
                                if trigger == nil {
                                        return writeErr(cmd, fmt.Errorf("missing trigger payload for id=%s", triggerID))
                                }
                                config, _ := trigger["config"].(map[string]any)
                                path, _ := config["path"].(string)
                                if strings.TrimSpace(path) == "" {
                                        return writeErr(cmd, fmt.Errorf("trigger %s has no config.path", triggerID))
                                }
                                eventPathRaw = path
                        } else {
                                if len(args) != 1 {
                                        return writeErr(cmd, errors.New("missing event-path (or provide --trigger)"))
                                }
                                eventPathRaw = args[0]
                        }

                        eventPath := escapePathSegments(eventPathRaw)
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

                        eventURL := fmt.Sprintf("/%s/events/%s", strings.TrimSpace(app.WorkspaceID), eventPath)
                        out, status, err := apiClient(app).DoRootREST(context.Background(), http.MethodPost, eventURL, nil, body)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        cmd.Flags().StringVar(&payload, "payload", "", "JSON object payload to POST (default: {})")
        cmd.Flags().StringVar(&webhookTriggerID, "trigger", "", "Resolve and POST to the webhook trigger's event path (webhook triggers only)")
        return cmd
}
