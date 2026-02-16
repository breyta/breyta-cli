package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhooksSend_CurrentEndpoint(t *testing.T) {
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
		"--current",
		"--json", `{"orderId":"o-1"}`,
	)
	if err != nil {
		t.Fatalf("webhooks send --current failed: %v\n%s", err, stdout)
	}
	if gotPath != "/api/events/current/webhooks/orders" {
		t.Fatalf("expected /api/events/current/webhooks/orders, got %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("expected bearer auth header for --current, got %q", gotAuth)
	}
}

func TestWebhooksSend_DraftAliasUsesCurrentEndpoint(t *testing.T) {
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
		"--draft",
		"--json", `{"orderId":"o-1"}`,
	)
	if err != nil {
		t.Fatalf("webhooks send --draft failed: %v\n%s", err, stdout)
	}
	if gotPath != "/api/events/current/webhooks/orders" {
		t.Fatalf("expected draft alias to map to /api/events/current/webhooks/orders, got %q", gotPath)
	}
}
