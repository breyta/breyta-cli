package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBreytaListStyles_AreTextFirst(t *testing.T) {
	s := breytaListStyles()

	var wantNoColor lipgloss.TerminalColor = lipgloss.NoColor{}
	if got := s.Title.GetBackground(); got != wantNoColor {
		t.Fatalf("expected list title background to be unset (%T), got %T", wantNoColor, got)
	}

	var wantAccent lipgloss.TerminalColor = breytaAccent
	if got := s.FilterPrompt.GetForeground(); got != wantAccent {
		t.Fatalf("expected filter prompt foreground %v, got %v", wantAccent, got)
	}
}

func TestBreytaDefaultItemStyles_SelectedHasAccentBorder(t *testing.T) {
	s := breytaDefaultItemStyles()

	if !s.SelectedTitle.GetBorderLeft() {
		t.Fatalf("expected SelectedTitle to have left border enabled")
	}
	if !s.SelectedDesc.GetBorderLeft() {
		t.Fatalf("expected SelectedDesc to have left border enabled")
	}

	var wantAccent lipgloss.TerminalColor = breytaAccent
	if got := s.SelectedTitle.GetBorderLeftForeground(); got != wantAccent {
		t.Fatalf("expected SelectedTitle left border foreground %v, got %v", wantAccent, got)
	}
	if got := s.SelectedDesc.GetBorderLeftForeground(); got != wantAccent {
		t.Fatalf("expected SelectedDesc left border foreground %v, got %v", wantAccent, got)
	}

	if !s.SelectedTitle.GetBold() {
		t.Fatalf("expected SelectedTitle to be bold")
	}
}
