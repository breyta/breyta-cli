package cli_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func jwtWithEmailForCLI(email string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"` + email + `"}`))
	return header + "." + payload + "."
}

func TestRunsList_SendsProfileIDFilter(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["profileId"] != "prof-1" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
			return
		}
		if got, _ := args["limit"].(float64); got != 10 {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing compact default limit"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"items": []any{}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "list",
		"--flow", "my-flow",
		"--installation-id", "prof-1",
	)
	if err != nil {
		t.Fatalf("runs list failed: %v\n%s", err, stdout)
	}
}

func TestAPICommand_LocalMembership403AutoBootstrapsAndRetries(t *testing.T) {
	commandCalls := 0
	bootstrapCalls := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/commands":
			commandCalls++
			if got := r.Header.Get("X-Breyta-Workspace"); got != "ws-acme" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "missing workspace header"})
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "flows.list" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "unexpected command"})
				return
			}
			if commandCalls == 1 {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Access denied: not a workspace member"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"items": []any{}},
			})
		case "/api/debug/workspace/bootstrap":
			bootstrapCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer user-dev" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing authorization"})
				return
			}
			if got := r.Header.Get("x-debug-user-id"); got != "user-dev" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing debug user"})
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["workspaceId"] != "ws-acme" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "wrong workspace"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success":     true,
				"workspaceId": "ws-acme",
				"created":     false,
				"member":      true,
				"role":        "admin",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "list",
		"--format", "json",
		"--limit", "1",
	)
	if err != nil {
		t.Fatalf("flows list failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if commandCalls != 2 {
		t.Fatalf("expected command retry after bootstrap, got %d command calls", commandCalls)
	}
	if bootstrapCalls != 1 {
		t.Fatalf("expected one local bootstrap call, got %d", bootstrapCalls)
	}
	if !strings.Contains(stdout, `"localWorkspaceBootstrap"`) {
		t.Fatalf("expected output metadata to mention local bootstrap:\n%s", stdout)
	}
}

func TestRunsList_SendsStructuredQueryFilters(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "my-flow" || args["profileId"] != "prof-1" || args["status"] != "failed" || args["version"] != float64(7) {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing structured filters"}})
			return
		}
		if args["query"] != "status:failed flow:my-flow installation:prof-1 version:7" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing canonical query"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"items": []any{}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "list",
		"--query", "status:failed flow:my-flow installation:prof-1 version:7",
	)
	if err != nil {
		t.Fatalf("runs list failed: %v\n%s", err, stdout)
	}
}

func TestRunsList_ExplicitFlagsOverrideQuery(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "override-flow" || args["profileId"] != "prof-9" || args["status"] != "completed" || args["version"] != float64(11) {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "explicit flags did not win"}})
			return
		}
		if args["query"] != "status:completed flow:override-flow installation:prof-9 version:11" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected canonical override query"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"items": []any{}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "list",
		"--query", "status:failed flow:my-flow installation:prof-1 version:7",
		"--flow", "override-flow",
		"--installation-id", "prof-9",
		"--status", "completed",
		"--version", "11",
	)
	if err != nil {
		t.Fatalf("runs list failed: %v\n%s", err, stdout)
	}
}

func TestRunsList_DedicatedFlowFlagPreservesExactValue(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != ":billing" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "dedicated flow slug was normalized"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"items": []any{}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "list",
		"--flow", ":billing",
	)
	if err != nil {
		t.Fatalf("runs list failed: %v\n%s", err, stdout)
	}
}

func TestRunsStart_SendsProfileID(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.start" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["profileId"] != "prof-2" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"started":    true,
				"workflowId": "wf-1",
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "start",
		"--flow", "my-flow",
		"--installation-id", "prof-2",
	)
	if err != nil {
		t.Fatalf("runs start failed: %v\n%s", err, stdout)
	}
}

func TestRunsStart_SendsInvocation(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.start" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["invocation"] != "import-orders" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing invocation"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"started": true, "workflowId": "wf-invocation-1"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "start",
		"--flow", "my-flow",
		"--invocation", "import-orders",
	)
	if err != nil {
		t.Fatalf("runs start failed: %v\n%s", err, stdout)
	}
}

func TestRunsStart_AllowsExplicitSource(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.start" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "my-flow" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["source"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=draft"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"started":    true,
				"workflowId": "wf-source-1",
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "start",
		"--flow", "my-flow",
		"--source", "draft",
	)
	if err != nil {
		t.Fatalf("runs start with --source failed: %v\n%s", err, stdout)
	}
}

func TestRunsStart_EmitsRunStartedTelemetry(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")
	t.Setenv("BREYTA_POSTHOG_API_KEY", "test-posthog-key")

	events := make(chan string, 4)
	posthog := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/capture/" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if event, _ := body["event"].(string); strings.TrimSpace(event) != "" {
			events <- event
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer posthog.Close()
	t.Setenv("BREYTA_POSTHOG_HOST", posthog.URL)

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.start" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"workflowId": "wf-telemetry-1"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", jwtWithEmailForCLI("user@example.com"),
		"runs", "start",
		"--flow", "my-flow",
	)
	if err != nil {
		t.Fatalf("runs start failed: %v\n%s", err, stdout)
	}

	select {
	case event := <-events:
		if event != "cli_run_started" {
			t.Fatalf("expected cli_run_started event, got %q", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected cli_run_started telemetry event")
	}
}

func TestFlowsDraftRun_EmitsRunStartedTelemetry(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")
	t.Setenv("BREYTA_POSTHOG_API_KEY", "test-posthog-key")

	events := make(chan string, 4)
	posthog := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/capture/" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if event, _ := body["event"].(string); strings.TrimSpace(event) != "" {
			events <- event
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer posthog.Close()
	t.Setenv("BREYTA_POSTHOG_HOST", posthog.URL)

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.start" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["source"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=draft"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"workflowId": "wf-telemetry-2"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", jwtWithEmailForCLI("user@example.com"),
		"flows", "draft", "run", "my-flow",
	)
	if err != nil {
		t.Fatalf("flows draft run failed: %v\n%s", err, stdout)
	}

	select {
	case event := <-events:
		if event != "cli_run_started" {
			t.Fatalf("expected cli_run_started event, got %q", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected cli_run_started telemetry event")
	}
}

func TestFlowsInstallations_Create_UsesFlowsInstallationsCreateCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.create" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "my-app" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["name"] != "Instance A" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing name"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"instance": map[string]any{"profileId": "prof-xyz"}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "create", "my-app",
		"--name", "Instance A",
	)
	if err != nil {
		t.Fatalf("flows installations create failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallations_Create_AllowsPublicInstallSourceRefs(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.create" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "my-app" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["sourceWorkspaceId"] != "ws-source" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing sourceWorkspaceId"}})
			return
		}
		if args["sourceFlowSlug"] != "public-source-flow" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing sourceFlowSlug"}})
			return
		}
		if args["enabled"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing enabled"}})
			return
		}
		if args["localPrivateTest"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing localPrivateTest"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"instance": map[string]any{"profileId": "prof-public"}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "create", "my-app",
		"--source-workspace-id", "ws-source",
		"--source-flow-slug", "public-source-flow",
		"--enable",
		"--local-private-test",
	)
	if err != nil {
		t.Fatalf("flows installations create with source refs failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallationsCreateHelpDocumentsLivePrerequisitesAndDefaults(t *testing.T) {
	stdout, _, err := runCLIArgs(t,
		"flows", "installations", "create", "--help",
	)
	if err != nil {
		t.Fatalf("flows installations create --help failed: %v\n%s", err, stdout)
	}
	for _, want := range []string{
		"active live version",
		"breyta flows release-check <flow-slug>",
		"zero-setup installations",
		"--local-private-test",
		"active source version",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected help to contain %q\n%s", want, stdout)
		}
	}
}

func TestFlowsInstallations_Get_UsesFlowsInstallationsGetCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["profileId"] != "prof-abc" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"installation": map[string]any{"profileId": "prof-abc"}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "get", "prof-abc",
	)
	if err != nil {
		t.Fatalf("flows installations get failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallations_SetInputs_UsesFlowsInstallationsSetInputsCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.set_inputs" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["profileId"] != "prof-abc" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
			return
		}
		inputs, _ := args["inputs"].(map[string]any)
		if inputs["region"] != "EU" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing inputs.region"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"instance": map[string]any{"profileId": "prof-abc"}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "set-inputs", "prof-abc",
		"--input", `{"region":"EU"}`,
	)
	if err != nil {
		t.Fatalf("flows installations set-inputs failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallations_Configure_SendsSchedules(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.set_inputs" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["profileId"] != "prof-sched" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
			return
		}
		schedules, _ := args["schedules"].(map[string]any)
		review, _ := schedules["scheduled-review"].(map[string]any)
		if review["enabled"] != false || review["preset"] != "weekly" || review["timezone"] != "Europe/Oslo" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing schedule settings"}})
			return
		}
		resets, _ := args["scheduleResets"].(map[string]any)
		if resets["scheduled-review"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing schedule reset"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"instance": map[string]any{"profileId": "prof-sched"}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "configure", "prof-sched",
		"--schedules", `{"scheduled-review":{"enabled":true,"preset":"weekly","timezone":"Europe/Oslo"}}`,
		"--schedule-disable", "scheduled-review",
		"--schedule-reset", "scheduled-review",
	)
	if err != nil {
		t.Fatalf("flows installations configure with --schedules failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallations_Configure_SupportsBindingAndActivationSetFlags(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.set_inputs" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["profileId"] != "prof-blob" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
			return
		}
		inputs, _ := args["inputs"].(map[string]any)
		if inputs["folder"] != "https://drive.google.com/drive/folders/demo" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing inputs.folder"}})
			return
		}
		bindings, _ := args["bindings"].(map[string]any)
		archive, _ := bindings["archive"].(map[string]any)
		if archive["connectionId"] != "conn-storage" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing bindings.archive.connectionId"}})
			return
		}
		config, _ := archive["config"].(map[string]any)
		if config["root"] != "customer-a/archive" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing bindings.archive.config.root"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"instance": map[string]any{"profileId": "prof-blob"}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "configure", "prof-blob",
		"--set", "activation.folder=https://drive.google.com/drive/folders/demo",
		"--set", "archive.conn=conn-storage",
		"--set", "archive.root=customer-a/archive",
	)
	if err != nil {
		t.Fatalf("flows installations configure with --set failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallations_Configure_SetFlagsTreatBindingFieldsCaseInsensitively(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.set_inputs" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		bindings, _ := args["bindings"].(map[string]any)
		archive, _ := bindings["archive"].(map[string]any)
		config, _ := archive["config"].(map[string]any)
		if config["prefix"] != "customer-a/archive" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing bindings.archive.config.prefix"}})
			return
		}
		if _, ok := config["root"]; ok {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected bindings.archive.config.root"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"instance": map[string]any{"profileId": "prof-blob"}}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "configure", "prof-blob",
		"--set", "archive.PREFIX=customer-a/archive",
	)
	if err != nil {
		t.Fatalf("flows installations configure with mixed-case prefix failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigure_UsesCanonicalCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		inputs, _ := args["inputs"].(map[string]any)
		if inputs["conn-api"] != "conn-123" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing conn-api"}})
			return
		}
		if inputs["form-region"] != "EU" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing form-region"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"profileId": "prof-draft"}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "flow-configure",
		"--set", "api.conn=conn-123",
		"--set", "activation.region=EU",
	)
	if err != nil {
		t.Fatalf("flows configure failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigure_SendsSchedules(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		schedules, _ := args["schedules"].(map[string]any)
		review, _ := schedules["scheduled-review"].(map[string]any)
		if review["enabled"] != false || review["preset"] != "monthly" || review["dayOfMonth"] != "last" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing schedules"}})
			return
		}
		daily, _ := schedules["daily-review"].(map[string]any)
		if daily["enabled"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing schedule enable"}})
			return
		}
		resets, _ := args["scheduleResets"].(map[string]any)
		if resets["monthly-review"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing schedule reset"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"profileId": "prof-draft"}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "flow-configure",
		"--schedules", `{"scheduled-review":{"enabled":false,"preset":"monthly","dayOfMonth":"last"}}`,
		"--schedule-enable", "daily-review",
		"--schedule-reset", "monthly-review",
	)
	if err != nil {
		t.Fatalf("flows configure with --schedules failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigure_LiveTarget_UsesCanonicalCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["target"] != "live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing target=live"}})
			return
		}
		inputs, _ := args["inputs"].(map[string]any)
		if inputs["conn-api"] != "conn-live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing conn-api"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"profileId": "prof-live"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "flow-configure",
		"--target", "live",
		"--set", "api.conn=conn-live",
	)
	if err != nil {
		t.Fatalf("flows configure --target live failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigure_VersionRequiresLiveTarget(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "configure", "flow-configure",
		"--set", "api.conn=conn-123",
		"--version", "3",
	)
	if err == nil {
		t.Fatalf("expected flows configure to fail without --target live when --version is provided")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--version requires --target live") {
		t.Fatalf("expected --version/--target guidance, got:\n%s", combined)
	}
}

func TestFlowsConfigure_LiveTargetFromDraft_UsesCanonicalCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" || args["target"] != "live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug/target"}})
			return
		}
		if args["fromDraft"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing fromDraft=true"}})
			return
		}
		if _, ok := args["inputs"]; ok {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "inputs should be omitted"}})
			return
		}
		if args["version"] != float64(7) {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing version=7"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"profileId": "prof-live", "target": "live", "version": 7},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "flow-configure",
		"--target", "live",
		"--from-draft",
		"--version", "7",
	)
	if err != nil {
		t.Fatalf("flows configure --target live --from-draft failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigureShow_UsesDraftProfileStatusByDefault(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "profiles.status" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["profileType"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected draft profileType"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"profileType": "draft"}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "show", "flow-configure",
	)
	if err != nil {
		t.Fatalf("flows configure show failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigureShow_LiveTarget_UsesProdProfileStatus(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "profiles.status" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["profileType"] != "prod" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected prod profileType"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"profileType": "prod"}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "show", "flow-configure",
		"--target", "live",
	)
	if err != nil {
		t.Fatalf("flows configure show --target live failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigureCheck_DefaultTarget(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.configure.check" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if _, ok := args["target"]; ok {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "target should be omitted by default"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-configure", "target": "draft", "ready": true},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "check", "flow-configure",
	)
	if err != nil {
		t.Fatalf("flows configure check failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigureCheck_LiveTarget(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.configure.check" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["target"] != "live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected target live"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-configure", "target": "live", "ready": false},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "check", "flow-configure",
		"--target", "live",
	)
	if err != nil {
		t.Fatalf("flows configure check --target live failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigureCheck_LiveTargetWithVersion(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.configure.check" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-configure" || args["target"] != "live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug/target"}})
			return
		}
		if args["version"] != float64(9) {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing version=9"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-configure", "target": "live", "version": 9, "ready": true},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "check", "flow-configure",
		"--target", "live",
		"--version", "9",
	)
	if err != nil {
		t.Fatalf("flows configure check --target live --version failed: %v\n%s", err, stdout)
	}
}

func TestFlowsConfigureCheck_VersionRequiresLiveTarget(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "configure", "check", "flow-configure",
		"--version", "9",
	)
	if err == nil {
		t.Fatalf("expected flows configure check to fail without --target live when --version is provided")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--version requires --target live") {
		t.Fatalf("expected --version/--target guidance, got:\n%s", combined)
	}
}

func TestFlowsConfigureSuggest_DefaultTarget_UsesTemplateStatusAndConnections(t *testing.T) {
	commandCalls := 0
	connectionCalls := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			command, _ := body["command"].(string)
			args, _ := body["args"].(map[string]any)
			switch command {
			case "profiles.template":
				commandCalls++
				if args["flowSlug"] != "flow-configure" {
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
					return
				}
				if args["profileType"] != "draft" {
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected draft profileType"}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"flowSlug": "flow-configure",
						"requirements": []any{
							map[string]any{"slot": ":api", "kind": "connection", "type": "http-api"},
							map[string]any{"kind": "form", "fields": []any{
								map[string]any{"key": ":region", "required": true},
							}},
						},
					},
				})
			case "profiles.status":
				commandCalls++
				if args["profileType"] != "draft" {
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected draft profileType"}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"bindingValues": map[string]any{},
						"activation": map[string]any{
							"region": map[string]any{"set": false},
						},
					},
				})
			default:
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			}
		case "/api/connections":
			connectionCalls++
			if r.URL.Query().Get("type") != "http-api" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "expected type=http-api"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{"connection-id": "conn-api-1", "name": "Primary API"},
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
		"flows", "configure", "suggest", "flow-configure",
	)
	if err != nil {
		t.Fatalf("flows configure suggest failed: %v\n%s", err, stdout)
	}
	if commandCalls != 2 {
		t.Fatalf("expected 2 command calls, got %d", commandCalls)
	}
	if connectionCalls != 1 {
		t.Fatalf("expected 1 connections call, got %d", connectionCalls)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if data["target"] != "draft" {
		t.Fatalf("expected target=draft, got %#v", data["target"])
	}
	setArgs, _ := data["suggestedSetArgs"].([]any)
	if len(setArgs) != 1 || setArgs[0] != "api.conn=conn-api-1" {
		t.Fatalf("expected suggestedSetArgs with api.conn=conn-api-1, got %#v", setArgs)
	}
	missingActivation, _ := data["missingActivationInputs"].([]any)
	if len(missingActivation) != 1 || missingActivation[0] != "region" {
		t.Fatalf("expected missingActivationInputs=[region], got %#v", missingActivation)
	}
}

func TestFlowsConfigureSuggest_LiveTarget_UsesProdProfileType(t *testing.T) {
	commandCalls := 0
	connectionCalls := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			command, _ := body["command"].(string)
			args, _ := body["args"].(map[string]any)
			switch command {
			case "profiles.template":
				commandCalls++
				if args["profileType"] != "prod" {
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected prod profileType"}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"flowSlug": "flow-configure",
						"requirements": []any{
							map[string]any{"slot": ":llm", "kind": "connection", "type": "llm-provider"},
						},
					},
				})
			case "profiles.status":
				commandCalls++
				if args["profileType"] != "prod" {
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected prod profileType"}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"bindingValues": map[string]any{"llm": "conn-existing"},
						"activation":    map[string]any{},
					},
				})
			default:
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			}
		case "/api/connections":
			connectionCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{"connection-id": "conn-existing", "name": "OpenAI"},
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
		"flows", "configure", "suggest", "flow-configure",
		"--target", "live",
	)
	if err != nil {
		t.Fatalf("flows configure suggest --target live failed: %v\n%s", err, stdout)
	}
	if commandCalls != 2 {
		t.Fatalf("expected 2 command calls, got %d", commandCalls)
	}
	if connectionCalls != 1 {
		t.Fatalf("expected 1 connections call, got %d", connectionCalls)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if data["target"] != "live" {
		t.Fatalf("expected target=live, got %#v", data["target"])
	}
	if data["profileType"] != "prod" {
		t.Fatalf("expected profileType=prod, got %#v", data["profileType"])
	}
	suggested, _ := data["suggestedSetArgs"].([]any)
	if len(suggested) != 0 {
		t.Fatalf("expected no suggested set args when already configured, got %#v", suggested)
	}
}

func TestConnectionsTest_All(t *testing.T) {
	var bulkCalls int
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/connections/test" && r.Method == http.MethodPost:
			bulkCalls++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["all"] != true {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing all=true"}})
				return
			}
			if body["onlyFailing"] != false {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "expected onlyFailing=false"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{"connection-id": "conn-1", "success": true},
					map[string]any{"connection-id": "conn-2", "success": true},
				},
				"summary": map[string]any{"total": 2, "tested": 2, "ok": 2, "failed": 0},
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
		"connections", "test", "--all",
	)
	if err != nil {
		t.Fatalf("connections test --all failed: %v\n%s", err, stdout)
	}
	if bulkCalls != 1 {
		t.Fatalf("expected one bulk test call, got %d", bulkCalls)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	summary, _ := data["summary"].(map[string]any)
	if summary["failed"] != float64(0) {
		t.Fatalf("expected failed=0, got %#v", summary["failed"])
	}
}

func TestConnectionsTest_AllOnlyFailing_ReturnsFailureEnvelope(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/connections/test" && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["all"] != true || body["onlyFailing"] != true {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "expected all=true onlyFailing=true"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{"connection-id": "conn-bad", "success": false, "message": "invalid credentials"},
				},
				"summary": map[string]any{"total": 2, "tested": 2, "ok": 1, "failed": 1},
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
		"connections", "test", "--all", "--only-failing",
	)
	if err == nil {
		t.Fatalf("expected connections test --all --only-failing to fail when one connection fails")
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	ok, _ := out["ok"].(bool)
	if ok {
		t.Fatalf("expected ok=false output, got %#v", out["ok"])
	}
	errorObj, _ := out["error"].(map[string]any)
	if errorObj["code"] != "connection_tests_failed" {
		t.Fatalf("expected connection_tests_failed code, got %#v", errorObj["code"])
	}
	details, _ := errorObj["details"].(map[string]any)
	items, _ := details["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected only failing item in details, got %#v", items)
	}
}

func TestConnectionsUsages_API(t *testing.T) {
	var called bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/connections/usages" {
			http.NotFound(w, r)
			return
		}
		called = true
		if r.URL.Query().Get("connection-id") != "conn-1" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing connection-id query"}})
			return
		}
		if r.URL.Query().Get("flow-slug") != "my-flow" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing flow-slug query"}})
			return
		}
		if r.URL.Query().Get("only-connected") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing only-connected query"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{
					"connection-id": "conn-1",
					"usage-count":   2,
					"usages": []any{
						map[string]any{"flowSlug": "my-flow", "target": "draft"},
						map[string]any{"flowSlug": "my-flow", "target": "live"},
					},
				},
			},
			"summary": map[string]any{"connections": 1, "connected": 1, "unconnected": 0, "bindings": 2},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "usages",
		"--connection-id", "conn-1",
		"--flow", "my-flow",
		"--only-connected",
	)
	if err != nil {
		t.Fatalf("connections usages failed: %v\n%s", err, stdout)
	}
	if !called {
		t.Fatalf("expected /api/connections/usages to be called")
	}
}

func TestConnectionsCleanupUnused_API_Preview(t *testing.T) {
	var called bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		called = true
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "connections.unused.cleanup" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if len(args) != 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected empty args for preview"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"preview": true,
				"summary": map[string]any{"unusedConnections": 2, "deletedConnections": 0},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "cleanup-unused",
	)
	if err != nil {
		t.Fatalf("connections cleanup-unused preview failed: %v\n%s", err, stdout)
	}
	if !called {
		t.Fatalf("expected /api/commands to be called")
	}
}

func TestConnectionsCleanupUnused_API_Apply(t *testing.T) {
	var called bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		called = true
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "connections.unused.cleanup" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["apply"] != true {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing apply=true"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"applied": true,
				"summary": map[string]any{"deletedConnections": 2, "deletedSecrets": 1},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "cleanup-unused", "--apply",
	)
	if err != nil {
		t.Fatalf("connections cleanup-unused apply failed: %v\n%s", err, stdout)
	}
	if !called {
		t.Fatalf("expected /api/commands to be called")
	}
}

func TestConnectionsDelete_InUse_ReturnsHintDetails(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/connections/conn-in-use" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   "Connection is still bound to flow profiles",
			"details": map[string]any{"hint": "Unset or move this connection first"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "delete", "conn-in-use",
	)
	if err == nil {
		t.Fatalf("expected connections delete to fail for in-use connection")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	ok, _ := out["ok"].(bool)
	if ok {
		t.Fatalf("expected ok=false output")
	}
	data, _ := out["data"].(map[string]any)
	details, _ := data["details"].(map[string]any)
	if details["hint"] != "Unset or move this connection first" {
		t.Fatalf("expected delete hint in response details, got %#v", details)
	}
}

func TestSecretsUsages_API(t *testing.T) {
	var called bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/secrets/usages" {
			http.NotFound(w, r)
			return
		}
		called = true
		if r.URL.Query().Get("secret-id") != "sec-1" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing secret-id query"}})
			return
		}
		if r.URL.Query().Get("flow-slug") != "my-flow" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing flow-slug query"}})
			return
		}
		if r.URL.Query().Get("only-connected") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing only-connected query"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{
					"secret-id":   "sec-1",
					"usage-count": 2,
					"usages": []any{
						map[string]any{"flowSlug": "my-flow", "target": "draft"},
						map[string]any{"flowSlug": "my-flow", "target": "live"},
					},
				},
			},
			"summary": map[string]any{"secrets": 1, "connected": 1, "unconnected": 0, "bindings": 2},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"secrets", "usages",
		"--secret-id", "sec-1",
		"--flow", "my-flow",
		"--only-connected",
	)
	if err != nil {
		t.Fatalf("secrets usages failed: %v\n%s", err, stdout)
	}
	if !called {
		t.Fatalf("expected /api/secrets/usages to be called")
	}
}

func TestSecretsDelete_InUse_ReturnsHintDetails(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/secrets/sec-in-use" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   "Secret is still bound to flow profiles",
			"details": map[string]any{"hint": "Unset or move this secret first"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"secrets", "delete", "sec-in-use",
	)
	if err == nil {
		t.Fatalf("expected secrets delete to fail for in-use secret")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	ok, _ := out["ok"].(bool)
	if ok {
		t.Fatalf("expected ok=false output")
	}
	data, _ := out["data"].(map[string]any)
	details, _ := data["details"].(map[string]any)
	if details["hint"] != "Unset or move this secret first" {
		t.Fatalf("expected delete hint in response details, got %#v", details)
	}
}

func TestFlowsInstallations_List_All_SendsAllFlag(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "my-app" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["all"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing all"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "my-app", "items": []any{}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "list", "my-app",
		"--all",
	)
	if err != nil {
		t.Fatalf("flows installations list --all failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallations_List_AllowsPublicInstallSourceRefs(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "public-flow" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["sourceWorkspaceId"] != "ws-public" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing sourceWorkspaceId"}})
			return
		}
		if args["sourceFlowSlug"] != "public-flow" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing sourceFlowSlug"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-consumer",
			"data":        map[string]any{"flowSlug": "public-flow", "items": []any{}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-consumer",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "list", "public-flow",
		"--source-workspace-id", "ws-public",
		"--source-flow-slug", "public-flow",
	)
	if err != nil {
		t.Fatalf("flows installations list with source refs failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallations_StatsAndEventsUseCreatorCommands(t *testing.T) {
	seen := map[string]bool{}
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "public-flow" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		switch body["command"] {
		case "flows.installations.stats":
			seen["stats"] = true
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"totalInstalls": 1}})
		case "flows.installations.events":
			seen["events"] = true
			if args["limit"] != float64(10) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing limit"}})
				return
			}
			if args["since"] != "7d" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing since"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"items": []any{}}})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	if stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "stats", "public-flow",
	); err != nil {
		t.Fatalf("flows installations stats failed: %v\n%s", err, stdout)
	}
	if stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "events", "public-flow",
		"--limit", "10",
		"--since", "7d",
	); err != nil {
		t.Fatalf("flows installations events failed: %v\n%s", err, stdout)
	}
	if !seen["stats"] || !seen["events"] {
		t.Fatalf("expected stats/events commands, got %#v", seen)
	}
}

func TestFlowsInstallations_Delete_UsesFlowsInstallationsDeleteCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.delete" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["profileId"] != "prof-del" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"deleted": true}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "delete", "prof-del",
	)
	if err != nil {
		t.Fatalf("flows installations delete failed: %v\n%s", err, stdout)
	}
}

func TestFlowsDelete_UsesFlowsDeleteCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.delete" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-del" || args["yes"] != true || args["force"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing delete args"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"deleted": true}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "delete", "flow-del",
		"--yes",
		"--force",
		"--timeout", "45s",
	)
	if err != nil {
		t.Fatalf("flows delete failed: %v\n%s", err, stdout)
	}
}

func TestFlowsDelete_TimeoutFlagBoundsRequest(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"deleted": true}})
	}))
	defer srv.Close()

	start := time.Now()
	_, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "delete", "flow-del",
		"--yes",
		"--timeout", "20ms",
	)
	if err == nil {
		t.Fatal("expected flows delete to fail when the timeout expires")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected timeout to bound request quickly, elapsed=%s", elapsed)
	}
}

func TestFlowsRelease_UsesCanonicalCommand(t *testing.T) {
	step := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		switch body["command"] {
		case "flows.release":
			step++
			if args["flowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "activeVersion": 3},
			})
		case "flows.promote":
			step++
			if args["flowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			if args["target"] != "live" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing promote target"}})
				return
			}
			if args["version"] != float64(3) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing promote version"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "profileId": "prof-live", "target": "live"},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "release", "flow-release",
	)
	if err != nil {
		t.Fatalf("flows release failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected release + promote commands, got %d", step)
	}
}

func TestFlowsRelease_SendsReleaseNoteAndHints(t *testing.T) {
	step := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		switch body["command"] {
		case "flows.release":
			step++
			if args["releaseNote"] != "## Summary\n\nFixed retries." {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing releaseNote"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "activeVersion": 7},
			})
		case "flows.promote":
			step++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "profileId": "prof-live", "target": "live"},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "release", "flow-release",
		"--release-note", "## Summary\n\nFixed retries.",
	)
	if err != nil {
		t.Fatalf("flows release with --release-note failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected release + promote commands, got %d", step)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	hints, _ := out["_hints"].([]any)
	if len(hints) == 0 {
		t.Fatalf("expected _hints in output, got %#v", out["_hints"])
	}
	if hints[0] != "breyta flows diff flow-release" {
		t.Fatalf("unexpected first hint: %#v", hints[0])
	}
	if hints[2] != "breyta flows versions update flow-release --version 7 --release-note-file ./release-note.md" {
		t.Fatalf("unexpected version update hint: %#v", hints[2])
	}
}

func TestFlowsRelease_DefaultInstallEmitsTelemetry(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")
	t.Setenv("BREYTA_POSTHOG_API_KEY", "test-posthog-key")

	events := make(chan string, 8)
	posthog := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/capture/" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if event, _ := body["event"].(string); strings.TrimSpace(event) != "" {
			events <- event
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer posthog.Close()
	t.Setenv("BREYTA_POSTHOG_HOST", posthog.URL)
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		switch body["command"] {
		case "flows.release":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "activeVersion": 3},
			})
		case "flows.promote":
			if args["version"] != float64(3) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing promote version"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "profileId": "prof-live", "target": "live"},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", jwtWithEmailForCLI("user@example.com"),
		"flows", "release", "flow-release",
	)
	if err != nil {
		t.Fatalf("flows release failed: %v\n%s", err, stdout)
	}

	want := map[string]bool{
		"cli_flow_released": false,
		"cli_flow_promoted": false,
	}
	deadline := time.After(2 * time.Second)
	for !(want["cli_flow_released"] && want["cli_flow_promoted"]) {
		select {
		case event := <-events:
			if _, ok := want[event]; ok {
				want[event] = true
			}
		case <-deadline:
			t.Fatalf("expected release/promote telemetry events, got released=%v promoted=%v", want["cli_flow_released"], want["cli_flow_promoted"])
		}
	}
}

func TestFlowsRelease_EmitsLiveVerificationHint(t *testing.T) {
	step := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		switch body["command"] {
		case "flows.release":
			step++
			if args["flowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "activeVersion": 3},
			})
		case "flows.promote":
			step++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "profileId": "prof-live", "target": "live"},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "release", "flow-release",
	)
	if err != nil {
		t.Fatalf("flows release failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected release + promote commands, got %d", step)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	verifyHint, _ := meta["verifyHint"].(string)
	if !strings.Contains(verifyHint, "flows show <slug> --target live") {
		t.Fatalf("expected verifyHint to mention flows show live target, got: %q", verifyHint)
	}
	verifyCommands, _ := meta["verifyCommands"].([]any)
	if len(verifyCommands) != 2 {
		t.Fatalf("expected two verify commands, got %#v", meta["verifyCommands"])
	}
	if verifyCommands[0] != "breyta flows show flow-release --target live" {
		t.Fatalf("unexpected first verify command: %#v", verifyCommands[0])
	}
	if verifyCommands[1] != "breyta flows run flow-release --target live --wait" {
		t.Fatalf("unexpected second verify command: %#v", verifyCommands[1])
	}
}

func TestFlowsDiff_UsesCanonicalCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.diff" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" || args["from"] != "live" || args["to"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected diff args"}})
			return
		}
		if args["view"] != "summary" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing view=summary"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-release", "changed": true, "diff": "@@ -1 +1 @@\n-old\n+new"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "diff", "flow-release",
	)
	if err != nil {
		t.Fatalf("flows diff failed: %v\n%s", err, stdout)
	}
}

func TestFlowsDiff_FileDefaultsToDraftToFile(t *testing.T) {
	tmp := t.TempDir()
	flowPath := filepath.Join(tmp, "flow-release.clj")
	if err := os.WriteFile(flowPath, []byte("{:slug :flow-release :flow '(do :local)}\n"), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		if body["command"] != "flows.diff" ||
			args["flowSlug"] != "flow-release" ||
			args["from"] != "draft" ||
			args["to"] != "file" ||
			args["fileLiteral"] != "{:slug :flow-release :flow '(do :local)}\n" ||
			args["fileLabel"] != "flow-release.clj" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected file diff args", "details": args}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-release", "changed": true},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "diff", "flow-release",
		"--file", flowPath,
	)
	if err != nil {
		t.Fatalf("flows diff --file failed: %v\n%s", err, stdout)
	}
}

func TestFlowsDiff_FullRequestsUnifiedDiff(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		if body["command"] != "flows.diff" || args["view"] != "full" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected flows.diff view=full"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-release", "changed": true, "diff": "@@ -1 +1 @@\n-old\n+new"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "diff", "flow-release",
		"--full",
	)
	if err != nil {
		t.Fatalf("flows diff --full failed: %v\n%s", err, stdout)
	}
}

func TestFlowsRelease_SkipPromoteInstallations_PromotesLiveOnly(t *testing.T) {
	step := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		switch body["command"] {
		case "flows.release":
			step++
			if args["flowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "activeVersion": 3},
			})
		case "flows.promote":
			step++
			if args["scope"] != "live" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing scope=live"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "profileId": "prof-live", "target": "live", "scope": "live"},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "release", "flow-release",
		"--skip-promote-installations",
	)
	if err != nil {
		t.Fatalf("flows release --skip-promote-installations failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected release + live promote with --skip-promote-installations, got %d commands", step)
	}
}

func TestFlowsRun_UsesCanonicalCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.run" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["target"] != "live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "installation runs should default to live target"}})
			return
		}
		if args["installationId"] != "prof-123" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing installationId"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"run": map[string]any{"workflowId": "wf-2", "status": "running"}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "flow-release",
		"--installation-id", "prof-123",
	)
	if err != nil {
		t.Fatalf("flows run failed: %v\n%s", err, stdout)
	}
}

func TestFlowsRun_SendsInvocation(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.run" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" || args["target"] != "draft" || args["invocation"] != "import-orders" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing invocation payload"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"run": map[string]any{"workflowId": "wf-invocation-2", "status": "running"}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "flow-release",
		"--invocation-id", "import-orders",
	)
	if err != nil {
		t.Fatalf("flows run failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInterfacesList_ReadsFlowInterfacesMetadata(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" || args["source"] != "draft" || args["includeFlowLiteral"] != false {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected flows.get args"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flow": map[string]any{
					"flowSlug": "flow-release",
					"invocations": map[string]any{
						"default": map[string]any{"inputs": []any{map[string]any{"name": "domain", "type": "text"}}},
					},
					"interfaces": map[string]any{
						"manual": []any{map[string]any{"id": "run", "label": "Run enrichment", "invocation": "default"}},
						"http":   []any{map[string]any{"id": "enrich", "invocation": "default", "method": "post", "path": "/enrich", "auth": "workspace-api-auth"}},
						"webhook": []any{map[string]any{"id": "stripe", "eventName": "billing.stripe.webhook", "invocation": "default",
							"auth": map[string]any{"type": "stripe-signature", "secretRef": "stripe-webhook-secret"}}},
						"mcp": []any{map[string]any{"tool-name": "enrich_company", "invocation": "default"}},
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
		"flows", "interfaces", "list", "flow-release",
	)
	if err != nil {
		t.Fatalf("flows interfaces list failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	items, _ := out.Data["items"].([]any)
	if len(items) != 4 {
		t.Fatalf("expected 4 interfaces, got %#v", out.Data["items"])
	}
	first, _ := items[0].(map[string]any)
	if first["family"] != "manual" || first["id"] != "run" || first["label"] != "Run enrichment" || first["invocationId"] != "default" {
		t.Fatalf("unexpected first interface item: %#v", first)
	}
	second, _ := items[2].(map[string]any)
	if second["family"] != "webhook" || second["id"] != "stripe" || second["eventName"] != "billing.stripe.webhook" {
		t.Fatalf("unexpected webhook interface item: %#v", second)
	}
	endpoint, _ := second["endpoint"].(map[string]any)
	if endpoint["method"] != "POST" || endpoint["auth"] != "workspace-api-auth" || endpoint["url"] != srv.URL+"/api/flows/flow-release/interfaces/draft/stripe" {
		t.Fatalf("expected source webhook endpoint to use interface id, got %#v", endpoint)
	}
	if endpoint["alternateUrl"] != srv.URL+"/api/workspaces/ws-acme/flows/flow-release/interfaces/draft/stripe" {
		t.Fatalf("expected workspace-scoped alternate endpoint, got %#v", endpoint)
	}
}

func TestFlowsInterfacesBareSlugSuggestsListSubcommand(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://localhost:65535",
		"--token", "user-dev",
		"flows", "interfaces", "reliability-wait-timeout-probe",
	)
	if err == nil {
		t.Fatalf("expected bare interfaces slug to fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "did you mean `breyta flows interfaces list reliability-wait-timeout-probe`") {
		t.Fatalf("expected list suggestion, got stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestFlowsInterfacesList_TargetLiveUsesAuthorLiveEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" || args["source"] != "active" || args["includeFlowLiteral"] != false {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected flows.get args", "details": args}})
			return
		}
		if _, ok := args["version"]; ok {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected version", "details": args}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flow": map[string]any{
					"flowSlug":    "flow-release",
					"invocations": map[string]any{"default": map[string]any{}},
					"interfaces": map[string]any{
						"http": []any{map[string]any{"id": "enrich", "invocation": "default", "method": "post", "path": "/enrich"}},
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
		"flows", "interfaces", "list", "flow-release",
		"--target", "live",
	)
	if err != nil {
		t.Fatalf("flows interfaces list --target live failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	if out.Data["target"] != "live" {
		t.Fatalf("expected live target metadata, got %#v", out.Data)
	}
	items, _ := out.Data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one interface, got %#v", out.Data["items"])
	}
	item, _ := items[0].(map[string]any)
	endpoint, _ := item["endpoint"].(map[string]any)
	if endpoint["url"] != srv.URL+"/api/flows/flow-release/interfaces/live/enrich" {
		t.Fatalf("expected live source endpoint, got %#v", endpoint)
	}
	if endpoint["alternateUrl"] != srv.URL+"/api/workspaces/ws-acme/flows/flow-release/interfaces/live/enrich" {
		t.Fatalf("expected workspace-scoped alternate endpoint, got %#v", endpoint)
	}
}

func TestFlowsMetrics_CallsMetricsCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.invocations.metrics" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" || args["entrypointId"] != "enrich" || args["installationId"] != "prof-live" || args["kind"] != "http" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected args", "details": args}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"meta":        map[string]any{"count": 1},
			"data": map[string]any{
				"flowSlug": "flow-release",
				"items": []any{map[string]any{
					"invocationKind": "http",
					"flowSlug":       "flow-release",
					"entrypointId":   "enrich",
					"installationId": "prof-live",
					"requestCount":   2,
					"successCount":   1,
					"errorCount":     1,
				}},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "metrics", "flow-release", "enrich",
		"--installation-id", "prof-live",
		"--kind", "http",
	)
	if err != nil {
		t.Fatalf("flows metrics failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	items, _ := out.Data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 metric item, got %#v", out.Data["items"])
	}
	item, _ := items[0].(map[string]any)
	if item["requestCount"] != float64(2) || item["successCount"] != float64(1) {
		t.Fatalf("unexpected metric item: %#v", item)
	}
}

func TestFlowsMetrics_SourceFiltersAuthorInterfaceScope(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.invocations.metrics" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" || args["entrypointId"] != "enrich" || args["interfaceScope"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected args", "details": args}})
			return
		}
		if _, ok := args["installationId"]; ok {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected installation id", "details": args}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"meta":        map[string]any{"count": 1},
			"data": map[string]any{
				"flowSlug": "flow-release",
				"items": []any{map[string]any{
					"invocationKind": "http",
					"interfaceScope": "draft",
					"flowSlug":       "flow-release",
					"entrypointId":   "enrich",
					"requestCount":   1,
					"successCount":   1,
				}},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "metrics", "flow-release", "enrich",
		"--source", "draft",
	)
	if err != nil {
		t.Fatalf("flows metrics failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	if out.Meta["source"] != "draft" {
		t.Fatalf("expected source metadata, got %#v", out.Meta)
	}
	items, _ := out.Data["items"].([]any)
	item, _ := items[0].(map[string]any)
	if item["interfaceScope"] != "draft" || item["requestCount"] != float64(1) {
		t.Fatalf("unexpected metric item: %#v", item)
	}
}

func TestFlowsMetrics_RejectsSourceWithInstallationID(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:1",
		"--token", "user-dev",
		"flows", "metrics", "flow-release", "enrich",
		"--source", "draft",
		"--installation-id", "prof-live",
	)
	if err == nil {
		t.Fatalf("expected incompatible flags error\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "--source cannot be combined with --installation-id or --target") {
		t.Fatalf("expected incompatible flags error, got stderr=%s stdout=%s", stderr, stdout)
	}
}

func TestFlowsInterfacesList_InstallationTargetResolvesPinnedVersion(t *testing.T) {
	var commands []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		commands = append(commands, command)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "flows.installations.get":
			if args["profileId"] != "prof-live" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing installation id"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"flowSlug":          "flow-release",
					"version":           9,
					"installedVersion":  9,
					"sourceWorkspaceId": "ws-provider",
					"sourceFlowSlug":    "flow-release",
				},
			})
		case "flows.get":
			if args["flowSlug"] != "flow-release" || args["source"] != "active" || args["version"] != float64(9) || args["sourceWorkspaceId"] != "ws-provider" || args["sourceFlowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected flows.get args"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"flow": map[string]any{
						"invocations": map[string]any{"default": map[string]any{}},
						"interfaces": map[string]any{
							"manual":  []any{map[string]any{"id": "run", "invocation": "default"}},
							"http":    []any{map[string]any{"id": "enrich", "invocation": "default"}},
							"webhook": []any{map[string]any{"id": "stripe", "eventName": "billing.stripe.webhook", "invocation": "default"}},
						},
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "interfaces", "list", "flow-release",
		"--installation-id", "prof-live",
	)
	if err != nil {
		t.Fatalf("flows interfaces list failed: %v\n%s", err, stdout)
	}
	if strings.Join(commands, ",") != "flows.installations.get,flows.get" {
		t.Fatalf("unexpected command sequence: %#v", commands)
	}
	out := decodeEnvelope(t, stdout)
	if out.Data["target"] != "installation" || out.Data["installationId"] != "prof-live" {
		t.Fatalf("expected installation target metadata, got %#v", out.Data)
	}
}

func TestFlowsInstallationsInterfaces_ResolvesInstallationFlowSlugAndVersion(t *testing.T) {
	var commands []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		commands = append(commands, command)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "flows.installations.get":
			if args["profileId"] != "prof-live" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing installation id"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"flowSlug":          "flow-release",
					"version":           9,
					"installedVersion":  9,
					"sourceWorkspaceId": "ws-provider",
					"sourceFlowSlug":    "flow-release",
				},
			})
		case "flows.get":
			if args["flowSlug"] != "flow-release" || args["source"] != "active" || args["version"] != float64(9) || args["includeFlowLiteral"] != false || args["sourceWorkspaceId"] != "ws-provider" || args["sourceFlowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected flows.get args"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"flow": map[string]any{
						"invocations": map[string]any{"default": map[string]any{}},
						"interfaces": map[string]any{
							"manual":  []any{map[string]any{"id": "run", "invocation": "default"}},
							"http":    []any{map[string]any{"id": "enrich", "invocation": "default"}},
							"webhook": []any{map[string]any{"id": "stripe", "eventName": "billing.stripe.webhook", "invocation": "default"}},
						},
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "interfaces", "prof-live",
	)
	if err != nil {
		t.Fatalf("flows installations interfaces failed: %v\n%s", err, stdout)
	}
	if strings.Join(commands, ",") != "flows.installations.get,flows.get" {
		t.Fatalf("unexpected command sequence: %#v", commands)
	}
	out := decodeEnvelope(t, stdout)
	if out.Data["target"] != "installation" || out.Data["installationId"] != "prof-live" || out.Data["flowSlug"] != "flow-release" {
		t.Fatalf("expected installation interface metadata, got %#v", out.Data)
	}
	items, _ := out.Data["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 interfaces, got %#v", out.Data["items"])
	}
	first, _ := items[1].(map[string]any)
	endpoint, _ := first["endpoint"].(map[string]any)
	if endpoint["method"] != "POST" || endpoint["auth"] != "workspace-api-auth" || endpoint["url"] != srv.URL+"/api/flows/flow-release/installations/prof-live/interfaces/enrich" {
		t.Fatalf("expected runtime endpoint metadata, got %#v", endpoint)
	}
	if endpoint["alternateUrl"] != srv.URL+"/api/workspaces/ws-acme/flows/flow-release/installations/prof-live/interfaces/enrich" {
		t.Fatalf("expected workspace-scoped alternate endpoint, got %#v", endpoint)
	}
	second, _ := items[2].(map[string]any)
	webhookEndpoint, _ := second["endpoint"].(map[string]any)
	if webhookEndpoint["method"] != "POST" || webhookEndpoint["auth"] != "webhook-auth" || webhookEndpoint["url"] != srv.URL+"/ws-acme/events/webhooks/flow-release/billing.stripe.webhook/prof-live" {
		t.Fatalf("expected webhook endpoint metadata, got %#v", webhookEndpoint)
	}
}

func TestFlowsInterfacesList_RejectsInstallationWithTarget(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:1",
		"--token", "user-dev",
		"flows", "interfaces", "list", "flow-release",
		"--installation-id", "prof-live",
		"--target", "live",
	)
	if err == nil {
		t.Fatalf("expected incompatible flags error\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "--installation-id cannot be combined with --target or --version") {
		t.Fatalf("expected incompatible flags error, got stderr=%s stdout=%s", stderr, stdout)
	}
}

func TestFlowsInterfacesShow_FindsMcpTool(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flow": map[string]any{
					"invocations": map[string]any{
						"default": map[string]any{"inputs": []any{map[string]any{"name": "domain"}}},
					},
					"interfaces": map[string]any{
						"mcp": []any{map[string]any{"tool-name": "enrich_company", "description": "Enrich company", "invocation": "default"}},
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
		"flows", "interfaces", "show", "flow-release", "enrich_company",
		"--family", "mcp",
	)
	if err != nil {
		t.Fatalf("flows interfaces show failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	iface, _ := out.Data["interface"].(map[string]any)
	if iface["family"] != "mcp" || iface["toolName"] != "enrich_company" || iface["invocationId"] != "default" {
		t.Fatalf("unexpected interface: %#v", iface)
	}
	if iface["runtimeStatus"] != "available" {
		t.Fatalf("expected MCP runtime to be available: %#v", iface)
	}
	endpoint, _ := iface["endpoint"].(map[string]any)
	if endpoint["method"] != "POST" || endpoint["auth"] != "workspace-api-auth" || endpoint["protocol"] != "mcp" || endpoint["transport"] != "streamable-http" {
		t.Fatalf("expected MCP endpoint metadata, got %#v", endpoint)
	}
	if endpoint["url"] != srv.URL+"/api/flows/flow-release/interfaces/draft/enrich_company" {
		t.Fatalf("unexpected MCP endpoint URL: %#v", endpoint)
	}
	if endpoint["alternateUrl"] != srv.URL+"/api/workspaces/ws-acme/flows/flow-release/interfaces/draft/enrich_company" {
		t.Fatalf("expected workspace-scoped alternate endpoint, got %#v", endpoint)
	}
	if iface["invocation"] == nil {
		t.Fatalf("expected invocation contract in interface: %#v", iface)
	}
}

func TestFlowsInterfacesCall_PostsToHTTPInterfaceRoute(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/flows/flow-release/installations/prof-live/interfaces/enrich" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		if r.Header.Get("X-Breyta-Workspace") != "ws-acme" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing workspace header"}})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		input, _ := body["input"].(map[string]any)
		if input["domain"] != "example.com" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing input"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"started":    true,
				"workflowId": "wf-interface-1",
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "interfaces", "call", "flow-release", "enrich",
		"--installation-id", "prof-live",
		"--input", `{"domain":"example.com"}`,
	)
	if err != nil {
		t.Fatalf("flows interfaces call failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	if out.Data["workflowId"] != "wf-interface-1" {
		t.Fatalf("unexpected interface call output: %#v", out.Data)
	}
}

func TestFlowsInterfacesCurl_InstallationBuildsRuntimeCommand(t *testing.T) {
	var commands []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		commands = append(commands, command)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "flows.installations.get":
			if args["profileId"] != "prof-live" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing installation id"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"flowSlug":         "flow-release",
					"installedVersion": 9,
				},
			})
		case "flows.get":
			if args["flowSlug"] != "flow-release" || args["source"] != "active" || args["version"] != float64(9) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected flows.get args", "details": args}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"flow": map[string]any{
						"invocations": map[string]any{"default": map[string]any{}},
						"interfaces": map[string]any{
							"http": []any{map[string]any{"id": "enrich", "invocation": "default", "method": "post"}},
						},
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "interfaces", "curl", "flow-release", "enrich",
		"--installation-id", "prof-live",
		"--input", `{"domain":"example.com"}`,
	)
	if err != nil {
		t.Fatalf("flows interfaces curl --installation-id failed: %v\n%s", err, stdout)
	}
	if strings.Join(commands, ",") != "flows.installations.get,flows.get" {
		t.Fatalf("unexpected command sequence: %#v", commands)
	}
	out := decodeEnvelope(t, stdout)
	if out.Data["installationId"] != "prof-live" {
		t.Fatalf("expected installation metadata, got %#v", out.Data)
	}
	curl, _ := out.Data["curl"].(string)
	if !strings.Contains(curl, srv.URL+"/api/flows/flow-release/installations/prof-live/interfaces/enrich") ||
		!strings.Contains(curl, "Authorization: Bearer ${BREYTA_TOKEN}") ||
		!strings.Contains(curl, `{"input":{"domain":"example.com"}}`) {
		t.Fatalf("unexpected curl command: %s", curl)
	}
	if strings.Contains(curl, "user-dev") {
		t.Fatalf("curl command leaked token: %s", curl)
	}
}

func TestFlowsInterfacesCall_TargetLiveCallsAuthorSourceEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/flows/flow-release/interfaces/live/enrich" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		input, _ := body["input"].(map[string]any)
		if input["domain"] != "example.com" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing input"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"workflowId": "wf-interface-live"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "interfaces", "call", "flow-release", "enrich",
		"--target", "live",
		"--input", `{"domain":"example.com"}`,
	)
	if err != nil {
		t.Fatalf("flows interfaces call --target live failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	if out.Data["workflowId"] != "wf-interface-live" {
		t.Fatalf("unexpected interface call output: %#v", out.Data)
	}
}

func TestFlowsInterfacesCall_WaitPollsRunCompletion(t *testing.T) {
	var runsGetPayloads []map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/flows/flow-release/installations/prof-live/interfaces/enrich":
			if r.Method != http.MethodPost {
				w.WriteHeader(405)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"workflowId":     "wf-interface-wait",
					"installationId": "prof-live",
					"status":         "running",
				},
			})
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "runs.get" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
				return
			}
			args, _ := body["args"].(map[string]any)
			runsGetPayloads = append(runsGetPayloads, args)
			if args["workflowId"] != "wf-interface-wait" || args["installationId"] != "prof-live" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected runs.get args"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"run": map[string]any{
						"workflowId":     "wf-interface-wait",
						"installationId": "prof-live",
						"status":         "completed",
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
		"flows", "interfaces", "call", "flow-release", "enrich",
		"--installation-id", "prof-live",
		"--wait",
		"--poll", "1ms",
		"--timeout", "2s",
	)
	if err != nil {
		t.Fatalf("flows interfaces call --wait failed: %v\n%s", err, stdout)
	}
	if len(runsGetPayloads) == 0 {
		t.Fatalf("expected at least one runs.get payload")
	}
	out := decodeEnvelope(t, stdout)
	run, _ := out.Data["run"].(map[string]any)
	if run["status"] != "completed" || run["workflowId"] != "wf-interface-wait" {
		t.Fatalf("unexpected waited run output: %#v", out.Data)
	}
}

func TestFlowsInterfacesCurl_TargetLiveBuildsCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" || args["source"] != "active" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected flows.get args"}})
			return
		}
		if _, ok := args["version"]; ok {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected version"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flow": map[string]any{
					"invocations": map[string]any{"default": map[string]any{}},
					"interfaces":  map[string]any{"http": []any{map[string]any{"id": "enrich", "invocation": "default"}}},
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
		"flows", "interfaces", "curl", "flow-release", "enrich",
		"--target", "live",
		"--input", `{"domain":"example.com"}`,
	)
	if err != nil {
		t.Fatalf("flows interfaces curl failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	curl, _ := out.Data["curl"].(string)
	if !strings.Contains(curl, srv.URL+"/api/flows/flow-release/interfaces/live/enrich") ||
		!strings.Contains(curl, "Authorization: Bearer ${BREYTA_TOKEN}") ||
		!strings.Contains(curl, `{"input":{"domain":"example.com"}}`) {
		t.Fatalf("unexpected curl command: %s", curl)
	}
	if strings.Contains(curl, "user-dev") {
		t.Fatalf("curl command leaked token: %s", curl)
	}
}

func TestFlowsInterfacesCall_DefaultsToDraftAuthorSourceEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/flows/flow-release/interfaces/draft/enrich" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"workflowId": "wf-interface-draft"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "interfaces", "call", "flow-release", "enrich",
	)
	if err != nil {
		t.Fatalf("flows interfaces call default draft failed: %v\n%s", err, stdout)
	}
	out := decodeEnvelope(t, stdout)
	if out.Data["workflowId"] != "wf-interface-draft" {
		t.Fatalf("unexpected interface call output: %#v", out.Data)
	}
}

func TestFlowsInterfacesCall_ErrorAddsRecoveryHints(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/flows/flow-release/installations/prof-live/interfaces/missing" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": false,
			"error": map[string]any{
				"message": "HTTP interface not found",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "interfaces", "call", "flow-release", "missing",
		"--installation-id", "prof-live",
	)
	if err == nil {
		t.Fatalf("expected interface not found error\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	out := decodeEnvelope(t, stdout)
	hint, _ := out.Meta["hint"].(string)
	if !strings.Contains(hint, "breyta flows installations interfaces prof-live") {
		t.Fatalf("expected installation interfaces hint, got %#v", out.Meta)
	}
	if !strings.Contains(stderr, `breyta docs find "flow interfaces invocation"`) {
		t.Fatalf("expected docs hint in stderr, got:\n%s", stderr)
	}
}

func TestFlowsRun_EmitsMappedRunStartedTelemetry(t *testing.T) {
	t.Setenv("BREYTA_POSTHOG_ENABLED", "true")
	t.Setenv("BREYTA_POSTHOG_DISABLED", "")
	t.Setenv("BREYTA_POSTHOG_API_KEY", "test-posthog-key")

	events := make(chan string, 8)
	posthog := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/capture/" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if event, _ := body["event"].(string); strings.TrimSpace(event) != "" {
			events <- event
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer posthog.Close()
	t.Setenv("BREYTA_POSTHOG_HOST", posthog.URL)

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.run" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"run": map[string]any{"workflowId": "wf-3", "status": "running"}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", jwtWithEmailForCLI("user@example.com"),
		"flows", "run", "flow-release",
	)
	if err != nil {
		t.Fatalf("flows run failed: %v\n%s", err, stdout)
	}

	foundMapped := false
	deadline := time.After(2 * time.Second)
	for !foundMapped {
		select {
		case event := <-events:
			if event == "cli_run_started" {
				foundMapped = true
			}
		case <-deadline:
			t.Fatal("expected cli_run_started telemetry event")
		}
	}
}

func TestFlowsRun_ExplicitDraftTarget_UsesCanonicalCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.run" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["target"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing target=draft"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"run": map[string]any{"workflowId": "wf-3", "status": "running"}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "flow-release",
		"--target", "draft",
	)
	if err != nil {
		t.Fatalf("flows run --target draft failed: %v\n%s", err, stdout)
	}
}

func TestFlowsRun_ForwardsTriggerID(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.run" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["target"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing target=draft"}})
			return
		}
		if args["triggerId"] != "manual-import" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing triggerId"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"run": map[string]any{"workflowId": "wf-trigger", "status": "running"}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "flow-release",
		"--target", "draft",
		"--trigger-id", "manual-import",
	)
	if err != nil {
		t.Fatalf("flows run --trigger-id failed: %v\n%s", err, stdout)
	}
}

func TestFlowsRun_ForwardsInterfaceID(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.run" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["triggerId"] != "manual-import" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing manual selector"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"run": map[string]any{"workflowId": "wf-interface", "status": "running"}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "flow-release",
		"--target", "draft",
		"--interface-id", "manual-import",
	)
	if err != nil {
		t.Fatalf("flows run --interface-id failed: %v\n%s", err, stdout)
	}
}

func TestFlowsRun_WaitPollsWhenWorkflowIDNestedUnderRun(t *testing.T) {
	var flowsRunCalls int
	var runsGetCalls int
	var finalHydrationCalls int

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "flows.run":
			flowsRunCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"run": map[string]any{
						"workflowId": "wf-nested",
						"status":     "running",
					},
				},
			})
		case "runs.get":
			runsGetCalls++
			if args["workflowId"] != "wf-nested" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected workflowId"}})
				return
			}
			if args["includeSteps"] == true {
				finalHydrationCalls++
				if args["includeResult"] != false {
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "final wait hydration should skip result"}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"run": map[string]any{
							"workflowId": "wf-nested",
							"status":     "completed",
							"counters":   map[string]any{"http-requests": 1},
							"steps":      []map[string]any{{"stepId": "fetch", "stepType": "http", "status": "completed"}},
						},
					},
				})
				return
			}
			if args["includeSteps"] != false || args["includeResult"] != false {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "wait polling should request compact runs.get"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"run": map[string]any{
						"workflowId": "wf-nested",
						"status":     "completed",
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "flow-release",
		"--wait",
		"--poll", "1ms",
		"--timeout", "2s",
	)
	if err != nil {
		t.Fatalf("flows run --wait failed: %v\n%s", err, stdout)
	}
	if flowsRunCalls != 1 {
		t.Fatalf("expected 1 flows.run call, got %d", flowsRunCalls)
	}
	if runsGetCalls < 1 {
		t.Fatalf("expected at least 1 runs.get poll, got %d", runsGetCalls)
	}
	if finalHydrationCalls != 1 {
		t.Fatalf("expected one terminal wait hydration call, got %d", finalHydrationCalls)
	}
	if !strings.Contains(stdout, "wf-nested") {
		t.Fatalf("expected output to include workflow id, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"http-requests":1`) {
		t.Fatalf("expected hydrated wait output to include fresh http counter, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"stepId":"fetch"`) {
		t.Fatalf("expected wait output to include compact step summary, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "resultPreview") {
		t.Fatalf("expected wait output to strip verbose step details, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "breyta runs inspect wf-nested") {
		t.Fatalf("expected wait output to include run inspection next command, got:\n%s", stdout)
	}
}

func TestFlowsRun_WaitForwardsInstallationIDToRunsGet(t *testing.T) {
	var runsGetPayloads []map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "flows.run":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"workflowId":     "wf-linked",
					"installationId": "prof-consumer",
					"status":         "running",
				},
			})
		case "runs.get":
			runsGetPayloads = append(runsGetPayloads, args)
			if args["workflowId"] != "wf-linked" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected workflowId"}})
				return
			}
			if args["installationId"] != "prof-consumer" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing installationId"}})
				return
			}
			if args["includeSteps"] == true {
				if args["includeResult"] != false {
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "final wait hydration should skip result"}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok": true,
					"data": map[string]any{
						"run": map[string]any{
							"workflowId":     "wf-linked",
							"installationId": "prof-consumer",
							"status":         "completed",
							"steps":          []map[string]any{},
						},
					},
				})
				return
			}
			if args["includeSteps"] != false || args["includeResult"] != false {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "wait polling should request compact runs.get"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"run": map[string]any{
						"workflowId":     "wf-linked",
						"installationId": "prof-consumer",
						"status":         "completed",
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-consumer",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "public-install-proof",
		"--installation-id", "prof-consumer",
		"--wait",
		"--poll", "1ms",
		"--timeout", "2s",
	)
	if err != nil {
		t.Fatalf("flows run --wait failed: %v\n%s", err, stdout)
	}
	if len(runsGetPayloads) == 0 {
		t.Fatalf("expected at least one runs.get payload")
	}
	for _, payload := range runsGetPayloads {
		if payload["installationId"] != "prof-consumer" {
			t.Fatalf("expected runs.get payload to include installationId, got %#v", payload)
		}
		if payload["includeSteps"] == true {
			if payload["includeResult"] != false {
				t.Fatalf("expected final wait hydration to skip result, got %#v", payload)
			}
			continue
		}
		if payload["includeSteps"] != false || payload["includeResult"] != false {
			t.Fatalf("expected compact runs.get payload, got %#v", payload)
		}
	}
}

