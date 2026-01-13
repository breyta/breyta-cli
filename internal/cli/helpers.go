package cli

import (
	"errors"

	"github.com/breyta/breyta-cli/internal/state"
)

func getWorkspace(st *state.State, workspaceID string) (*state.Workspace, error) {
	if st == nil {
		return nil, errors.New("state is nil")
	}
	ws := st.Workspaces[workspaceID]
	if ws == nil {
		return nil, errors.New("workspace not found")
	}
	return ws, nil
}
