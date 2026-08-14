package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCustomEngineUsesTokenEnvironment(t *testing.T) {
	t.Setenv("BREYTA_TOKEN", "token-123")
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"items": []any{}}})
	}))
	defer srv.Close()
	if _, _, err := runCLIArgs(t, "--api", srv.URL, "--workspace", "ws-acme", "flows", "list"); err != nil {
		t.Fatalf("flows list: %v", err)
	}
}

func TestExplicitTokenWinsOverAPIKeyEnvironment(t *testing.T) {
	t.Setenv("BREYTA_API_KEY", "service-key")
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer personal-token" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"items": []any{}}})
	}))
	defer srv.Close()
	if _, _, err := runCLIArgs(t, "--api", srv.URL, "--workspace", "ws-acme", "--token", "personal-token", "flows", "list"); err != nil {
		t.Fatalf("flows list: %v", err)
	}
}
