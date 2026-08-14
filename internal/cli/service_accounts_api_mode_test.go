package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestServiceAccountsListFiltersNestedEngineResponse(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/service-accounts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{
			map[string]any{"serviceAccountId": "sa-active", "status": "active"},
			map[string]any{"serviceAccountId": "sa-disabled", "status": "disabled"},
		}}})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"service-accounts", "list", "--status", "active")
	if err != nil {
		t.Fatalf("service-accounts list failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	env := decodeEnvelope(t, stdout)
	nested, _ := env.Data["data"].(map[string]any)
	items, _ := nested["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["serviceAccountId"] != "sa-active" {
		t.Fatalf("expected only active nested item, got %#v", env.Data)
	}
	if _, leaked := env.Data["items"]; leaked {
		t.Fatalf("filter must preserve the nested response shape, got %#v", env.Data)
	}
}

func TestServiceAccountsCreate_UsesEngineREST(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/service-accounts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotArgs)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"serviceAccountId": "sa-1",
			"name":             "Automation",
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"service-accounts", "create",
		"--name", "Automation",
		"--scope", "flows.run,flows.read",
		"--scope", "resources.read",
		"--metadata", `{"owner":"it"}`,
	)
	if err != nil {
		t.Fatalf("service-accounts create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["name"].(string); got != "Automation" {
		t.Fatalf("expected name=Automation, got %#v", gotArgs["name"])
	}
	capabilities, _ := gotArgs["capabilities"].([]any)
	if len(capabilities) != 3 || capabilities[0] != "flows.run" || capabilities[1] != "flows.read" || capabilities[2] != "resources.read" {
		t.Fatalf("expected capabilities=[flows.run flows.read resources.read], got %#v", gotArgs["capabilities"])
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

func TestServiceAccountsUpdate_UsesEngineRESTAndCommaSeparatedScopes(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/service-accounts/sa-1" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotArgs)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"serviceAccountId": "sa-1",
			"name":             "Agent",
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"service-accounts", "update", "sa-1",
		"--scope", "flows.read,flows.manage",
		"--scope", "resources.write",
	)
	if err != nil {
		t.Fatalf("service-accounts update failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if _, present := gotArgs["serviceAccountId"]; present {
		t.Fatalf("service account id belongs in the REST path, got body %#v", gotArgs)
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

func TestServiceAccountsKeysCreate_UsesEngineREST(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/service-accounts/sa-1/keys" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotArgs)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"serviceAccountId": "sa-1",
			"key": map[string]any{
				"keyId":  "sak-1",
				"apiKey": "bsa_sak-1_secret",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
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

	if _, present := gotArgs["serviceAccountId"]; present {
		t.Fatalf("service account id belongs in the REST path, got body %#v", gotArgs)
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
