package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDevModeUsesTokenEnv(t *testing.T) {
	t.Setenv("BREYTA_DEV", "1")
	t.Setenv("BREYTA_TOKEN", "token-123")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected Authorization header to use BREYTA_TOKEN, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"items": []any{},
			},
		})
	}))
	defer srv.Close()

	_, _, err := runCLIArgs(t,
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"flows", "list",
	)
	if err != nil {
		t.Fatalf("flows list failed: %v", err)
	}
}

func TestTokenFlagRejectedOutsideDevMode(t *testing.T) {
	_ = os.Unsetenv("BREYTA_DEV")
	stdout, stderr, err := runCLIArgs(t,
		"--token", "should-fail",
		"flows", "list",
	)
	if err == nil {
		t.Fatalf("expected error when using --token without --dev; stdout=%q stderr=%q", stdout, stderr)
	}
}
