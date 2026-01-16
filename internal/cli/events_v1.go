package cli

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

func computeHMACSHA256Base64(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func newEventsPostCmd(app *App) *cobra.Command {
	var payload string
	var webhookTriggerID string
	var hmacSecret string
	var signatureHeader string
	cmd := &cobra.Command{
		Use:   "post [event-path]",
		Short: "POST a JSON event payload to /events/*",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "events requires --api/BREYTA_API_URL (API mode)")
			}
			if err := requireAPI(app); err != nil {
				return writeFailure(cmd, app, "api_auth_required", err, "Provide --token or run `breyta auth login`.", nil)
			}

			eventPathRaw := ""
			if strings.TrimSpace(webhookTriggerID) != "" {
				// Resolve trigger -> webhook path.
				client := apiClient(app)
				triggerID := strings.TrimSpace(webhookTriggerID)
				outAny, status, err := client.DoREST(context.Background(), http.MethodGet, "/api/triggers/"+url.PathEscape(triggerID), nil, nil)
				if err != nil {
					return writeFailure(cmd, app, "trigger_fetch_failed", err, "Check trigger id and API connectivity.", map[string]any{"triggerId": triggerID})
				}
				if status >= 400 {
					return writeREST(cmd, app, status, outAny)
				}

				out, _ := outAny.(map[string]any)
				triggerAny := out["trigger"]
				trigger, _ := triggerAny.(map[string]any)
				if trigger == nil {
					return writeFailure(cmd, app, "trigger_payload_missing", fmt.Errorf("missing trigger payload for id=%s", triggerID), "The server response did not include a trigger object.", outAny)
				}
				config, _ := trigger["config"].(map[string]any)
				path, _ := config["path"].(string)
				if strings.TrimSpace(path) == "" {
					return writeFailure(cmd, app, "trigger_path_missing", fmt.Errorf("trigger %s has no config.path", triggerID), "Use `breyta triggers webhook-url` to verify trigger configuration.", trigger)
				}
				eventPathRaw = path
			} else {
				if len(args) != 1 {
					return writeFailure(cmd, app, "missing_event_path", errors.New("missing event-path (or provide --trigger)"), "Provide an event-path argument (e.g. `breyta events post demo-hook`) or pass --trigger <id>.", nil)
				}
				eventPathRaw = args[0]
			}

			eventPath := escapePathSegments(eventPathRaw)
			if eventPath == "" {
				return writeFailure(cmd, app, "empty_event_path", errors.New("empty event-path"), "Provide a non-empty event-path (e.g. `webhooks/my-flow/ping`).", map[string]any{"eventPath": eventPathRaw})
			}

			body := map[string]any{}
			if strings.TrimSpace(payload) != "" {
				var v any
				if err := json.Unmarshal([]byte(payload), &v); err != nil {
					return writeFailure(cmd, app, "invalid_payload_json", errors.New("invalid --payload JSON"), "Provide a JSON object string (example: --payload '{\"ok\":true}').", map[string]any{"payload": payload})
				}
				m, ok := v.(map[string]any)
				if !ok {
					return writeFailure(cmd, app, "payload_not_object", errors.New("--payload must be a JSON object"), "Provide an object (example: --payload '{\"ok\":true}').", map[string]any{"payload": payload})
				}
				body = m
			}

			eventURL := fmt.Sprintf("/%s/events/%s", strings.TrimSpace(app.WorkspaceID), eventPath)
			headers := map[string]string{}
			var bodyBytes []byte
			if body != nil {
				var buf bytes.Buffer
				if err := json.NewEncoder(&buf).Encode(body); err != nil {
					return writeFailure(cmd, app, "payload_encode_failed", errors.New("failed to encode JSON payload"), "Try a simpler payload or report a bug.", nil)
				}
				bodyBytes = buf.Bytes()
			}
			if strings.TrimSpace(hmacSecret) != "" {
				h := strings.TrimSpace(signatureHeader)
				if h == "" {
					h = "X-Signature"
				}
				headers[h] = computeHMACSHA256Base64(hmacSecret, bodyBytes)
			}

			out, status, err := apiClient(app).DoRootRESTBytes(context.Background(), http.MethodPost, eventURL, nil, bodyBytes, headers)
			if err != nil {
				return writeFailure(cmd, app, "events_post_failed", err, "Check API connectivity and auth; verify the trigger/webhook path is correct.", map[string]any{"eventURL": eventURL})
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&payload, "payload", "", "JSON object payload to POST (default: {})")
	cmd.Flags().StringVar(&webhookTriggerID, "trigger", "", "Resolve and POST to the webhook trigger's event path (webhook triggers only)")
	cmd.Flags().StringVar(&hmacSecret, "hmac-secret", "", "HMAC signing secret (returned by `breyta triggers webhook-secret ...`) (API mode only)")
	cmd.Flags().StringVar(&signatureHeader, "signature-header", "X-Signature", "Signature header name for HMAC signing (API mode only)")
	return cmd
}
