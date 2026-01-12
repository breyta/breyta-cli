package tui

import (
	"path/filepath"
	"testing"

	"breyta-cli/internal/configstore"

	"github.com/charmbracelet/bubbles/list"
)

func TestApplyModal_WorkspaceDefault_PersistsAPIURLAndWorkspace(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg"))

	apiURL := "https://example.invalid"

	m := newHomeModel(HomeConfig{APIURL: apiURL})

	l := newModalList("Pick default workspace", true)
	_ = l.SetItems([]list.Item{
		modalItem{id: "ws-1", name: "ws-1", desc: "ws-1"},
	})
	l.Select(0)

	m.modal = &modalModel{
		kind: modalWorkspaceDefault,
		list: l,
	}

	m.applyModal()

	p, err := configstore.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	st, err := configstore.Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st.APIURL != apiURL {
		t.Fatalf("APIURL: got %q want %q", st.APIURL, apiURL)
	}
	if st.WorkspaceID != "ws-1" {
		t.Fatalf("WorkspaceID: got %q want %q", st.WorkspaceID, "ws-1")
	}
}

func TestWorkspaceNameOrID_FallsBackToID(t *testing.T) {
	m := homeModel{workspaces: nil}
	if got := m.workspaceNameOrID("ws-1"); got != "ws-1" {
		t.Fatalf("workspaceNameOrID: got %q want %q", got, "ws-1")
	}
}
