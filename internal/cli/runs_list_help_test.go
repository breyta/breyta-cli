package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunsListHelpDoesNotAdvertiseLocalOnlyIncludeSteps(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t, "runs", "list", "--help")
	if err != nil {
		t.Fatalf("runs list --help failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if strings.Contains(stdout, "\n      --include-steps") {
		t.Fatalf("runs list help should not advertise the unsupported list flag:\n%s", stdout)
	}
	if !strings.Contains(stdout, "runs show <workflow-id> --include-steps") {
		t.Fatalf("runs list help should point to the supported detail command:\n%s", stdout)
	}
}

func TestRunsListHiddenIncludeStepsStillWorksInLocalStateMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--state", filepath.Join(tmp, "state.json"),
		"--api", "",
		"--token", "user-dev",
		"runs", "list", "subscription-renewal", "--include-steps",
	)
	if err != nil {
		t.Fatalf("runs list --include-steps failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected seeded local runs")
	}
	first, _ := items[0].(map[string]any)
	if _, ok := first["steps"]; !ok {
		t.Fatalf("expected hidden compatibility flag to include steps, got %#v", first)
	}
}
