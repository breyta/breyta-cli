package tui

import (
	"path/filepath"
	"testing"

	"github.com/breyta/breyta-cli/internal/configstore"
)

func TestWorkspaceNameOrID_FallsBackToID(t *testing.T) {
	m := homeModel{workspaces: nil}
	if got := m.workspaceNameOrID("ws-1"); got != "ws-1" {
		t.Fatalf("workspaceNameOrID: got %q want %q", got, "ws-1")
	}
}

func TestLoadConfig_UsesDevProfileWhenDevModeEnabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := configstore.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	st := &configstore.Store{
		DevMode:   true,
		DevActive: "local",
		DevProfiles: map[string]configstore.DevProfile{
			"local": {
				APIURL:      configstore.DefaultLocalAPIURL,
				WorkspaceID: "ws-dev",
			},
		},
	}
	if err := configstore.SaveAtomic(path, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	m := homeModel{cfg: HomeConfig{}}
	apiURL, ws := m.loadConfig()
	if apiURL != configstore.DefaultLocalAPIURL {
		t.Fatalf("expected dev API url %q, got %q (config at %s)", configstore.DefaultLocalAPIURL, apiURL, filepath.Dir(path))
	}
	if ws != "ws-dev" {
		t.Fatalf("expected dev workspace %q, got %q", "ws-dev", ws)
	}
}
