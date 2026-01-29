package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/breyta/breyta-cli/internal/configstore"
)

func TestDevModeUsesTokenEnv(t *testing.T) {
	t.Setenv("BREYTA_TOKEN", "token-123")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := configstore.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if err := configstore.SaveAtomic(path, &configstore.Store{DevMode: true}); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

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

	_, _, err = runCLIArgs(t,
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"flows", "list",
	)
	if err != nil {
		t.Fatalf("flows list failed: %v", err)
	}
}

func TestTokenFlagRejectedOutsideDevMode(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--token", "should-fail",
		"flows", "list",
	)
	if err == nil {
		t.Fatalf("expected error when using --token without --dev; stdout=%q stderr=%q", stdout, stderr)
	}
}
