package configstore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveAtomicAndLoad_RoundTripAndTrim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	st := &Store{
		APIURL:         "  http://localhost:8090  ",
		WorkspaceID:    "  ws-acme  ",
		RunConfigID:    "  run-cfg-123  ",
		DevMode:        true,
		DevAPIURL:      "  http://dev.local  ",
		DevWorkspaceID: "  ws-dev  ",
		DevToken:       "  dev-token  ",
		DevRunConfigID: "  dev-run-cfg  ",
		DevActive:      "  staging  ",
		DevProfiles: map[string]DevProfile{
			"staging": {
				APIURL:        "  https://staging.example  ",
				WorkspaceID:   "  ws-staging  ",
				Token:         "  staging-token  ",
				RunConfigID:   "  staging-runcfg  ",
				AuthStorePath: "  /tmp/auth.staging.json  ",
			},
		},
	}
	if err := SaveAtomic(path, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("expected 0600 perms, got %o", fi.Mode().Perm())
		}
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.APIURL != "http://localhost:8090" {
		t.Fatalf("expected trimmed api url, got %q", loaded.APIURL)
	}
	if loaded.WorkspaceID != "ws-acme" {
		t.Fatalf("expected trimmed workspace id, got %q", loaded.WorkspaceID)
	}
	if loaded.RunConfigID != "run-cfg-123" {
		t.Fatalf("expected trimmed run config id, got %q", loaded.RunConfigID)
	}
	if !loaded.DevMode {
		t.Fatalf("expected dev mode true")
	}
	if loaded.DevAPIURL != "http://dev.local" {
		t.Fatalf("expected trimmed dev api url, got %q", loaded.DevAPIURL)
	}
	if loaded.DevWorkspaceID != "ws-dev" {
		t.Fatalf("expected trimmed dev workspace id, got %q", loaded.DevWorkspaceID)
	}
	if loaded.DevToken != "dev-token" {
		t.Fatalf("expected trimmed dev token, got %q", loaded.DevToken)
	}
	if loaded.DevRunConfigID != "dev-run-cfg" {
		t.Fatalf("expected trimmed dev run config id, got %q", loaded.DevRunConfigID)
	}
	if loaded.DevActive != "staging" {
		t.Fatalf("expected trimmed dev active, got %q", loaded.DevActive)
	}
	if prof, ok := loaded.DevProfiles["staging"]; !ok {
		t.Fatalf("expected staging profile to exist")
	} else {
		if prof.APIURL != "https://staging.example" {
			t.Fatalf("expected trimmed staging api url, got %q", prof.APIURL)
		}
		if prof.WorkspaceID != "ws-staging" {
			t.Fatalf("expected trimmed staging workspace id, got %q", prof.WorkspaceID)
		}
		if prof.Token != "staging-token" {
			t.Fatalf("expected trimmed staging token, got %q", prof.Token)
		}
		if prof.RunConfigID != "staging-runcfg" {
			t.Fatalf("expected trimmed staging run config id, got %q", prof.RunConfigID)
		}
		if prof.AuthStorePath != "/tmp/auth.staging.json" {
			t.Fatalf("expected trimmed staging auth store, got %q", prof.AuthStorePath)
		}
	}
}

func TestSaveAtomic_Validations(t *testing.T) {
	if err := SaveAtomic("", &Store{}); err == nil {
		t.Fatalf("expected error for missing path")
	}
	if err := SaveAtomic("x.json", nil); err == nil {
		t.Fatalf("expected error for missing store")
	}
}

func TestLoad_Validations(t *testing.T) {
	if _, err := Load(""); err == nil {
		t.Fatalf("expected error for missing path")
	}
}
