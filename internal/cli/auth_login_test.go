package cli_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/breyta/breyta-cli/internal/authstore"
)

func TestAuthLoginVerifiesAndStoresPersonalToken(t *testing.T) {
	server := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/auth/me" {
			http.NotFound(w, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer personal-token" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"email": "operator@example.test"}, "workspaces": []any{}})
	}))
	defer server.Close()

	storePath := filepath.Join(t.TempDir(), "auth.json")
	stdout, stderr, err := runCLIArgs(t, "--api", server.URL, "--token", "personal-token", "auth", "login", "--store", storePath)
	if err != nil {
		t.Fatalf("login: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	store, err := authstore.Load(storePath)
	if err != nil {
		t.Fatalf("load auth store: %v", err)
	}
	if token, ok := store.Get(server.URL); !ok || token != "personal-token" {
		t.Fatalf("stored token = %q, %v", token, ok)
	}
}