func TestFlowsRun_WaitTimeoutIncludesHydratedSnapshotAndLongerTimeoutHint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "flows.run":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"workflowId": "wf-slow",
					"status":     "running",
				},
			})
		case "runs.get":
			run := map[string]any{
				"workflowId": "wf-slow",
				"status":     "running",
			}
			if args["includeSteps"] == true {
				run["steps"] = []any{map[string]any{"stepId": "slow-step", "stepType": "llm", "status": "running", "resultPreview": "omit me"}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"data": map[string]any{"run": run},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "flow-slow",
		"--wait",
		"--poll", "1ms",
		"--timeout", "1ms",
	)
	if err == nil {
		t.Fatalf("expected flows run --wait to exit nonzero when the wait times out\nstdout=%s", stdout)
	}
	if !strings.Contains(stdout, `"timedOut":true`) {
		t.Fatalf("expected timeout metadata, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"stepId":"slow-step"`) {
		t.Fatalf("expected hydrated compact step snapshot, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "resultPreview") {
		t.Fatalf("expected timeout snapshot to strip verbose step details, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "breyta flows run flow-slow --wait --timeout 2m") {
		t.Fatalf("expected longer-timeout next command, got:\n%s", stdout)
	}
}

func TestFlowsRun_WaitReturnsNonZeroForFailedRun(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "flows.run":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"workflowId": "wf-failed",
					"status":     "running",
				},
			})
		case "runs.get":
			if args["includeSteps"] == true {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok": true,
					"data": map[string]any{
						"run": map[string]any{
							"workflowId": "wf-failed",
							"status":     "failed",
							"steps": []any{
								map[string]any{"stepId": "fetch", "status": "failed", "error": map[string]any{"message": "HTTP 403"}},
							},
						},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"run": map[string]any{
						"workflowId": "wf-failed",
						"status":     "failed",
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "flow-release",
		"--wait",
		"--poll", "1ms",
		"--timeout", "2s",
	)
	if err == nil {
		t.Fatalf("expected failed waited run to exit nonzero\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, `"status":"failed"`) {
		t.Fatalf("expected failed run output, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "flow run finished with status failed") {
		t.Fatalf("expected failure guidance on stderr, got:\n%s", stderr)
	}
}

func TestFlowsRun_MissingRunInputsShowsInputHintOnly(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.run" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": false,
			"error": map[string]any{
				"message": "Missing required run inputs",
				"details": map[string]any{"missingKeys": []string{"public_document_url"}},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "packet-builder",
	)
	if err == nil {
		t.Fatalf("expected missing input command to fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, `"missingRunInputs":[`) || !strings.Contains(stdout, `public_document_url`) {
		t.Fatalf("expected missing run input metadata, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "connections list") || strings.Contains(stdout, "flows promote") {
		t.Fatalf("expected run-input guidance without setup/promote hints, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, `--input '{\"public_document_url\":\"\\u003cvalue\\u003e\"}'`) {
		t.Fatalf("expected exact --input next command, got:\n%s", stdout)
	}
}

func TestRunsShow_ForwardsInstallationIDInAPIMode(t *testing.T) {
	var capturedArgs map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		if command != "runs.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		capturedArgs = args
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"run": map[string]any{
					"workflowId":     "wf-linked",
					"installationId": "prof-consumer",
					"status":         "completed",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-consumer",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "show", "wf-linked",
		"--installation-id", "prof-consumer",
	)
	if err != nil {
		t.Fatalf("runs show failed: %v\n%s", err, stdout)
	}
	if capturedArgs == nil {
		t.Fatalf("expected runs.get request")
	}
	if capturedArgs["workflowId"] != "wf-linked" {
		t.Fatalf("expected workflowId wf-linked, got %#v", capturedArgs["workflowId"])
	}
	if capturedArgs["installationId"] != "prof-consumer" {
		t.Fatalf("expected installationId prof-consumer, got %#v", capturedArgs["installationId"])
	}
	if capturedArgs["includeSteps"] != false || capturedArgs["includeResult"] != false {
		t.Fatalf("expected default compact runs.get flags, got %#v", capturedArgs)
	}
}

func TestRunsShow_FullRequestsStepsAndResultInAPIMode(t *testing.T) {
	var capturedArgs map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedArgs, _ = body["args"].(map[string]any)
		if body["command"] != "runs.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"run": map[string]any{
					"workflowId": "wf-full",
					"status":     "completed",
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
		"runs", "show", "wf-full",
		"--full",
	)
	if err != nil {
		t.Fatalf("runs show --full failed: %v\n%s", err, stdout)
	}
	if capturedArgs["includeSteps"] != true || capturedArgs["includeResult"] != true {
		t.Fatalf("expected --full to request steps and result, got %#v", capturedArgs)
	}
}

func TestRunsInspect_CompactsServerRunPayloadInAPIMode(t *testing.T) {
	var capturedArgs map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		capturedArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"run": map[string]any{
					"workflowId": "wf-inspect",
					"flowSlug":   "flow-inspect",
					"status":     "completed",
					"input":      map[string]any{"prompt": strings.Repeat("x", 2048)},
					"result":     map[string]any{"body": strings.Repeat("y", 2048)},
					"steps": []map[string]any{
						{
							"stepId":        "fetch",
							"stepType":      "http",
							"status":        "completed",
							"input":         map[string]any{"url": "https://example.com", "headers": map[string]any{"authorization": "secret"}},
							"result":        map[string]any{"body": strings.Repeat("z", 2048)},
							"resultPreview": map[string]any{"status": 200},
							"usage":         map[string]any{"httpRequests": 1},
						},
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
		"runs", "inspect", "wf-inspect",
	)
	if err != nil {
		t.Fatalf("runs inspect failed: %v\n%s", err, stdout)
	}
	if capturedArgs["includeSteps"] != true || capturedArgs["includeResult"] != false || capturedArgs["compactInspect"] != true {
		t.Fatalf("expected compact runs.get inspect flags, got %#v", capturedArgs)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := envelope["data"].(map[string]any)
	run, _ := data["run"].(map[string]any)
	if _, ok := run["input"]; ok {
		t.Fatalf("expected run input to be removed from compact inspect: %#v", run["input"])
	}
	if _, ok := run["result"]; ok {
		t.Fatalf("expected run result to be removed from compact inspect: %#v", run["result"])
	}
	steps, _ := run["steps"].([]any)
	if len(steps) != 1 {
		t.Fatalf("expected one compact step, got %#v", steps)
	}
	step, _ := steps[0].(map[string]any)
	if _, ok := step["input"]; ok {
		t.Fatalf("expected step input to be removed from compact inspect: %#v", step["input"])
	}
	if _, ok := step["result"]; ok {
		t.Fatalf("expected step result to be removed from compact inspect: %#v", step["result"])
	}
	if step["hasInput"] != true || step["hasOutput"] != true {
		t.Fatalf("expected compact hasInput/hasOutput flags, got %#v", step)
	}
	cost, _ := step["cost"].(map[string]any)
	if cost["httpRequests"] != float64(1) {
		t.Fatalf("expected compact cost summary, got %#v", step["cost"])
	}
	meta, _ := envelope["meta"].(map[string]any)
	if meta["outputView"] != "compact" || meta["compactInspect"] != true {
		t.Fatalf("expected compact inspect metadata, got %#v", meta)
	}
}

func TestRunsStep_InspectsOneStepInAPIMode(t *testing.T) {
	var capturedArgs map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		capturedArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"run": map[string]any{
					"workflowId": "wf-step",
					"flowSlug":   "flow-step",
					"steps": []map[string]any{
						{"stepId": "fetch", "stepType": "http", "status": "completed", "inputPreview": map[string]any{"url": "https://example.com"}, "resultPreview": map[string]any{"status": 200}},
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
		"runs", "step", "wf-step", "fetch",
	)
	if err != nil {
		t.Fatalf("runs step failed: %v\n%s", err, stdout)
	}
	if capturedArgs["includeSteps"] != true || capturedArgs["includeResult"] != false {
		t.Fatalf("expected compact runs.get step flags, got %#v", capturedArgs)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["stepId"] != "fetch" || data["type"] != "http" {
		t.Fatalf("unexpected step data: %#v", data)
	}
	if _, ok := data["input"]; !ok {
		t.Fatalf("expected compact input in step data: %#v", data)
	}
	if _, ok := data["output"]; !ok {
		t.Fatalf("expected compact output in step data: %#v", data)
	}
}

func TestRunsStep_FullRequestsCapturedStepOutputInAPIMode(t *testing.T) {
	var capturedArgs map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "runs.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		capturedArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"run": map[string]any{
					"workflowId": "wf-step",
					"flowSlug":   "flow-step",
					"steps": []map[string]any{
						{
							"stepId":         "review",
							"stepType":       "agent",
							"status":         "completed",
							"resultPreview":  map[string]any{"summary": "compact"},
							"output":         map[string]any{"nested": map[string]any{"ok": true}},
							"outputResource": "res://v1/ws/ws-acme/result/run/wf-step/step/review/output",
						},
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
		"runs", "step", "wf-step", "review", "--full",
	)
	if err != nil {
		t.Fatalf("runs step --full failed: %v\n%s", err, stdout)
	}
	if capturedArgs["includeStepResults"] != true || capturedArgs["stepId"] != "review" {
		t.Fatalf("expected full step payload request, got %#v", capturedArgs)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := envelope["data"].(map[string]any)
	output, _ := data["output"].(map[string]any)
	nested, _ := output["nested"].(map[string]any)
	if nested["ok"] != true {
		t.Fatalf("expected full nested output, got %#v", data["output"])
	}
	if data["outputResource"] == "" {
		t.Fatalf("expected outputResource, got %#v", data)
	}
}

func TestRunsEvents_ListsTimelineInAPIMode(t *testing.T) {
	var capturedCommand string
	var capturedArgs map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedCommand, _ = body["command"].(string)
		capturedArgs, _ = body["args"].(map[string]any)
		if capturedCommand != "runs.events" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"meta": map[string]any{
				"count": 2,
			},
			"data": map[string]any{
				"workflowId": "wf-events",
				"items": []map[string]any{
					{"type": "run_started", "workflowId": "wf-events"},
					{"type": "step_started", "workflowId": "wf-events", "stepId": "review"},
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
		"runs", "events", "wf-events", "--step", "review", "--limit", "25",
	)
	if err != nil {
		t.Fatalf("runs events failed: %v\n%s", err, stdout)
	}
	if capturedCommand != "runs.events" || capturedArgs["workflowId"] != "wf-events" || capturedArgs["stepId"] != "review" || capturedArgs["limit"] != float64(25) {
		t.Fatalf("expected runs.events payload, command=%q args=%#v", capturedCommand, capturedArgs)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := envelope["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected timeline items, got %#v", data)
	}
}

func TestRunsContinue_ApprovesLatestWaitInAPIMode(t *testing.T) {
	var sawList bool
	var approvedWait string

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/waits":
			sawList = true
			if got := r.URL.Query().Get("workflowId"); got != "wf-wait" {
				t.Fatalf("expected workflowId wf-wait, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"items": []map[string]any{
						{"waitId": "wait-old", "workflowId": "wf-wait", "registeredAt": "2026-05-13T08:00:00Z", "approval": map[string]any{"actions": []string{"approve", "reject"}}},
						{"waitId": "wait-new", "workflowId": "wf-wait", "registeredAt": "2026-05-13T09:00:00Z", "approval": map[string]any{"actions": []string{"approve", "reject"}}},
						{"waitId": "wait-other", "workflowId": "wf-other", "registeredAt": "2026-05-13T10:00:00Z", "approval": map[string]any{"actions": []string{"approve", "reject"}}},
					},
				},
			})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/waits/") && strings.HasSuffix(r.URL.Path, "/approve"):
			approvedWait = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/waits/"), "/approve")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"approved": true}})
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
		"runs", "continue", "wf-wait",
		"--approve-latest-wait",
	)
	if err != nil {
		t.Fatalf("runs continue failed: %v\n%s", err, stdout)
	}
	if !sawList {
		t.Fatalf("expected waits list request")
	}
	if approvedWait != "wait-new" {
		t.Fatalf("expected latest wait-new approval, got %q", approvedWait)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["continued"] != true || data["action"] != "approve" {
		t.Fatalf("unexpected continue output: %#v", data)
	}
}

func TestRunsContinue_ExplainsIneligibleActiveWaits(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/waits" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"items": []map[string]any{
					{"waitId": "wait-manual", "workflowId": "wf-wait", "stepId": "approval", "status": "running", "actions": []string{"continue"}},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "continue", "wf-wait",
		"--approve-latest-wait",
	)
	if err == nil {
		t.Fatalf("expected ineligible wait to fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, `"code":"no_approval_wait"`) {
		t.Fatalf("expected no_approval_wait code, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"activeWaitCount":1`) || !strings.Contains(stdout, "missing approval metadata or approve action") {
		t.Fatalf("expected active wait eligibility details, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "wait.approval or wait.actions must contain approve") {
		t.Fatalf("expected required wait shape guidance, got:\n%s", stdout)
	}
}

func TestRunsLogsAPIModePointsToSupportedInspection(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "logs",
			args: []string{"runs", "logs", "wf-123"},
			want: []string{"API run logs are not available yet", "breyta runs inspect wf-123", "breyta runs step wf-123 STEP_ID --full"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := []string{
				"--dev",
				"--workspace", "ws-acme",
				"--api", "http://127.0.0.1:9",
				"--token", "user-dev",
			}
			stdout, stderr, err := runCLIArgs(t, append(base, tc.args...)...)
			if err == nil {
				t.Fatalf("expected API-mode unsupported command to fail\nstdout=%s\nstderr=%s", stdout, stderr)
			}
			for _, want := range tc.want {
				if !strings.Contains(stdout, want) {
					t.Fatalf("expected output to contain %q, got:\n%s", want, stdout)
				}
			}
		})
	}
}

func TestFlowsRun_RejectsPreviewTarget(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "run", "flow-preview",
		"--target", "preview",
	)
	if err == nil {
		t.Fatalf("expected flows run to fail for unsupported preview target")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "invalid --target (expected draft or live)") {
		t.Fatalf("expected invalid target guidance, got:\n%s", combined)
	}
}

func TestFlowsRun_RejectsEndUserTarget(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "run", "flow-end-user",
		"--target", "end-user",
	)
	if err == nil {
		t.Fatalf("expected flows run to fail for unsupported end-user target")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "invalid --target (expected draft or live)") {
		t.Fatalf("expected invalid target guidance, got:\n%s", combined)
	}
}

func TestFlowsInstallPromote_LiveScope_UsesCanonicalCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.promote" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["scope"] != "live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing scope=live"}})
			return
		}
		if args["policy"] != "pinned" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing policy=pinned"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"target": "live", "scope": "live", "policy": "pinned"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "promote", "flow-release",
		"--scope", "live",
		"--policy", "pinned",
	)
	if err != nil {
		t.Fatalf("flows promote --scope live failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallPromote_InvalidScope(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "promote", "flow-release",
		"--scope", "workspace",
	)
	if err == nil {
		t.Fatalf("expected flows promote to fail for invalid scope")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "invalid --scope (expected all or live)") {
		t.Fatalf("expected invalid scope guidance, got:\n%s", combined)
	}
}

func TestFlowsValidate_DefaultsToDraftSource(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.validate" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-validate" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["source"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=draft"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-validate", "source": "draft", "valid": true},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "validate", "flow-validate",
	)
	if err != nil {
		t.Fatalf("flows validate failed: %v\n%s", err, stdout)
	}
}

func TestFlowsValidate_TargetLive_UsesResolvedVersion(t *testing.T) {
	step := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/flow-profiles":
			step++
			if got := r.URL.Query().Get("flow-slug"); got != "flow-validate" {
				t.Fatalf("expected flow-slug=flow-validate, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"profile-id": "prof-live", "version": 9, "enabled": false, "updated-at": "2026-02-16T20:00:00Z", "config": map[string]any{"install-scope": "live"}},
				},
			})
		case "/api/commands":
			step++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "flows.validate" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
				return
			}
			args, _ := body["args"].(map[string]any)
			if args["source"] != "active" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=active"}})
				return
			}
			if args["version"] != float64(9) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing version=9"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-validate", "source": "active", "version": 9, "valid": true},
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
		"flows", "validate", "flow-validate",
		"--target", "live",
	)
	if err != nil {
		t.Fatalf("flows validate --target live failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected profile resolve + validate command, got %d", step)
	}
}

