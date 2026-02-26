package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/authinfo"
)

const (
	defaultPostHogHost = "https://eu.i.posthog.com"
	defaultPostHogKey  = "phc_IWzuX5ONKdDwYVJX1zgXdUDcuBU8DIFGDIe5WywISiT"
)

type posthogCapturePayload struct {
	Event      string
	DistinctID string
	Properties map[string]any
}

var posthogCaptureFn = sendPosthogCapture

func trackAuthLoginTelemetry(app *App, source, token string, uid any) {
	if !posthogEnabledForLogin(app) {
		return
	}

	distinctID := telemetryDistinctID(uid, token)
	if strings.TrimSpace(distinctID) == "" {
		return
	}

	props := map[string]any{
		"product":  "flows",
		"channel":  "cli",
		"source":   source,
		"api_host": apiHostname(app.APIURL),
	}
	if email := strings.TrimSpace(authinfo.EmailFromToken(token)); email != "" {
		props["email_domain"] = emailDomain(email)
	}

	payload := posthogCapturePayload{
		Event:      "cli_auth_login_completed",
		DistinctID: distinctID,
		Properties: props,
	}

	// Best-effort, non-blocking telemetry.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		_ = posthogCaptureFn(ctx, payload)
	}()
}

func posthogEnabledForLogin(app *App) bool {
	if forceEnabled := strings.EqualFold(strings.TrimSpace(os.Getenv("BREYTA_POSTHOG_ENABLED")), "true"); forceEnabled {
		return true
	}
	if disabled := strings.EqualFold(strings.TrimSpace(os.Getenv("BREYTA_POSTHOG_DISABLED")), "true"); disabled {
		return false
	}
	return strings.EqualFold(apiHostname(app.APIURL), "flows.breyta.ai")
}

func telemetryDistinctID(uid any, token string) string {
	if uidStr, ok := uid.(string); ok {
		uidStr = strings.TrimSpace(uidStr)
		if uidStr != "" {
			return uidStr
		}
	}
	if email := strings.TrimSpace(authinfo.EmailFromToken(token)); email != "" {
		return "email:" + strings.ToLower(email)
	}
	return ""
}

func apiHostname(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Hostname()))
}

func emailDomain(email string) string {
	parts := strings.SplitN(strings.TrimSpace(email), "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[1]))
}

func posthogHost() string {
	if host := strings.TrimSpace(os.Getenv("BREYTA_POSTHOG_HOST")); host != "" {
		return host
	}
	if host := strings.TrimSpace(os.Getenv("POSTHOG_HOST")); host != "" {
		return host
	}
	return defaultPostHogHost
}

func posthogAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("BREYTA_POSTHOG_API_KEY")); key != "" {
		return key
	}
	if key := strings.TrimSpace(os.Getenv("POSTHOG_API_KEY")); key != "" {
		return key
	}
	return defaultPostHogKey
}

func sendPosthogCapture(ctx context.Context, payload posthogCapturePayload) error {
	body, err := json.Marshal(map[string]any{
		"api_key":     posthogAPIKey(),
		"event":       payload.Event,
		"distinct_id": payload.DistinctID,
		"properties":  payload.Properties,
	})
	if err != nil {
		return err
	}

	host := strings.TrimRight(posthogHost(), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/capture/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("posthog capture failed (status=%d)", resp.StatusCode)
	}
	return nil
}
