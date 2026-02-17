package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhooksSend_DraftEndpoint(t *testing.T) {
	var gotPath string
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "tok-123",
		"webhooks", "send",
		"--path", "webhooks/orders",
		"--draft",
		"--json", `{"orderId":"o-1"}`,
	)
	if err != nil {
		t.Fatalf("webhooks send --draft failed: %v\n%s", err, stdout)
	}
	if gotPath != "/api/events/draft/webhooks/orders" {
		t.Fatalf("expected /api/events/draft/webhooks/orders, got %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("expected bearer auth header for --draft, got %q", gotAuth)
	}
}

func TestWebhooksSend_DefaultEndpoint(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "tok-123",
		"webhooks", "send",
		"--path", "webhooks/orders",
		"--json", `{"orderId":"o-1"}`,
	)
	if err != nil {
		t.Fatalf("webhooks send failed: %v\n%s", err, stdout)
	}
	if gotPath != "/ws-acme/events/webhooks/orders" {
		t.Fatalf("expected /ws-acme/events/webhooks/orders, got %q", gotPath)
	}
}

func TestWebhooksSend_ValidateOnly_DraftAddsDraftQuery(t *testing.T) {
	var gotPath string
	var gotDraft string
	var gotPersistResources string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotDraft = r.URL.Query().Get("draft")
		gotPersistResources = r.URL.Query().Get("persist-resources")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "tok-123",
		"webhooks", "send",
		"--path", "webhooks/orders",
		"--draft",
		"--validate-only",
		"--persist-resources",
		"--json", `{"orderId":"o-1"}`,
	)
	if err != nil {
		t.Fatalf("webhooks send --validate-only --draft failed: %v\n%s", err, stdout)
	}
	if gotPath != "/api/events/validate/webhooks/orders" {
		t.Fatalf("expected /api/events/validate/webhooks/orders, got %q", gotPath)
	}
	if gotDraft != "true" {
		t.Fatalf("expected draft=true query flag, got %q", gotDraft)
	}
	if gotPersistResources != "true" {
		t.Fatalf("expected persist-resources=true query flag, got %q", gotPersistResources)
	}
}