func TestFlowsCompile_UsesActiveSourceInAPIMode(t *testing.T) {
	calls := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.compile" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-compile" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["source"] != "active" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=active"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-compile", "source": "active", "compiled": true},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "compile", "flow-compile",
	)
	if err != nil {
		t.Fatalf("flows compile failed: %v\n%s", err, stdout)
	}
	if calls != 1 {
		t.Fatalf("expected one flows.compile command call, got %d", calls)
	}
}

func TestFlowsShow_DefaultsToDraftSource(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-show" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["source"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=draft"}})
			return
		}
		if args["view"] != "summary" || args["includeFlowLiteral"] != false || args["includeTemplates"] != false || args["includeFunctions"] != false {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected compact flows.get defaults"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flow": map[string]any{"slug": "flow-show", "version": 7}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "show", "flow-show",
	)
	if err != nil {
		t.Fatalf("flows show failed: %v\n%s", err, stdout)
	}
}

func TestFlowsShow_FullRequestsVerboseFields(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		if body["command"] != "flows.get" || args["view"] != "full" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected flows.get view=full"}})
			return
		}
		if args["includeFlowLiteral"] != true || args["includeTemplates"] != true || args["includeFunctions"] != true {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected full include flags"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flow": map[string]any{"slug": "flow-show", "version": 7}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "show", "flow-show",
		"--full",
	)
	if err != nil {
		t.Fatalf("flows show --full failed: %v\n%s", err, stdout)
	}
}

