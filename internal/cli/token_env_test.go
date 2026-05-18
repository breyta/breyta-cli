package cli_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/breyta/breyta-cli/internal/authstore"
	"github.com/breyta/breyta-cli/internal/configstore"
)

func TestDevModeUsesTokenEnv(t *testing.T) {
	t.Setenv("BREYTA_TOKEN", "token-123")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	path, err := configstore.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if err := configstore.SaveAtomic(path, &configstore.Store{DevMode: true}); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestDevModeEnvUsesAPIURLAndTokenEnv(t *testing.T) {
	t.Setenv("BREYTA_DEV", "1")
	t.Setenv("BREYTA_TOKEN", "token-env")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-env" {
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

	t.Setenv("BREYTA_API_URL", srv.URL)
	_, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"flows", "list",
	)
	if err != nil {
		t.Fatalf("flows list with BREYTA_DEV env failed: %v", err)
	}
}

func TestDevModeEnvAllowsWorkspaceBootstrap(t *testing.T) {
	t.Setenv("BREYTA_DEV", "1")
	t.Setenv("BREYTA_TOKEN", "dev-user-123")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/debug/workspace/bootstrap" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer dev-user-123" {
			t.Fatalf("expected Authorization header to use BREYTA_TOKEN, got %q", got)
		}
		if got := r.Header.Get("x-debug-user-id"); got != "dev-user-123" {
			t.Fatalf("expected x-debug-user-id header to use BREYTA_TOKEN, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"workspaceId": "ws-acme",
		})
	}))
	defer srv.Close()

	t.Setenv("BREYTA_API_URL", srv.URL)
	_, _, err := runCLIArgs(t,
		"workspaces", "bootstrap", "ws-acme",
		"--name", "IT",
	)
	if err != nil {
		t.Fatalf("workspaces bootstrap with BREYTA_DEV env failed: %v", err)
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

func TestAPIFlagRejectedOutsideDevModeWithoutAPIKey(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--api", "https://example.invalid",
		"flows", "list",
	)
	if err == nil {
		t.Fatalf("expected error when using --api without machine auth; stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestAPIKeyFlagAllowedOutsideDevMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer bsa_sak-123_secret" {
			t.Fatalf("expected Authorization header to use --api-key, got %q", got)
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
		"--api-key", "bsa_sak-123_secret",
		"--workspace", "ws-acme",
		"flows", "list",
	)
	if err != nil {
		t.Fatalf("flows list with --api-key failed: %v", err)
	}
}

func TestAPIKeyEnvAllowedOutsideDevMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))
	t.Setenv("BREYTA_API_KEY", "bsa_sak-456_secret")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer bsa_sak-456_secret" {
			t.Fatalf("expected Authorization header to use BREYTA_API_KEY, got %q", got)
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

	t.Setenv("BREYTA_API_URL", srv.URL)
	_, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"flows", "list",
	)
	if err != nil {
		t.Fatalf("flows list with BREYTA_API_KEY failed: %v", err)
	}
}

func TestExplicitTokenFlagWinsOverAPIKeyEnvInDevMode(t *testing.T) {
	t.Setenv("BREYTA_API_KEY", "bsa_sak-999_secret")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	path, err := configstore.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if err := configstore.SaveAtomic(path, &configstore.Store{DevMode: true}); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-flag" {
			t.Fatalf("expected Authorization header to use explicit --token, got %q", got)
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
		"--token", "token-flag",
		"flows", "list",
	)
	if err != nil {
		t.Fatalf("flows list with explicit --token failed: %v", err)
	}
}

func TestLocalAPIKeyFlagDoesNotSuppressAuthStoreTokenLoading(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	storePath := filepath.Join(tmp, "auth.json")
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events/draft/webhooks/orders" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer authstore-token" {
			t.Fatalf("expected Authorization header to use auth store token, got %q", got)
		}
		if got := r.Header.Get("X-API-Key"); got != "payload-key" {
			t.Fatalf("expected local webhook api-key header, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
		})
	}))
	defer srv.Close()

	st := &authstore.Store{}
	st.Set(srv.URL, "authstore-token")
	if err := authstore.SaveAtomic(storePath, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	_, _, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"webhooks", "send",
		"--draft",
		"--path", "webhooks/orders",
		"--json", `{"ok":true}`,
		"--api-key", "payload-key",
	)
	if err != nil {
		t.Fatalf("webhooks send with local --api-key failed: %v", err)
	}
}
