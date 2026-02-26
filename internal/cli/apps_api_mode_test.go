package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestFlowsInstallations_Get_UsesFlowsInstallationsGetCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsConfigure_LiveTarget_UsesProdBindingsApply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "profiles.bindings.apply" {
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

func TestFlowsConfigureShow_UsesDraftProfileStatusByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsConfigureSuggest_DefaultTarget_UsesTemplateStatusAndConnections(t *testing.T) {
	commandCalls := 0
	connectionCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestConnectionsDelete_InUse_ReturnsHintDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsInstallations_Delete_UsesFlowsInstallationsDeleteCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsRelease_EmitsLiveVerificationHint(t *testing.T) {
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

func TestFlowsRelease_PromoteScopeLive_UsesCanonicalCommand(t *testing.T) {
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
		"--promote-scope", "live",
	)
	if err != nil {
		t.Fatalf("flows release --promote-scope live failed: %v\n%s", err, stdout)
	}
	if step != 2 {
		t.Fatalf("expected release + promote commands, got %d", step)
	}
}

func TestFlowsRelease_NoInstallRejectsPromoteScope(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "release", "flow-release",
		"--no-install",
		"--promote-scope", "live",
	)
	if err == nil {
		t.Fatalf("expected flows release --no-install --promote-scope to fail")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--promote-scope cannot be used with --no-install") {
		t.Fatalf("expected promote-scope/no-install validation message, got:\n%s", combined)
	}
}

func TestFlowsRelease_InvalidPromoteScopeFailsBeforeRelease(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "release", "flow-release",
		"--promote-scope", "workspace",
	)
	if err == nil {
		t.Fatalf("expected flows release --promote-scope workspace to fail")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "invalid --scope (expected all or live)") {
		t.Fatalf("expected invalid scope guidance, got:\n%s", combined)
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
		if _, ok := args["target"]; ok {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "target should be omitted when not explicitly set"}})
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

func TestFlowsRun_ExplicitDraftTarget_UsesCanonicalCommand(t *testing.T) {
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

func TestFlowsRun_WaitPollsWhenWorkflowIDNestedUnderRun(t *testing.T) {
	var flowsRunCalls int
	var runsGetCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if !strings.Contains(stdout, "wf-nested") {
		t.Fatalf("expected output to include workflow id, got:\n%s", stdout)
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

func TestFlowsInstallPromoteLiveTrackLatest_UsesCanonicalCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if args["policy"] != "track-latest" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing policy=track-latest"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"target": "live", "policy": "track-latest"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "promote", "flow-release",
		"--policy", "track-latest",
	)
	if err != nil {
		t.Fatalf("flows promote live track-latest failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallPromote_LiveScope_UsesCanonicalCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"target": "live", "scope": "live"},
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
	)
	if err != nil {
		t.Fatalf("flows promote --scope live failed: %v\n%s", err, stdout)
	}
}

func TestFlowsInstallPromote_InvalidPolicy(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", "http://127.0.0.1:9",
		"--token", "user-dev",
		"flows", "promote", "flow-release",
		"--policy", "nope",
	)
	if err == nil {
		t.Fatalf("expected flows promote to fail for invalid policy")
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "invalid --policy (expected pinned or track-latest)") {
		t.Fatalf("expected invalid policy guidance, got:\n%s", combined)
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

func TestFlowsValidate_TargetLive_UsesResolvedVersion(t *testing.T) {
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsShow_TargetLive_UsesResolvedVersion(t *testing.T) {
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsShow_TargetDraft_UsesCurrentSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsPromote_WithVersion_UsesInstallPromoteCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsRelease_DeployKeyForwardedToReleaseAndPromote(t *testing.T) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
