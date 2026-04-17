package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceAccountsCreate_UsesAPICommand(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "service_accounts.create" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
			return
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"serviceAccount": map[string]any{
					"serviceAccountId": "sa-1",
					"name":             "Jobs worker",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"service-accounts", "create",
		"--name", "Jobs worker",
		"--capability", "jobs.worker,flows.read",
		"--capability", "resources.read",
		"--job-type", "demo.agent-review",
		"--metadata", `{"owner":"it"}`,
	)
	if err != nil {
		t.Fatalf("service-accounts create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["name"].(string); got != "Jobs worker" {
		t.Fatalf("expected name=Jobs worker, got %#v", gotArgs["name"])
	}
	capabilities, _ := gotArgs["capabilities"].([]any)
	if len(capabilities) != 3 || capabilities[0] != "jobs.worker" || capabilities[1] != "flows.read" || capabilities[2] != "resources.read" {
		t.Fatalf("expected capabilities=[jobs.worker flows.read resources.read], got %#v", gotArgs["capabilities"])
	}
	jobTypes, _ := gotArgs["allowedJobTypes"].([]any)
	if len(jobTypes) != 1 || jobTypes[0] != "demo.agent-review" {
		t.Fatalf("expected allowedJobTypes=[demo.agent-review], got %#v", gotArgs["allowedJobTypes"])
	}
	metadata, _ := gotArgs["metadata"].(map[string]any)
	if got, _ := metadata["owner"].(string); got != "it" {
		t.Fatalf("expected metadata.owner=it, got %#v", metadata["owner"])
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
}

func TestServiceAccountsUpdate_UsesCommaSeparatedCapabilities(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "service_accounts.update" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
			return
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"serviceAccount": map[string]any{
					"serviceAccountId": "sa-1",
					"name":             "Agent",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"service-accounts", "update", "sa-1",
		"--capability", "flows.read,flows.manage",
		"--capability", "resources.write",
	)
	if err != nil {
		t.Fatalf("service-accounts update failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["serviceAccountId"].(string); got != "sa-1" {
		t.Fatalf("expected serviceAccountId=sa-1, got %#v", gotArgs["serviceAccountId"])
	}
	capabilities, _ := gotArgs["capabilities"].([]any)
	if len(capabilities) != 3 || capabilities[0] != "flows.read" || capabilities[1] != "flows.manage" || capabilities[2] != "resources.write" {
		t.Fatalf("expected capabilities=[flows.read flows.manage resources.write], got %#v", gotArgs["capabilities"])
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
}

func TestServiceAccountsKeysCreate_UsesAPICommand(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "service_accounts.keys.create" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
			return
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"serviceAccountId": "sa-1",
				"key": map[string]any{
					"keyId":  "sak-1",
					"apiKey": "bsa_sak-1_secret",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"service-accounts", "keys", "create", "sa-1",
		"--name", "runner key",
		"--expires-at", "2030-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("service-accounts keys create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["serviceAccountId"].(string); got != "sa-1" {
		t.Fatalf("expected serviceAccountId=sa-1, got %#v", gotArgs["serviceAccountId"])
	}
	if got, _ := gotArgs["name"].(string); got != "runner key" {
		t.Fatalf("expected name=runner key, got %#v", gotArgs["name"])
	}
	if got, _ := gotArgs["expiresAt"].(string); got != "2030-01-01T00:00:00Z" {
		t.Fatalf("expected expiresAt=2030-01-01T00:00:00Z, got %#v", gotArgs["expiresAt"])
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
}
