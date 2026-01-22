package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
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

func TestDocs_Index_ShowsMockSurfaceInDevMode(t *testing.T) {
	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://localhost:9999",
		"--token", "user-dev",
		"docs",
	)
	if err != nil {
		t.Fatalf("docs failed: %v\n%s", err, stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("registry")) {
		t.Fatalf("expected docs index to include registry in dev mode\n---\n%s", stdout)
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

func TestDocs_Index_IncludesResourcesByDefault(t *testing.T) {
	stdout, _, err := runCLIArgs(t,
		"docs",
	)
	if err != nil {
		t.Fatalf("docs failed: %v\n%s", err, stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("`resources`")) {
		t.Fatalf("expected docs index to include resources\n---\n%s", stdout)
	}
}

func TestDocs_CanDocumentHiddenCommandByName(t *testing.T) {
	stdout, _, err := runCLIArgs(t,
		"docs", "resources",
	)
	if err != nil {
		t.Fatalf("docs resources failed: %v\n%s", err, stdout)
	}
	if !bytes.HasPrefix([]byte(stdout), []byte("## breyta resources")) {
		t.Fatalf("expected markdown docs header for resources\n---\n%s", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("Unified resource access")) {
		t.Fatalf("expected resources docs content\n---\n%s", stdout)
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
		"--dev",
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
		"--dev",
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

func TestFlowsBindingsApply_UsesProfilesBindingsApplyCommand(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "profiles.bindings.apply" {
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
		args, _ := body["args"].(map[string]any)
		inputs, _ := args["inputs"].(map[string]any)
		if inputs["conn-api"] != "conn-123" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing conn-api",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "flow-1",
				"ok":       true,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "bindings", "apply", "flow-1",
		"--set", "api.conn=conn-123",
	)
	if err != nil {
		t.Fatalf("flows bindings apply failed: %v\n%s", err, stdout)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
}

func TestFlowsActivate_SendsVersionArg(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "profiles.activate" {
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
		args, _ := body["args"].(map[string]any)
		if args["version"] != "2" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing version",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "flow-2",
				"enabled":  true,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "activate", "flow-2",
		"--version", "2",
	)
	if err != nil {
		t.Fatalf("flows activate failed: %v\n%s", err, stdout)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
}

func TestFlowsBindingsTemplate_PrefillsCurrentBindings(t *testing.T) {
	t.Helper()
	statusCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd, _ := body["command"].(string)
		switch cmd {
		case "profiles.template":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"requirements": []any{
						map[string]any{
							"slot":  "api",
							"type":  "http-api",
							"label": "API",
							"auth":  map[string]any{"type": "api-key"},
						},
					},
				},
			})
		case "profiles.status":
			statusCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"bindingValues": map[string]any{"api": "conn-123"},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "unexpected command",
				},
			})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "bindings", "template", "flow-1",
	)
	if err != nil {
		t.Fatalf("flows bindings template failed: %v\n%s", err, stdout)
	}
	if !statusCalled {
		t.Fatalf("expected profiles.status to be called")
	}
		// EDN encoding may omit whitespace between tokens (still valid EDN), so accept both.
		if !regexp.MustCompile(`:conn\s*"conn-123"`).MatchString(stdout) {
			t.Fatalf("expected template to include conn binding, got:\n%s", stdout)
		}
	}

func TestFlowsBindingsTemplate_CleanSkipsBindings(t *testing.T) {
	t.Helper()
	statusCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd, _ := body["command"].(string)
		switch cmd {
		case "profiles.template":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"requirements": []any{
						map[string]any{
							"slot": "api",
							"type": "http-api",
							"auth": map[string]any{"type": "api-key"},
						},
					},
				},
			})
		case "profiles.status":
			statusCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "unexpected command",
				},
			})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "bindings", "template", "flow-1",
		"--clean",
	)
	if err != nil {
		t.Fatalf("flows bindings template --clean failed: %v\n%s", err, stdout)
	}
	if statusCalled {
		t.Fatalf("expected profiles.status not to be called")
	}
	if bytes.Contains([]byte(stdout), []byte(`:conn "`)) {
		t.Fatalf("expected clean template to omit conn binding, got:\n%s", stdout)
	}
}

func TestResources_DefaultsToAPIMode(t *testing.T) {
	// Ensure API-only commands don't fail with "requires API mode" just because the API URL
	// hasn't been defaulted yet; they should proceed to normal auth errors instead.
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(t.TempDir(), "auth.json"))

	_, stderr, err := runCLIArgs(t, "resources", "list")
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if bytes.Contains([]byte(stderr), []byte("requires API mode")) {
		t.Fatalf("unexpected api-mode error:\n%s", stderr)
	}
	if !bytes.Contains([]byte(stderr), []byte("missing token")) {
		t.Fatalf("expected missing-token error, got:\n%s", stderr)
	}
}
