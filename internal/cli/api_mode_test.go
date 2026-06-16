package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/breyta/breyta-cli/internal/cli"
)

func runCLIArgs(t *testing.T, args ...string) (string, string, error) {
	return runCLIArgsWithContext(t, context.Background(), args...)
}

func runCLIArgsWithContext(t *testing.T, ctx context.Context, args ...string) (string, string, error) {
	t.Helper()
	cmd := cli.NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)
	if ctx == nil {
		ctx = context.Background()
	}
	err := cmd.ExecuteContext(ctx)
	return out.String(), errOut.String(), err
}

func TestDocs_Help_DefaultSurface(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	stdout, _, err := runCLIArgs(t,
		"docs",
	)
	if err != nil {
		t.Fatalf("docs help failed: %v\n%s", err, stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("find")) {
		t.Fatalf("expected docs help to include find command\n---\n%s", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("show")) {
		t.Fatalf("expected docs help to include show command\n---\n%s", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("fields")) {
		t.Fatalf("expected docs help to include fields command\n---\n%s", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("sync")) {
		t.Fatalf("expected docs help to include sync command\n---\n%s", stdout)
	}
}

func TestDocsFind_UsesDocsAPI(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("q"); got != "flows push" {
			t.Fatalf("expected q=flows push, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"pages": []map[string]any{
					{"slug": "build-flow-authoring", "title": "Build: Flow Authoring", "source": "flows-api"},
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
		"docs", "find", "flows push",
		"--with-summary=false",
	)
	if err != nil {
		t.Fatalf("docs find failed: %v\n%s", err, stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("docs:build-flow-authoring\tflows-api\t\tBuild: Flow Authoring")) {
		t.Fatalf("expected docs find row, got:\n%s", stdout)
	}
}

func TestFlowsList_UsesAPIInAPIMode(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsDeploy_SendsDeployKeyFlag(t *testing.T) {
	t.Helper()
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.deploy" {
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
		if args["deployKey"] != "ci-secret" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing deployKey",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "flow-guarded",
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
		"flows", "deploy", "flow-guarded",
		"--deploy-key", "ci-secret",
	)
	if err != nil {
		t.Fatalf("flows deploy failed: %v\n%s", err, stdout)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
}

func TestFlowsVersionsActivate_SendsDeployKeyFromEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BREYTA_FLOW_DEPLOY_KEY", "env-secret")
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.versions.activate" {
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
		if args["deployKey"] != "env-secret" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing deployKey",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "flow-guarded",
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
		"flows", "versions", "activate", "flow-guarded",
		"--version", "2",
	)
	if err != nil {
		t.Fatalf("flows versions activate failed: %v\n%s", err, stdout)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
}

func TestFlowsVersionsUpdate_SendsReleaseNote(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.versions.update" {
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
		if args["flowSlug"] != "flow-guarded" || args["version"] != float64(2) || args["releaseNote"] != "Release note body" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "unexpected args",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "flow-guarded",
				"version": map[string]any{
					"version":     2,
					"releaseNote": "Release note body",
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
		"flows", "versions", "update", "flow-guarded",
		"--version", "2",
		"--release-note", "Release note body",
	)
	if err != nil {
		t.Fatalf("flows versions update failed: %v\n%s", err, stdout)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
}

func TestFlowsVersionsUpdate_RejectsConflictingReleaseNoteFlags(t *testing.T) {
	t.Helper()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:1",
		"--token", "user-dev",
		"flows", "versions", "update", "flow-guarded",
		"--version", "2",
		"--release-note", "Release note body",
		"--clear-release-note",
	)
	if err == nil {
		t.Fatalf("expected conflicting release note flags to fail, got success:\n%s", stdout)
	}
	if !strings.Contains(stderr, "--clear-release-note cannot be combined with --release-note/--release-note-file") {
		t.Fatalf("expected conflicting flag error, got stderr:\n%s", stderr)
	}
}

func TestFlowsPush_SendsDeployKeyFromEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BREYTA_FLOW_DEPLOY_KEY", "env-secret")
	tmp := t.TempDir()
	flowFile := filepath.Join(tmp, "flow.clj")
	if err := os.WriteFile(flowFile, []byte("{:slug :push-guarded :name \"Push Guarded\" :concurrency {:type :singleton :on-new-version :supersede} :flow '(let [input (flow/input)] input)}\n"), 0o644); err != nil {
		t.Fatalf("failed to write test flow file: %v", err)
	}
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.put_draft" {
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
		if args["deploy-key"] != "env-secret" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing deploy key",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "push-guarded",
				"saved":    true,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "push",
		"--file", flowFile,
		"--target", "draft",
		"--validate=false",
	)
	if err != nil {
		t.Fatalf("flows push failed: %v\n%s", err, stdout)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
}

func TestFlowsPush_RendersAPIDeprecationWarnings(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	flowFile := filepath.Join(tmp, "flow.clj")
	if err := os.WriteFile(flowFile, []byte("{:slug :push-deprecated :name \"Push Deprecated\" :concurrency {:type :singleton :on-new-version :supersede} :flow '(let [input (flow/input)] input)}\n"), 0o644); err != nil {
		t.Fatalf("failed to write test flow file: %v", err)
	}
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.put_draft" {
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
			"meta": map[string]any{
				"warnings": []map[string]any{
					{
						"kind":        "deprecation",
						"code":        "deprecated_run_collected_form",
						"message":     "Run-collected :requires form fields are deprecated for new flow source.",
						"path":        []any{":requires", 0, ":collect"},
						"replacement": ":invocations {:default {:inputs [...]}}",
						"docsUrl":     "/docs/reference-flow-definition#deprecated-run-form-shapes",
					},
					{
						"kind":    "config",
						"message": "not a deprecation warning",
					},
				},
			},
			"data": map[string]any{
				"flowSlug": "push-deprecated",
				"saved":    true,
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "push",
		"--file", flowFile,
		"--target", "draft",
		"--validate=false",
	)
	if err != nil {
		t.Fatalf("flows push failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stderr, "warning: deprecated flow source pattern: Run-collected :requires form fields are deprecated") {
		t.Fatalf("expected deprecation warning on stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "path: :requires 0 :collect") {
		t.Fatalf("expected warning path on stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "use: :invocations {:default {:inputs [...]}}") {
		t.Fatalf("expected replacement on stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "docs: "+srv.URL+"/docs/reference-flow-definition#deprecated-run-form-shapes") {
		t.Fatalf("expected docs URL on stderr, got:\n%s", stderr)
	}
	if strings.Contains(stderr, "not a deprecation warning") {
		t.Fatalf("expected non-deprecation warnings to stay out of stderr, got:\n%s", stderr)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
}

func TestFlowsPush_ReturnsSavedDraftWhenImmediateValidationCannotFindCreatedFlow(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	flowFile := filepath.Join(tmp, "flow.clj")
	if err := os.WriteFile(flowFile, []byte("{:slug :push-create-missing :name \"Push Create Missing\" :concurrency {:type :singleton :on-new-version :supersede} :flow '(let [input (flow/input)] input)}\n"), 0o644); err != nil {
		t.Fatalf("failed to write test flow file: %v", err)
	}
	step := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				"data": map[string]any{
					"flowSlug":    "push-create-missing",
					"savedDraft":  true,
					"flowVersion": 1,
				},
			})
		case "flows.validate":
			step++
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "not_found",
					"message": "Flow not found",
					"details": map[string]any{"flowSlug": "push-create-missing"},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": map[string]any{"message": "unexpected command"},
			})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "push",
		"--file", flowFile,
	)
	if err != nil {
		t.Fatalf("flows push should keep successful draft save when validation cannot find the new flow: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected put_draft + validate commands, got %d", step)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := e["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", e)
	}
	meta, _ := e["meta"].(map[string]any)
	if meta["validated"] != false || meta["validateSource"] != "draft" {
		t.Fatalf("expected validation warning metadata, got %#v", meta)
	}
	if _, ok := meta["validationWarning"].(string); !ok {
		t.Fatalf("expected validationWarning metadata, got %#v", meta)
	}
}

func TestFlowsPush_RejectsTargetLiveWithEducationalHint(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	flowFile := filepath.Join(tmp, "flow.clj")
	if err := os.WriteFile(flowFile, []byte("{:slug :push-live-target :name \"Push Live Target\" :concurrency {:type :singleton :on-new-version :supersede} :flow '(let [input (flow/input)] input)}\n"), 0o644); err != nil {
		t.Fatalf("failed to write test flow file: %v", err)
	}

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "push",
		"--file", flowFile,
		"--target", "live",
	)
	if err == nil {
		t.Fatalf("expected flows push --target live to fail")
	}
	combined := []byte(stdout + stderr)
	if !bytes.Contains(combined, []byte("--target live is not supported for flows push")) {
		t.Fatalf("expected educational target-live message, got:\n%s", string(combined))
	}
	if !bytes.Contains(combined, []byte("breyta flows release <slug>")) {
		t.Fatalf("expected release guidance in error, got:\n%s", string(combined))
	}
	if !bytes.Contains(combined, []byte("breyta flows promote <slug>")) {
		t.Fatalf("expected promote guidance in error, got:\n%s", string(combined))
	}
}

func TestFlowsBindingsTemplate_PrefillsCurrentBindings(t *testing.T) {
	t.Helper()
	statusCalled := false
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))

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

func TestResourcesSearch_UsesSearchEndpointAndQueryParams(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("q"); got != "transcript summary" {
			t.Fatalf("expected q=transcript summary, got %q", got)
		}
		if got := r.URL.Query().Get("type"); got != "result" {
			t.Fatalf("expected type=result, got %q", got)
		}
		if got := r.URL.Query().Get("content-sources"); got != "result,file" {
			t.Fatalf("expected content-sources=result,file, got %q", got)
		}
		if got := r.URL.Query().Get("storage-backend"); got != "platform" {
			t.Fatalf("expected storage-backend=platform, got %q", got)
		}
		if got := r.URL.Query().Get("storage-root"); got != "reports/acme" {
			t.Fatalf("expected storage-root=reports/acme, got %q", got)
		}
		if got := r.URL.Query().Get("path-prefix"); got != "exports/2026" {
			t.Fatalf("expected path-prefix=exports/2026, got %q", got)
		}
		if got := r.URL.Query().Get("mode"); got != "hybrid" {
			t.Fatalf("expected mode=hybrid, got %q", got)
		}
		if got := r.URL.Query().Get("keyword-mode"); got != "balanced" {
			t.Fatalf("expected keyword-mode=balanced, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "30" {
			t.Fatalf("expected limit=30, got %q", got)
		}
		if got := r.URL.Query().Get("offset"); got != "10" {
			t.Fatalf("expected offset=10, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "transcript summary",
			"items": []any{
				map[string]any{
					"uri":  "res://v1/ws/ws-acme/result/run/wf-123/flow-output",
					"type": "result",
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
		"resources", "search", "transcript summary",
		"--type", "result",
		"--content-sources", "result,file",
		"--storage-backend", "platform",
		"--storage-root", "reports/acme",
		"--path-prefix", "exports/2026",
		"--mode", "hybrid",
		"--keyword-mode", "balanced",
		"--limit", "30",
		"--offset", "10",
	)
	if err != nil {
		t.Fatalf("resources search failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", out)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["query"].(string); got != "transcript summary" {
		t.Fatalf("unexpected data.query: %q", got)
	}
}

func TestResourcesSearchIndexUpdate_PostsPayload(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/search-index" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("invalid request json: %v", err)
		}
		if got := body["op"]; got != "update" {
			t.Fatalf("expected op=update, got %#v", got)
		}
		if got := body["uri"]; got != "res://v1/ws/ws-acme/file/file-1" {
			t.Fatalf("unexpected uri: %#v", got)
		}
		searchIndex, _ := body["searchIndex"].(map[string]any)
		if searchIndex == nil {
			t.Fatalf("missing searchIndex in body: %#v", body)
		}
		if got := searchIndex["text"]; got != "A focused transcript summary" {
			t.Fatalf("unexpected searchIndex.text: %#v", got)
		}
		if got := searchIndex["sourceLabel"]; got != "user testing video" {
			t.Fatalf("unexpected searchIndex.sourceLabel: %#v", got)
		}
		if got := searchIndex["includeRawContent"]; got != true {
			t.Fatalf("unexpected searchIndex.includeRawContent: %#v", got)
		}
		tags, _ := searchIndex["tags"].([]any)
		if len(tags) != 2 || tags[0] != "testing" || tags[1] != "transcript" {
			t.Fatalf("unexpected tags: %#v", tags)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"operation":  "update",
			"uri":        body["uri"],
			"reindexed?": true,
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "search-index", "update", "res://v1/ws/ws-acme/file/file-1",
		"--text", "A focused transcript summary",
		"--source-label", "user testing video",
		"--tags", "testing,transcript",
		"--include-raw-content",
	)
	if err != nil {
		t.Fatalf("resources search-index update failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", out)
	}
}

func TestResourcesSearch_DefaultOutputIsCompact(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Fatalf("expected compact default limit=10, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "invoice",
			"items": []any{
				map[string]any{
					"uri":          "res://v1/ws/ws-acme/result/run/wf-123/flow-output",
					"type":         "result",
					"path":         "/very/long/internal/storage/path/persist/invoice-output.json",
					"content-type": "application/json",
					"snippet":      strings.Repeat("invoice summary ", 50),
					"details": map[string]any{
						"workflow-id": "wf-123",
						"flow-slug":   "invoice-reader",
						"step-id":     "summarize",
					},
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
		"resources", "search", "invoice",
	)
	if err != nil {
		t.Fatalf("resources search failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if data["outputView"] != "compact" {
		t.Fatalf("expected compact output view, got %#v", data["outputView"])
	}
	items, _ := data["items"].([]any)
	item, _ := items[0].(map[string]any)
	if _, ok := item["path"]; ok {
		t.Fatalf("expected compact item to omit path, got %#v", item)
	}
	if _, ok := item["content-type"]; ok {
		t.Fatalf("expected compact item to omit duplicate kebab content type, got %#v", item)
	}
	if item["displayName"] == "" || item["sourceLabel"] == "" || item["workflowId"] != "wf-123" || item["flowSlug"] != "invoice-reader" {
		t.Fatalf("unexpected compact item fields: %#v", item)
	}
	if item["hitRef"] != "resource:res://v1/ws/ws-acme/result/run/wf-123/flow-output" ||
		item["nextCommand"] != "breyta resources read 'res://v1/ws/ws-acme/result/run/wf-123/flow-output' --limit 5" {
		t.Fatalf("expected resource hit ref and next command, got %#v", item)
	}
	if got, _ := item["snippet"].(string); got == "" || len(got) >= len(strings.Repeat("invoice summary ", 50)) {
		t.Fatalf("expected truncated snippet, got %#v", item["snippet"])
	}
}

func TestResourcesList_UsesPickerStyleQueryParams(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("query"); got != "transcript" {
			t.Fatalf("expected query=transcript, got %q", got)
		}
		if got := r.URL.Query().Get("types"); got != "file,result" {
			t.Fatalf("expected types=file,result, got %q", got)
		}
		if got := r.URL.Query().Get("accept"); got != "text/*,application/json" {
			t.Fatalf("expected accept=text/*,application/json, got %q", got)
		}
		if got := r.URL.Query().Get("exclude-tier"); got != "ephemeral" {
			t.Fatalf("expected exclude-tier=ephemeral, got %q", got)
		}
		if got := r.URL.Query().Get("storage-backend"); got != "platform" {
			t.Fatalf("expected storage-backend=platform, got %q", got)
		}
		if got := r.URL.Query().Get("storage-root"); got != "reports/acme" {
			t.Fatalf("expected storage-root=reports/acme, got %q", got)
		}
		if got := r.URL.Query().Get("path-prefix"); got != "exports/2026" {
			t.Fatalf("expected path-prefix=exports/2026, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "1000" {
			t.Fatalf("expected limit=1000, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{
					"uri":  "res://v1/ws/ws-acme/result/blob/bucket/persist/transcript-a.json",
					"type": "result",
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
		"resources", "list",
		"--query", "transcript",
		"--types", "file,result",
		"--accept", "text/*,application/json",
		"--exclude-tier", "ephemeral",
		"--storage-backend", "platform",
		"--storage-root", "reports/acme",
		"--path-prefix", "exports/2026",
		"--limit", "1000",
	)
	if err != nil {
		t.Fatalf("resources list failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %+v", out)
	}
}

func TestResourcesUpload_UploadsLocalFileAndPrintsURI(t *testing.T) {
	const resourceURI = "res://v1/ws/ws-acme/file/uploaded-hero"
	var sawInit, sawDirect, sawComplete bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/files/uploads/init":
			sawInit = true
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["filename"] != "hero-card.png" {
				t.Fatalf("expected filename hero-card.png, got %#v", body["filename"])
			}
			if body["content-type"] != "image/png" {
				t.Fatalf("expected content-type image/png, got %#v", body["content-type"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"uri": resourceURI}})
		case "/api/files/uploads/direct":
			sawDirect = true
			if got := r.URL.Query().Get("uri"); got != resourceURI {
				t.Fatalf("expected direct upload uri %s, got %q", resourceURI, got)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "png-bytes" {
				t.Fatalf("expected uploaded body, got %q", string(body))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/api/files/uploads/complete":
			sawComplete = true
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["uri"] != resourceURI {
				t.Fatalf("expected complete uri %s, got %#v", resourceURI, body["uri"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"contentType": "image/png", "sizeBytes": 9}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(path, []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "upload", path,
		"--name", "hero-card.png",
		"--content-type", "image/png",
		"--print-uri",
	)
	if err != nil {
		t.Fatalf("resources upload failed: %v\n%s", err, stdout)
	}
	if strings.TrimSpace(stdout) != resourceURI {
		t.Fatalf("expected printed resource URI %q, got %q", resourceURI, stdout)
	}
	if !sawInit || !sawDirect || !sawComplete {
		t.Fatalf("expected init/direct/complete calls, got init=%v direct=%v complete=%v", sawInit, sawDirect, sawComplete)
	}
}

func TestResourcesRead_CompactsBlobByDefaultAndFullKeepsRawPayload(t *testing.T) {
	uri := "res://v1/ws/ws-acme/result/blob/output-json"
	longBody := strings.Repeat("payload ", 900)
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/content" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("uri"); got != uri {
			t.Fatalf("expected uri=%s, got %q", uri, got)
		}
		if r.URL.Query().Get("view") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uri":         uri,
				"contentType": "application/json",
				"body":        longBody,
			})
			return
		}
		if got := r.URL.Query().Get("view"); got != "summary" {
			t.Fatalf("expected default read view=summary, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "25" {
			t.Fatalf("expected default preview limit=25, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uri":         uri,
			"contentType": "application/json",
			"body":        longBody,
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "read", uri,
	)
	if err != nil {
		t.Fatalf("resources read failed: %v\n%s", err, stdout)
	}
	var compactOut map[string]any
	if err := json.Unmarshal([]byte(stdout), &compactOut); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	compactData, _ := compactOut["data"].(map[string]any)
	if _, ok := compactData["body"]; ok {
		t.Fatalf("expected compact read to omit raw body, got %#v", compactData)
	}
	if got, _ := compactData["preview"].(string); got == "" || len(got) >= len(longBody) {
		t.Fatalf("expected truncated preview, got %#v", compactData["preview"])
	}
	if compactData["truncated"] != true {
		t.Fatalf("expected truncated=true, got %#v", compactData["truncated"])
	}

	stdout, _, err = runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "read", uri,
		"--full",
	)
	if err != nil {
		t.Fatalf("resources read --full failed: %v\n%s", err, stdout)
	}
	var fullOut map[string]any
	if err := json.Unmarshal([]byte(stdout), &fullOut); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	fullData, _ := fullOut["data"].(map[string]any)
	if got, _ := fullData["body"].(string); got != longBody {
		t.Fatalf("expected --full to keep raw body, got %#v", fullData["body"])
	}
}

func TestResourcesRead_FallsBackToRunsGetForStepOutputRefs(t *testing.T) {
	uri := "res://v1/ws/ws-acme/result/run/wf-step/step/review/output"
	var sawResourceRead bool
	var capturedArgs map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/resources/content":
			sawResourceRead = true
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": "Access denied: not a workspace member",
			})
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "runs.get" {
				t.Fatalf("expected fallback runs.get, got %#v", body["command"])
			}
			capturedArgs, _ = body["args"].(map[string]any)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"run": map[string]any{
						"workflowId": "wf-step",
						"steps": []map[string]any{
							{
								"stepId": "review",
								"status": "completed",
								"output": map[string]any{"full": "nested-output"},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "read", uri,
		"--full",
	)
	if err != nil {
		t.Fatalf("resources read --full fallback failed: %v\n%s", err, stdout)
	}
	if !sawResourceRead {
		t.Fatalf("expected initial resources/content request")
	}
	if capturedArgs["includeStepResults"] != true || capturedArgs["stepId"] != "review" {
		t.Fatalf("expected fallback step payload request, got %#v", capturedArgs)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if data["full"] != "nested-output" {
		t.Fatalf("expected full fallback payload, got %#v", data)
	}
}

func TestResourcesRead_CompactsBinaryBlobWithoutRawPreview(t *testing.T) {
	uri := "res://v1/ws/ws-acme/result/blob/file-pdf"
	pdfBody := "%PDF-1.7\x00\x01binary"
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/content" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uri":         uri,
			"contentType": "application/pdf",
			"sizeBytes":   len([]byte(pdfBody)),
			"body":        pdfBody,
			"downloadUrl": "https://download.example/file.pdf",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "read", uri,
	)
	if err != nil {
		t.Fatalf("resources read failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if _, ok := data["preview"]; ok {
		t.Fatalf("expected binary compact output to omit raw preview, got %#v", data["preview"])
	}
	if data["shape"] != "binary" || data["binaryPreview"] != true || data["bodyOmitted"] != true {
		t.Fatalf("expected binary metadata, got %#v", data)
	}
	if got, _ := data["firstBytes"].(string); !strings.HasPrefix(got, "25 50 44 46") {
		t.Fatalf("expected PDF byte signature, got %q", got)
	}
	if data["downloadUrl"] != "https://download.example/file.pdf" {
		t.Fatalf("expected downloadUrl, got %#v", data["downloadUrl"])
	}
}
