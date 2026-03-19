package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRunsList_PositionalFlowFiltersOutsideAPIMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	statePath := filepath.Join(tmp, "state.json")
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--state", statePath,
		"--api", "",
		"--token", "user-dev",
		"runs", "list", "subscription-renewal",
	)
	if err != nil {
		t.Fatalf("runs list failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["flowSlug"].(string); got != "subscription-renewal" {
		t.Fatalf("expected filtered flowSlug subscription-renewal, got %q", got)
	}
	items, _ := data["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 filtered runs, got %d", len(items))
	}
	for _, item := range items {
		row, _ := item.(map[string]any)
		if got, _ := row["flowSlug"].(string); got != "subscription-renewal" {
			t.Fatalf("expected only subscription-renewal runs, got row flowSlug=%q", got)
		}
	}
}
