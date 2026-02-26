package cli

import (
	"context"
	"encoding/base64"
	"testing"
	"time"
)

func jwtWithEmail(email string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"` + email + `"}`))
	return header + "." + payload + "."
}

func TestTrackAuthLoginTelemetry_SendsForHostedFlows(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	orig := posthogCaptureFn
	t.Cleanup(func() { posthogCaptureFn = orig })

	captured := make(chan posthogCapturePayload, 1)
	posthogCaptureFn = func(_ context.Context, payload posthogCapturePayload) error {
		captured <- payload
		return nil
	}

	app := &App{APIURL: "https://flows.breyta.ai"}
	trackAuthLoginTelemetry(app, "browser", jwtWithEmail("user@example.com"), nil)

	select {
	case payload := <-captured:
		if payload.Event != "cli_auth_login_completed" {
			t.Fatalf("unexpected event: %q", payload.Event)
		}
		if payload.DistinctID != "email:user@example.com" {
			t.Fatalf("unexpected distinct id: %q", payload.DistinctID)
		}
		if got, _ := payload.Properties["source"].(string); got != "browser" {
			t.Fatalf("unexpected source property: %q", got)
		}
		if got, _ := payload.Properties["api_host"].(string); got != "flows.breyta.ai" {
			t.Fatalf("unexpected api_host property: %q", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected telemetry capture to be called")
	}
}

func TestTrackAuthLoginTelemetry_DoesNotSendForNonHostedAPI(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	orig := posthogCaptureFn
	t.Cleanup(func() { posthogCaptureFn = orig })

	captured := make(chan posthogCapturePayload, 1)
	posthogCaptureFn = func(_ context.Context, payload posthogCapturePayload) error {
		captured <- payload
		return nil
	}

	app := &App{APIURL: "http://localhost:8089"}
	trackAuthLoginTelemetry(app, "browser", jwtWithEmail("user@example.com"), nil)

	select {
	case payload := <-captured:
		t.Fatalf("did not expect telemetry capture, got payload: %+v", payload)
	case <-time.After(200 * time.Millisecond):
		// Expected: no telemetry for non-hosted API by default.
	}
}
