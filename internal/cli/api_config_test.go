package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/breyta/breyta-cli/internal/configstore"
)

func TestAPIUse_WorksWithDevFlag(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"api", "use", "local",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("api use failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("expected JSON output, got:\n%s\nerr=%v", stdout, err)
	}

	path, err := configstore.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	st, err := configstore.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st == nil {
		t.Fatalf("expected config store to exist")
	}
	if st.APIURL != configstore.DefaultLocalAPIURL {
		t.Fatalf("expected stored api url %q, got %q", configstore.DefaultLocalAPIURL, st.APIURL)
	}
}