func TestFlowsShow_TargetLive_UsesResolvedVersion(t *testing.T) {
	step := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/flow-profiles":
			step++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"profile-id": "prof-live", "version": 11, "enabled": false, "updated-at": "2026-02-16T20:05:00Z", "config": map[string]any{"install-scope": "live"}},
				},
			})
		case "/api/commands":
			step++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "flows.get" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
				return
			}
			args, _ := body["args"].(map[string]any)
			if args["source"] != "active" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=active"}})
				return
			}
			if args["version"] != float64(11) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing version=11"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flow": map[string]any{"slug": "flow-show", "version": 11}},
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
		"flows", "show", "flow-show",
		"--target", "live",
	)
	if err != nil {
		t.Fatalf("flows show --target live failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected profile resolve + show command, got %d", step)
	}
}

func TestFlowsShow_TargetDraft_UsesDraftSource(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/flow-profiles":
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "should not resolve live profile for --target draft"}})
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "flows.get" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
				return
			}
			args, _ := body["args"].(map[string]any)
			if args["source"] != "draft" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=draft"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flow": map[string]any{"slug": "flow-show", "version": 7}},
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
		"flows", "show", "flow-show",
		"--target", "draft",
	)
	if err != nil {
		t.Fatalf("flows show --target draft failed: %v\n%s", err, stdout)
	}
}

