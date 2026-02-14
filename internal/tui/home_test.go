package tui

import (
	"path/filepath"
	"testing"

	"github.com/breyta/breyta-cli/internal/configstore"
	"github.com/charmbracelet/bubbles/list"
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

func TestHeaderKeyHints_IncludesAuth(t *testing.T) {
	m := homeModel{}
	hints := m.headerKeyHints()
	found := false
	for _, h := range hints {
		if h == "<a> Auth" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected headerKeyHints to include %q, got %v", "<a> Auth", hints)
	}
}

func TestRefreshOptions_WhenLoggedOut_ShowsLoginHintWithAngleBrackets(t *testing.T) {
	options := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	m := homeModel{options: options, token: ""}
	m.refreshOptions()

	items := m.options.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	wi, ok := items[0].(workspaceItem)
	if !ok {
		t.Fatalf("expected workspaceItem, got %T", items[0])
	}
	if wi.desc != "Press <a> to login" {
		t.Fatalf("expected logged-out hint %q, got %q", "Press <a> to login", wi.desc)
	}
}

func TestNewWorkspaceDefaultModal_SelectsCurrentDefault(t *testing.T) {
	m := homeModel{
		workspaces: []meWorkspace{
			{ID: "ws-1", Name: "Alpha"},
			{ID: "ws-2", Name: "Beta"},
		},
		defaultWS: "ws-2",
	}

	md := m.newWorkspaceDefaultModal()
	if md == nil {
		t.Fatalf("expected modal, got nil")
	}
	if md.kind != modalWorkspaceDefault {
		t.Fatalf("expected modal kind %v, got %v", modalWorkspaceDefault, md.kind)
	}
	it, ok := md.list.SelectedItem().(modalItem)
	if !ok {
		t.Fatalf("expected selected item type modalItem, got %T", md.list.SelectedItem())
	}
	if it.id != "ws-2" {
		t.Fatalf("expected selected default workspace %q, got %q", "ws-2", it.id)
	}
}

func TestApplyModal_WorkspaceDefault_PersistsConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	options := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	m := homeModel{
		options: options,
		apiURL:  configstore.DefaultProdAPIURL,
		token:   "token",
		workspaces: []meWorkspace{
			{ID: "ws-1", Name: "Alpha"},
			{ID: "ws-2", Name: "Beta"},
		},
		defaultWS: "ws-1",
	}

	md := m.newWorkspaceDefaultModal()
	if md == nil {
		t.Fatalf("expected modal, got nil")
	}
	// Ensure ws-2 is selected.
	for i, it := range md.list.Items() {
		mi, ok := it.(modalItem)
		if ok && mi.id == "ws-2" {
			md.list.Select(i)
			break
		}
	}
	m.modal = md

	cmd := m.applyModal()
	if cmd != nil {
		t.Fatalf("expected nil cmd for workspace default modal, got %T", cmd)
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
	if st.WorkspaceID != "ws-2" {
		t.Fatalf("expected stored default workspace %q, got %q", "ws-2", st.WorkspaceID)
	}
	if st.APIURL != configstore.DefaultProdAPIURL {
		t.Fatalf("expected stored api url %q, got %q", configstore.DefaultProdAPIURL, st.APIURL)
	}
	if m.defaultWS != "ws-2" {
		t.Fatalf("expected model defaultWS %q, got %q", "ws-2", m.defaultWS)
	}
}
