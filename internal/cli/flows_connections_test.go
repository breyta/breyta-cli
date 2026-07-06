package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestFlowsConnectionsStatusSendsCommand(t *testing.T) {
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
				"summary": map[string]any{
					"ready":     1,
					"missing":   1,
					"unhealthy": 0,
				},
				"requirements": []any{
					map[string]any{"slot": "api", "status": "ready", "connectionId": "conn-api"},
					map[string]any{"slot": "crm", "status": "missing"},
				},
				"availableConnections": []any{
					map[string]any{"connectionId": "conn-api", "type": "http-api"},
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
		"flows", "connections", "status", "my-flow",
		"--source", "draft",
		"--step", "tools/search",
	)
	if err != nil {
		t.Fatalf("flows connections status failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.connections.status" {
		t.Fatalf("expected flows.connections.status, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["step"] != "tools/search" {
		t.Fatalf("expected flowSlug/source args, got %#v", args)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	summary, _ := data["summary"].(map[string]any)
	if summary["ready"] != float64(1) || summary["missing"] != float64(1) {
		t.Fatalf("expected readiness summary to pass through, got %#v", summary)
	}
}

func TestFlowsConnectionsStatusSendsVersionSource(t *testing.T) {
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
				"summary":  map[string]any{"ready": 0},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "connections", "status", "my-flow",
		"--source", "version",
		"--version", "7",
	)
	if err != nil {
		t.Fatalf("flows connections status --source version failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	args, _ := got["args"].(map[string]any)
	if args["source"] != "version" || args["version"] != "7" {
		t.Fatalf("expected source version args, got %#v", args)
	}
}

func TestFlowsConnectionsAuthoringSubcommandsSendCommands(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	cases := []struct {
		name       string
		apiCommand string
	}{
		{name: "suggest", apiCommand: "flows.connections.suggest"},
		{name: "setup", apiCommand: "flows.connections.setup"},
		{name: "test", apiCommand: "flows.connections.test"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
						"source":   "active",
						"summary":  map[string]any{"ready": 1},
					},
				})
			}))
			defer srv.Close()

			stdout, stderr, err := runCLIArgs(t,
				"--dev",
				"--workspace", "ws-acme",
				"--api", srv.URL,
				"--token", "user-dev",
				"flows", "connections", tc.name, "my-flow",
				"--source", "active",
			)
			if err != nil {
				t.Fatalf("flows connections %s failed: %v\nstdout=%s\nstderr=%s", tc.name, err, stdout, stderr)
			}
			if got["command"] != tc.apiCommand {
				t.Fatalf("expected %s, got %#v", tc.apiCommand, got["command"])
			}
			args, _ := got["args"].(map[string]any)
			if args["flowSlug"] != "my-flow" || args["source"] != "active" {
				t.Fatalf("expected flowSlug/source args, got %#v", args)
			}
		})
	}
}