func TestFlowsShow_TargetLive_ResolvesAcrossProfilePagination(t *testing.T) {
	t.Parallel()

	profileCalls := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/flow-profiles":
			profileCalls++
			cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
			switch cursor {
			case "":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"profile-id": "prof-end-user", "version": 3, "enabled": true, "updated-at": "2026-02-16T20:00:00Z", "user-id": "u-1"},
					},
					"meta": map[string]any{
						"hasMore":    true,
						"nextCursor": "page-2",
					},
				})
			case "page-2":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"profile-id": "prof-live", "version": 17, "enabled": true, "updated-at": "2026-02-16T20:05:00Z", "config": map[string]any{"install-scope": "live"}},
					},
					"meta": map[string]any{
						"hasMore": false,
					},
				})
			default:
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected cursor"}})
			}
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "flows.get" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
				return
			}
			args, _ := body["args"].(map[string]any)
			if args["source"] != "active" || args["version"] != float64(17) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "expected source=active version=17"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flow": map[string]any{"slug": "flow-show", "version": 17}},
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
		"flows", "show", "flow-show",
		"--target", "live",
	)
	if err != nil {
		t.Fatalf("flows show --target live failed: %v\n%s", err, stdout)
	}
	if profileCalls != 2 {
		t.Fatalf("expected 2 paged profile calls, got %d", profileCalls)
	}
}

