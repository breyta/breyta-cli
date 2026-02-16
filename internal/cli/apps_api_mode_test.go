package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunsList_SendsProfileIDFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		"--profile-id", "prof-1",
	)
	if err != nil {
		t.Fatalf("runs list failed: %v\n%s", err, stdout)
	}
}

func TestRunsStart_SendsProfileID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		"--profile-id", "prof-2",
	)
	if err != nil {
		t.Fatalf("runs start failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallations_Create_UsesFlowsInstallationsCreateCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.install.create" {
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

func TestFlowsInstallations_Get_UsesFlowsInstallationsGetCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.install.show" {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.install.configure" {
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

func TestFlowsConfigure_UsesCanonicalCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsInstallations_List_All_SendsAllFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.install.list" {
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

func TestFlowsInstallations_Delete_UsesFlowsInstallationsDeleteCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.install.delete" {
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

func TestFlowsRelease_UsesCanonicalCommand(t *testing.T) {
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case "flows.install.promote":
			step++
			if args["flowSlug"] != "flow-release" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
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
				"data":        map[string]any{"flowSlug": "flow-release", "profileId": "prof-live", "scope": "live"},
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

func TestFlowsRelease_NoInstallSkipsPromote(t *testing.T) {
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.release" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		step++
		args, _ := body["args"].(map[string]any)
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
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "release", "flow-release",
		"--no-install",
	)
	if err != nil {
		t.Fatalf("flows release --no-install failed: %v\n%s", err, stdout)
	}
	if step != 1 {
		t.Fatalf("expected release only with --no-install, got %d commands", step)
	}
}

func TestFlowsRun_UsesCanonicalCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if _, ok := args["scope"]; ok {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "scope should be omitted when not explicitly set"}})
			return
		}
		if args["profileId"] != "prof-123" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing profileId"}})
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

func TestFlowsRun_RejectsPreviewScope(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "run", "flow-preview",
		"--scope", "preview",
	)
	if err == nil {
		t.Fatalf("expected flows run to fail for unsupported preview scope")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "invalid --scope (expected live)") {
		t.Fatalf("expected invalid scope guidance, got:\n%s", combined)
	}
}

func TestFlowsRun_RejectsEndUserScope(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "run", "flow-end-user",
		"--scope", "end-user",
	)
	if err == nil {
		t.Fatalf("expected flows run to fail for unsupported end-user scope")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "invalid --scope (expected live)") {
		t.Fatalf("expected invalid scope guidance, got:\n%s", combined)
	}
}

func TestFlowsInstallPromoteLiveTrackLatest_UsesCanonicalCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.install.promote" {
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
		if args["scope"] != "live" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing scope=live"}})
			return
		}
		if args["policy"] != "track-latest" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing policy=track-latest"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"scope": "live", "policy": "track-latest"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "install", "promote", "flow-release",
		"--scope", "live",
		"--policy", "track-latest",
	)
	if err != nil {
		t.Fatalf("flows install promote live track-latest failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallPromote_InvalidPolicy(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "install", "promote", "flow-release",
		"--policy", "nope",
	)
	if err == nil {
		t.Fatalf("expected flows install promote to fail for invalid policy")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "invalid --policy (expected pinned or track-latest)") {
		t.Fatalf("expected invalid policy guidance, got:\n%s", combined)
	}
}

func TestFlowsValidate_DefaultsToCurrentSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if args["source"] != "current" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=current"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "flow-validate", "source": "current", "valid": true},
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

func TestFlowsShow_DefaultsToCurrentSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if args["source"] != "current" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing source=current"}})
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

func TestFlowsInstallSetEnabledFalse_UsesDisableCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.install.disable" {
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
		"flows", "install", "set-enabled", "prof-disable",
		"--enabled=false",
	)
	if err != nil {
		t.Fatalf("flows install set-enabled failed: %v\n%s", err, stdout)
	}
}

func TestFlowsDraftReset_UsesFlowsDraftResetCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
