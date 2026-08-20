package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/breyta/breyta-cli/internal/configstore"
)

func TestAPIShowReportsEffectiveEnvironmentOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	path, err := configstore.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if err := configstore.SaveAtomic(path, &configstore.Store{APIURL: "https://stored.example"}); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}
	t.Setenv("BREYTA_API_URL", "http://127.0.0.1:38190/")

	stdout, stderr, err := runCLIArgs(t, "api", "show")
	if err != nil {
		t.Fatalf("api show failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	var out struct {
		Data struct {
			APIURL string `json:"apiUrl"`
		} `json:"data"`
		Meta struct {
			Source       string `json:"source"`
			StoredAPIURL string `json:"storedApiUrl"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	if out.Data.APIURL != "http://127.0.0.1:38190" || out.Meta.Source != "env" {
		t.Fatalf("expected effective env endpoint, got %#v", out)
	}
	if out.Meta.StoredAPIURL != "https://stored.example" {
		t.Fatalf("expected differing stored endpoint in metadata, got %#v", out.Meta)
	}
}

func TestAPIUse_WorksWithDevFlag(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	stdout, stderr, err := runCLIArgs(t,
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
