package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWorkspaceMembersListBuildsCanonicalMembersRequestAndDefaultsToTable(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		if r.URL.Path != "/api/workspaces/ws-breyta/members" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workspaceId":    "ws-breyta",
			"includePending": false,
			"total":          2,
			"items": []map[string]any{
				{"userId": "user-1", "role": "creator", "name": "Ada Admin", "email": "ada@example.com"},
				{"userId": "user-2", "role": "billing", "name": "Moe Member", "email": "moe@example.com"},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-breyta",
		"--token", "tok-1",
		"workspaces", "members", "list",
	)
	if err != nil {
		t.Fatalf("command failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if gotPath != "/api/workspaces/ws-breyta/members" {
		t.Fatalf("expected members path, got %q", gotPath)
	}
	if gotQuery.Get("role") != "" {
		t.Fatalf("expected no role filter, got %q", gotQuery.Get("role"))
	}
	if gotQuery.Get("include-pending") != "" {
		t.Fatalf("expected no include-pending query by default, got %q", gotQuery.Get("include-pending"))
	}
	if !strings.Contains(stdout, "userId") || !strings.Contains(stdout, "role") {
		t.Fatalf("expected table header, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "user-1") || !strings.Contains(stdout, "ada@example.com") {
		t.Fatalf("expected admin member row, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "user-2") || !strings.Contains(stdout, "moe@example.com") {
		t.Fatalf("expected member row, got:\n%s", stdout)
	}
}

func TestWorkspaceMembersListSupportsRoleAndPendingFiltersAndJSONOutput(t *testing.T) {
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		if r.URL.Path != "/api/workspaces/ws-breyta/members" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workspaceId":    "ws-breyta",
			"includePending": true,
			"roleFilter":     "admin",
			"total":          2,
			"items": []map[string]any{
				{"userId": "user-1", "role": "creator", "name": "Ada Admin", "email": "ada@example.com", "status": "active"},
				{"userId": "user-3", "role": "admin", "name": "Pat Pending", "email": "pat@example.com", "status": "pending"},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-breyta",
		"--token", "tok-1",
		"workspaces", "members", "list",
		"--role", "admin",
		"--include-pending",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("command failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if gotQuery.Get("role") != "admin" {
		t.Fatalf("expected role=admin, got %q", gotQuery.Get("role"))
	}
	if gotQuery.Get("include-pending") != "true" {
		t.Fatalf("expected include-pending=true, got %q", gotQuery.Get("include-pending"))
	}

	out := decodeEnvelope(t, stdout)
	if !out.OK {
		t.Fatalf("expected ok=true, got output: %s", stdout)
	}
	if out.Data == nil {
		t.Fatalf("expected data object in output: %s", stdout)
	}
	if workspaceID, _ := out.Data["workspaceId"].(string); workspaceID != "ws-breyta" {
		t.Fatalf("expected data.workspaceId=ws-breyta, got %#v", out.Data["workspaceId"])
	}
	items, _ := out.Data["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected two member items, got %#v", out.Data["items"])
	}
	first, _ := items[0].(map[string]any)
	if role, _ := first["role"].(string); role != "creator" {
		t.Fatalf("expected first item role=creator, got %#v", first["role"])
	}
	if status, _ := first["status"].(string); status != "active" {
		t.Fatalf("expected first item status=active, got %#v", first["status"])
	}
}
