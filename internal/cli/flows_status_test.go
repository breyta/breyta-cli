package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestFlowsStatusSendsCommand(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "my-flow",
				"source":   "draft",
				"ready":    true,
				"summary": map[string]any{
					"entrypointCount":   1,
					"stepCount":         1,
					"verifiedStepCount": 1,
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
		"flows", "status", "my-flow",
		"--source", "draft",
		"--check",
	)
	if err != nil {
		t.Fatalf("flows status failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.status" {
		t.Fatalf("expected flows.status, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["check"] != true {
		t.Fatalf("expected flowSlug/source/check args, got %#v", args)
	}
}

func TestFlowsStatusSendsVersionSource(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "my-flow",
				"source":   "version",
				"version":  7,
				"ready":    true,
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "status", "my-flow",
		"--source", "version",
		"--version", "7",
	)
	if err != nil {
		t.Fatalf("flows status --source version failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	args, _ := got["args"].(map[string]any)
	if args["source"] != "version" || args["version"] != "7" {
		t.Fatalf("expected source version args, got %#v", args)
	}
}
