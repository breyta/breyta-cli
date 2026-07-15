package cli_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestContract_WorkspacesListMarksCurrent(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	stdout, _, err := runCLI(t, statePath, "workspaces", "list", "--pretty")
	if err != nil {
		t.Fatalf("expected success, got error: %v\n%s", err, stdout)
	}
	e := decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("expected ok=true, got ok=false: %+v", e)
	}
	itemsAny, ok := e.Data["items"]
	if !ok {
		t.Fatalf("missing data.items")
	}
	items, ok := itemsAny.([]any)
	if !ok {
		t.Fatalf("data.items is not an array: %T", itemsAny)
	}
	if len(items) == 0 {
		t.Fatalf("expected at least 1 workspace")
	}

	found := false
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id != "demo-workspace" {
			continue
		}
		found = true
		cur, _ := m["current"].(bool)
		if !cur {
			t.Fatalf("expected demo-workspace current=true, got: %+v", m)
		}
	}
	if !found {
		t.Fatalf("expected to find demo-workspace in list")
	}
}

func TestContract_WorkspacesCurrent(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	stdout, _, err := runCLI(t, statePath, "workspaces", "current", "--pretty")
	if err != nil {
		t.Fatalf("expected success, got error: %v\n%s", err, stdout)
	}
	e := decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("expected ok=true, got ok=false: %+v", e)
	}
	workspaceAny, ok := e.Data["workspace"]
	if !ok {
		t.Fatalf("missing data.workspace")
	}
	workspace, ok := workspaceAny.(map[string]any)
	if !ok {
		t.Fatalf("data.workspace is not an object: %T", workspaceAny)
	}
	id, _ := workspace["id"].(string)
	if id != "demo-workspace" {
		t.Fatalf("unexpected workspace id: %q", id)
	}
	name, _ := workspace["name"].(string)
	if name == "" {
		t.Fatalf("expected workspace name to be present, got: %+v", workspace)
	}
	cur, _ := workspace["current"].(bool)
	if !cur {
		t.Fatalf("expected current=true, got: %+v", workspace)
	}
}

func TestWorkspacesAPIReadbackPreservesBillingReadiness(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{"id": "user-1"},
			"workspaces": []map[string]any{
				{
					"id":   "ws-acme",
					"name": "Acme",
					"role": "admin",
					"billing": map[string]any{
						"status":           "none",
						"paidSubscription": false,
						"freeTrial":        false,
						"runReady":         false,
						"billingUrl":       "https://flows.breyta.ai/ws-acme/billing",
					},
					"runEntitlement": map[string]any{
						"ready":      false,
						"decision":   "block",
						"severity":   "error",
						"code":       "billing_subscription_required",
						"nextAction": "Subscribe or upgrade to continue.",
						"billingUrl": "https://flows.breyta.ai/ws-acme/billing",
					},
				},
			},
		})
	}))
	defer srv.Close()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "current", args: []string{"workspaces", "current", "--pretty"}},
		{name: "show", args: []string{"workspaces", "show", "ws-acme", "--pretty"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := []string{"--dev", "--api", srv.URL, "--token", "user-dev", "--workspace", "ws-acme"}
			stdout, _, err := runCLIArgs(t, append(base, tc.args...)...)
			if err != nil {
				t.Fatalf("expected success, got error: %v\n%s", err, stdout)
			}
			out := decodeEnvelope(t, stdout)
			if !out.OK {
				t.Fatalf("expected ok=true, got ok=false: %+v", out)
			}
			workspace, ok := out.Data["workspace"].(map[string]any)
			if !ok {
				t.Fatalf("expected data.workspace object, got %#v", out.Data["workspace"])
			}
			billing, ok := workspace["billing"].(map[string]any)
			if !ok {
				t.Fatalf("expected workspace.billing object, got %#v", workspace["billing"])
			}
			if billing["status"] != "none" || billing["runReady"] != false || billing["billingUrl"] == "" {
				t.Fatalf("unexpected billing readback: %#v", billing)
			}
			runEntitlement, ok := workspace["runEntitlement"].(map[string]any)
			if !ok {
				t.Fatalf("expected workspace.runEntitlement object, got %#v", workspace["runEntitlement"])
			}
			if runEntitlement["ready"] != false || runEntitlement["decision"] != "block" || runEntitlement["code"] != "billing_subscription_required" {
				t.Fatalf("unexpected run entitlement readback: %#v", runEntitlement)
			}
		})
	}
}
