package cli_test

import (
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