func TestFlowsPull_TargetLive_UsesResolvedVersion(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "flow-show.clj")
	step := 0

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/flow-profiles":
			step++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"profile-id": "prof-live", "version": 13, "enabled": false, "updated-at": "2026-02-16T20:10:00Z", "config": map[string]any{"install-scope": "live"}},
				},
			})
		case "/api/commands":
			step++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "flows.get" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
				return
			}
			args, _ := body["args"].(map[string]any)
			if args["source"] != "active" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=active"}})
				return
			}
			if args["version"] != float64(13) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing version=13"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowLiteral": "{:slug :flow-show :flow '(identity 1)}"},
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
		"flows", "pull", "flow-show",
		"--target", "live",
		"--out", outPath,
	)
	if err != nil {
		t.Fatalf("flows pull --target live failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected profile resolve + pull command, got %d", step)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read pulled flow: %v", err)
	}
	if !strings.Contains(string(raw), ":flow-show") {
		t.Fatalf("pulled flow file did not contain expected content: %s", string(raw))
	}
}

func TestFlowsPull_DefaultsToDraftSource(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "flow-draft.clj")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["source"] != "draft" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=draft"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowLiteral": "{:slug :flow-draft :flow '(identity 2)}"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "pull", "flow-draft",
		"--out", outPath,
	)
	if err != nil {
		t.Fatalf("flows pull failed: %v\n%s", err, stdout)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read pulled flow: %v", err)
	}
	if !strings.Contains(string(raw), ":flow-draft") {
		t.Fatalf("pulled flow file did not contain expected content: %s", string(raw))
	}
}

