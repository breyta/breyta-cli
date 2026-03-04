package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFlowsPush_EmitsPushTelemetryWhenPostPushValidationFails(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")

	origUseDoAPICommandFn := useDoAPICommandFn
	origDoAPICommandFn := doAPICommandFn
	t.Cleanup(func() {
		useDoAPICommandFn = origUseDoAPICommandFn
		doAPICommandFn = origDoAPICommandFn
	})
	useDoAPICommandFn = false
	doAPICommandFn = doAPICommand

	origCapture := posthogCaptureFn
	t.Cleanup(func() { posthogCaptureFn = origCapture })

	captured := make(chan posthogCapturePayload, 1)
	posthogCaptureFn = func(_ context.Context, payload posthogCapturePayload) error {
		captured <- payload
		return nil
	}

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	if err := os.WriteFile(flowFile, []byte("{:slug :flow-push-telemetry :name \"Flow Push Telemetry\" :flow '(identity 1)}"), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["command"] {
		case "flows.put_draft":
			step++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-push-telemetry", "saved": true},
			})
		case "flows.validate":
			step++
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error":       map[string]any{"message": "validation failed"},
				"data":        map[string]any{"flowSlug": "flow-push-telemetry", "valid": false},
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error":       map[string]any{"message": "unexpected command"},
			})
		}
	}))
	defer srv.Close()

	app := &App{
		WorkspaceID:   "ws-acme",
		APIURL:        srv.URL,
		Token:         "token-without-email",
		TokenExplicit: true,
	}
	cmd := newFlowsPushCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected validation failure from flows push")
	}
	if step != 2 {
		t.Fatalf("expected put_draft + validate commands, got %d\noutput:\n%s", step, out.String())
	}

	select {
	case payload := <-captured:
		if payload.Event != "cli_flow_pushed" {
			t.Fatalf("unexpected event: %q", payload.Event)
		}
		if got, _ := payload.Properties["flow_slug"].(string); got != "flow-push-telemetry" {
			t.Fatalf("unexpected flow_slug: %q", got)
		}
		if got, ok := payload.Properties["validated"].(bool); !ok || got {
			t.Fatalf("expected validated=false, got %#v", payload.Properties["validated"])
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected cli_flow_pushed telemetry event")
	}
}
