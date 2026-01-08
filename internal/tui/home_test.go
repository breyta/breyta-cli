package tui

import (
	"testing"

	"breyta-cli/internal/configstore"
)

func TestParseMeWorkspaces(t *testing.T) {
	out := map[string]any{
		"workspaces": []any{
			map[string]any{"id": "ws-a", "name": "A"},
			map[string]any{"id": "ws-b", "name": "B"},
		},
	}
	ws, err := parseMeWorkspaces(out)
	if err != nil {
		t.Fatalf("parseMeWorkspaces error: %v", err)
	}
	if len(ws) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(ws))
	}
	if ws[0].ID != "ws-a" || ws[0].Name != "A" {
		t.Fatalf("unexpected first workspace: %+v", ws[0])
	}
	if ws[1].ID != "ws-b" || ws[1].Name != "B" {
		t.Fatalf("unexpected second workspace: %+v", ws[1])
	}
}

func TestParseMeWorkspaces_Unexpected(t *testing.T) {
	_, err := parseMeWorkspaces("not a map")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCycleAPIURL(t *testing.T) {
	if got := cycleAPIURL(""); got != configstore.DefaultLocalAPIURL {
		t.Fatalf("expected local, got %q", got)
	}
	if got := cycleAPIURL(configstore.DefaultLocalAPIURL); got != configstore.DefaultProdAPIURL {
		t.Fatalf("expected prod, got %q", got)
	}
	if got := cycleAPIURL(configstore.DefaultProdAPIURL); got != configstore.DefaultLocalAPIURL {
		t.Fatalf("expected local, got %q", got)
	}
	// Unknown/custom should snap to local.
	if got := cycleAPIURL("https://example.com"); got != configstore.DefaultLocalAPIURL {
		t.Fatalf("expected local, got %q", got)
	}
}