func TestFlowsPull_HonorsCommandContext(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "flow-timeout.clj")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowLiteral": "{:slug :flow-timeout :flow '(identity 3)}"},
		})
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, stderr, err := runCLIArgsWithContext(t, ctx,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "pull", "flow-timeout",
		"--out", outPath,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v\n%s", err, stderr)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("expected pull to stop promptly on context cancellation, took %s", elapsed)
	}
}

func TestFlowsPromote_WithVersion_UsesInstallPromoteCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.promote" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-release" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		if args["target"] != "live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing target=live"}})
			return
		}
		if args["version"] != float64(7) {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing version=7"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-release", "target": "live", "version": 7},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "promote", "flow-release",
		"--version", "7",
	)
	if err != nil {
		t.Fatalf("flows promote --version failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallSetEnabledFalse_UsesDisableCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.installations.set_enabled" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["profileId"] != "prof-disable" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
			return
		}
		if args["enabled"] != false {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing enabled=false"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"instance": map[string]any{"profileId": "prof-disable", "enabled": false}},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "installations", "set-enabled", "prof-disable",
		"--enabled=false",
	)
	if err != nil {
		t.Fatalf("flows installations set-enabled failed: %v\n%s", err, stdout)
	}
}

func TestFlowsDraftReset_UsesFlowsDraftResetCommand(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.draft.reset" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		args, _ := body["args"].(map[string]any)
		if args["flowSlug"] != "flow-reset" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"draftDeleted": true}})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "draft", "reset", "flow-reset",
	)
	if err != nil {
		t.Fatalf("flows draft reset failed: %v\n%s", err, stdout)
	}
}

