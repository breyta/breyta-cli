package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/breyta/breyta-cli/internal/cli"
)

func runCLIArgs(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := cli.NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestDocs_Index_HidesMockSurfaceInAPIMode(t *testing.T) {
	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", "http://localhost:9999",
		"--token", "user-dev",
		"docs",
	)
	if err != nil {
		t.Fatalf("docs failed: %v\n%s", err, stdout)
	}
	if bytes.Contains([]byte(stdout), []byte("registry")) {
		t.Fatalf("expected docs index to hide registry in api mode\n---\n%s", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("`flows`")) {
		t.Fatalf("expected docs index to include flows\n---\n%s", stdout)
	}
}

func TestDocs_Index_HidesMockSurfaceByDefault(t *testing.T) {
	stdout, _, err := runCLIArgs(t,
		"docs",
	)
	if err != nil {
		t.Fatalf("docs failed: %v\n%s", err, stdout)
	}
	if bytes.Contains([]byte(stdout), []byte("registry")) {
		t.Fatalf("expected docs index to hide registry by default\n---\n%s", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("`flows`")) {
		t.Fatalf("expected docs index to include flows\n---\n%s", stdout)
	}
}

func TestFlowsList_UsesAPIInAPIMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "unexpected command",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"items": []any{
					map[string]any{"flowSlug": "x", "name": "X", "activeVersion": 1},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "list",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows list failed: %v\n%s", err, stdout)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
	data, _ := e["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestAPIMode_NoStateFileNeeded(t *testing.T) {
	// Ensure that just running docs in API mode doesn't require mock state setup.
	// (Some older tests set --state; API mode should not depend on it.)
	_ = filepath.Separator
	stdout, stderr, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", "http://localhost:9999",
		"--token", "user-dev",
		"flows", "list",
	)
	// It will fail because it can't reach localhost:9999, but it must not be a state-file error.
	if err == nil {
		t.Fatalf("expected error (no server), got success:\n%s", stdout)
	}
	if bytes.Contains([]byte(stderr), []byte("--state")) || bytes.Contains([]byte(stderr), []byte("mock state")) {
		t.Fatalf("unexpected state-file error in api mode\n---\nstdout:\n%s\n---\nstderr:\n%s", stdout, stderr)
	}
}
