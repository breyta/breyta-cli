package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCmpOrDash(t *testing.T) {
	if got := cmpOrDash(""); got != "-" {
		t.Fatalf("expected '-', got %q", got)
	}
	if got := cmpOrDash("  "); got != "-" {
		t.Fatalf("expected '-', got %q", got)
	}
	if got := cmpOrDash(" x "); got != "x" {
		t.Fatalf("expected 'x', got %q", got)
	}
}

func TestMinInt(t *testing.T) {
	if got := minInt(1, 2); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	if got := minInt(2, 1); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestTranslateNavKeys(t *testing.T) {
	if got := translateNavKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}); got.Type != tea.KeyRunes {
		t.Fatalf("expected passthrough, got %#v", got)
	}
	if got := translateNavKeys(tea.KeyMsg{Type: tea.KeyCtrlN}); got.Type != tea.KeyDown {
		t.Fatalf("expected ctrl+n -> down, got %#v", got)
	}
	if got := translateNavKeys(tea.KeyMsg{Type: tea.KeyCtrlP}); got.Type != tea.KeyUp {
		t.Fatalf("expected ctrl+p -> up, got %#v", got)
	}
}