func TestFlowsRelease_DeployKeyForwardedToReleaseAndPromote(t *testing.T) {
	step := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		switch body["command"] {
		case "flows.release":
			step++
			if args["flowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			if args["deployKey"] != "guarded-secret" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing deployKey"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "activeVersion": 3},
			})
		case "flows.promote":
			step++
			if args["flowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			if args["target"] != "live" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing promote target"}})
				return
			}
			if args["version"] != float64(3) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing promote version"}})
				return
			}
			if args["deployKey"] != "guarded-secret" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing deployKey"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "flow-release", "profileId": "prof-live", "target": "live"},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "release", "flow-release",
		"--deploy-key", "guarded-secret",
	)
	if err != nil {
		t.Fatalf("flows release with --deploy-key failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected release + promote commands, got %d", step)
	}
}

func TestFlowsRelease_RequiresAuthPreflightWhenInstallEnabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))

	called := false
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.NotFound(w, r)
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"flows", "release", "flow-release",
	)
	if err == nil {
		t.Fatalf("expected flows release to fail without auth token")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "missing token") && !strings.Contains(combined, "missing --token or BREYTA_TOKEN") {
		t.Fatalf("expected missing-token guidance, got:\n%s", combined)
	}
	if called {
		t.Fatalf("expected auth preflight to fail before API call")
	}
}
