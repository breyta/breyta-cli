package cli

import (
	"errors"
	"net/url"
	"strings"

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

func escapePathSegments(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(part))
	}
	return strings.Join(escaped, "/")
}
