package tui

import "testing"

func TestWorkspaceNameOrID_FallsBackToID(t *testing.T) {
	m := homeModel{workspaces: nil}
	if got := m.workspaceNameOrID("ws-1"); got != "ws-1" {
		t.Fatalf("workspaceNameOrID: got %q want %q", got, "ws-1")
	}
}
