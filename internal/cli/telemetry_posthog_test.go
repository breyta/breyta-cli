package cli

import (
	"context"
	"encoding/base64"
	"testing"
	"time"
)

func jwtForTelemetry(email, uid, name string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := `{"email":"` + email + `","user_id":"` + uid + `","sub":"` + uid + `","name":"` + name + `"}`
	payload = base64.RawURLEncoding.EncodeToString([]byte(payload))
	return header + "." + payload + "."
}

func TestTrackAuthLoginTelemetry_IdentifiesForHostedFlows(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	orig := posthogCaptureFn
	origIdentify := posthogIdentifyFn
	t.Cleanup(func() {
		posthogCaptureFn = orig
		posthogIdentifyFn = origIdentify
	})

	identified := make(chan posthogIdentifyPayload, 1)
	posthogIdentifyFn = func(_ context.Context, payload posthogIdentifyPayload) error {
		identified <- payload
		return nil
	}

	app := &App{APIURL: "https://flows.breyta.ai"}
	trackAuthLoginTelemetry(app, "browser", jwtForTelemetry("user@example.com", "uid-123", "User Example"), nil)

	select {
	case payload := <-identified:
		if payload.DistinctID != "uid-123" {
			t.Fatalf("unexpected distinct id: %q", payload.DistinctID)
		}
		if got, _ := payload.Properties["email"].(string); got != "user@example.com" {
			t.Fatalf("unexpected email property: %q", got)
		}
		if got, _ := payload.Properties["source"].(string); got != "browser" {
			t.Fatalf("unexpected source property: %q", got)
		}
		if got, _ := payload.Properties["api_host"].(string); got != "flows.breyta.ai" {
			t.Fatalf("unexpected api_host property: %q", got)
		}
		if got, _ := payload.Properties["name"].(string); got != "User Example" {
			t.Fatalf("unexpected name property: %q", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected identify to be called")
	}
}

func TestTrackAuthLoginTelemetry_DoesNotSendForNonHostedAPI(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	origIdentify := posthogIdentifyFn
	t.Cleanup(func() { posthogIdentifyFn = origIdentify })

	identified := make(chan posthogIdentifyPayload, 1)
	posthogIdentifyFn = func(_ context.Context, payload posthogIdentifyPayload) error {
		identified <- payload
		return nil
	}

	app := &App{APIURL: "http://localhost:8089"}
	trackAuthLoginTelemetry(app, "browser", jwtForTelemetry("user@example.com", "uid-123", "User Example"), nil)

	select {
	case payload := <-identified:
		t.Fatalf("did not expect identify, got payload: %+v", payload)
	case <-time.After(200 * time.Millisecond):
		// Expected: no telemetry for non-hosted API by default.
	}
}

func TestTrackCommandTelemetry_EmitsMappedEvent(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	orig := posthogCaptureFn
	t.Cleanup(func() { posthogCaptureFn = orig })

	captured := make(chan posthogCapturePayload, 1)
	posthogCaptureFn = func(_ context.Context, payload posthogCapturePayload) error {
		captured <- payload
		return nil
	}

	app := &App{
		APIURL: "https://flows.breyta.ai",
		Token:  jwtForTelemetry("user@example.com", "uid-123", "User Example"),
	}
	trackCommandTelemetry(app, "flows.validate", map[string]any{"flowSlug": "daily-sales"}, 200, true)

	select {
	case payload := <-captured:
		if payload.Event != "cli_flow_validated" {
			t.Fatalf("unexpected event: %q", payload.Event)
		}
		if payload.DistinctID != "uid-123" {
			t.Fatalf("unexpected distinct id: %q", payload.DistinctID)
		}
		if got, _ := payload.Properties["flow_slug"].(string); got != "daily-sales" {
			t.Fatalf("unexpected flow_slug: %q", got)
		}
		if got, _ := payload.Properties["command"].(string); got != "flows.validate" {
			t.Fatalf("unexpected command property: %q", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected telemetry capture to be called")
	}
}

func TestTrackCommandTelemetry_SkipsUnmappedCommand(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	orig := posthogCaptureFn
	t.Cleanup(func() { posthogCaptureFn = orig })

	captured := make(chan posthogCapturePayload, 1)
	posthogCaptureFn = func(_ context.Context, payload posthogCapturePayload) error {
		captured <- payload
		return nil
	}

	app := &App{
		APIURL: "https://flows.breyta.ai",
		Token:  jwtForTelemetry("user@example.com", "uid-123", "User Example"),
	}
	trackCommandTelemetry(app, "flows.list", nil, 200, true)

	select {
	case payload := <-captured:
		t.Fatalf("did not expect telemetry capture, got payload: %+v", payload)
	case <-time.After(200 * time.Millisecond):
		// Expected: no telemetry for unmapped commands.
	}
}

func TestTrackAuthLoginTelemetry_UsesTokenHashDistinctIDWhenUIDAndEmailMissing(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	origIdentify := posthogIdentifyFn
	t.Cleanup(func() { posthogIdentifyFn = origIdentify })

	identified := make(chan posthogIdentifyPayload, 1)
	posthogIdentifyFn = func(_ context.Context, payload posthogIdentifyPayload) error {
		identified <- payload
		return nil
	}

	app := &App{APIURL: "https://flows.breyta.ai"}
	token := "opaque-token-without-email-claim"
	trackAuthLoginTelemetry(app, "browser", token, nil)

	select {
	case payload := <-identified:
		want := telemetryDistinctID(nil, token)
		if payload.DistinctID != want {
			t.Fatalf("unexpected distinct id: got %q want %q", payload.DistinctID, want)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected identify to be called")
	}
}

func TestTrackAuthLoginTelemetry_PrefersOpaqueTokenFallbackOverSeparateUID(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	origIdentify := posthogIdentifyFn
	t.Cleanup(func() { posthogIdentifyFn = origIdentify })

	identified := make(chan posthogIdentifyPayload, 1)
	posthogIdentifyFn = func(_ context.Context, payload posthogIdentifyPayload) error {
		identified <- payload
		return nil
	}

	app := &App{APIURL: "https://flows.breyta.ai"}
	token := "opaque-token-without-email-claim"
	trackAuthLoginTelemetry(app, "browser", token, "uid-from-auth-response")

	select {
	case payload := <-identified:
		want := telemetryDistinctID(nil, token)
		if payload.DistinctID != want {
			t.Fatalf("unexpected distinct id: got %q want %q", payload.DistinctID, want)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected identify to be called")
	}
}

func TestTrackAuthAndCommandTelemetry_UseSameDistinctIDScheme(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	orig := posthogCaptureFn
	origIdentify := posthogIdentifyFn
	t.Cleanup(func() {
		posthogCaptureFn = orig
		posthogIdentifyFn = origIdentify
	})

	captured := make(chan string, 2)
	posthogIdentifyFn = func(_ context.Context, payload posthogIdentifyPayload) error {
		captured <- payload.DistinctID
		return nil
	}
	posthogCaptureFn = func(_ context.Context, payload posthogCapturePayload) error {
		captured <- payload.DistinctID
		return nil
	}

	token := jwtForTelemetry("user@example.com", "uid-123", "User Example")
	app := &App{
		APIURL: "https://flows.breyta.ai",
		Token:  token,
	}
	trackAuthLoginTelemetry(app, "browser", token, "uid-123")
	trackCommandTelemetry(app, "flows.validate", map[string]any{"flowSlug": "daily-sales"}, 200, true)

	first := <-captured
	second := <-captured
	if first != second {
		t.Fatalf("expected identical distinct ids, got %q and %q", first, second)
	}
}

func TestTelemetryDistinctID_UsesUIDWhenTokenMissing(t *testing.T) {
	if got := telemetryDistinctID("uid-123", ""); got != "uid-123" {
		t.Fatalf("unexpected distinct id: %q", got)
	}
}

func TestPosthogEnabledForLogin_DisabledOverridesEnabled(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "true")

	app := &App{APIURL: "https://flows.breyta.ai"}
	if posthogEnabledForLogin(app) {
		t.Fatal("expected telemetry to be disabled when BREYTA_POSTHOG_DISABLED=true")
	}
}
