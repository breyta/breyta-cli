package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// These colors are aligned with breyta-frontend-mono/packages/breyta-design/src/styles/colors.ts
// and adapted to remain readable on both light and dark terminal backgrounds.
var (
	breytaTextColor = lipgloss.AdaptiveColor{Light: "#311f35", Dark: "#f8f4fb"} // colors.text / colors["text-10"]
	breytaMuted     = lipgloss.AdaptiveColor{Light: "#88708d", Dark: "#eae3f0"} // colors["text-40"] / colors["text-20"]
	// Borders must remain visible on light terminals; keep light-theme borders darker.
	breytaBorder = lipgloss.AdaptiveColor{Light: "#88708d", Dark: "#eae3f0"} // colors["text-40"] / colors["text-20"]
	breytaAccent = lipgloss.AdaptiveColor{Light: "#6556c3", Dark: "#8870bd"} // colors.link / colors.subtle
	breytaDanger = lipgloss.AdaptiveColor{Light: "#a32138", Dark: "#b82727"} // colors.danger / colors.rooster
)

func faintIfDark(s lipgloss.Style) lipgloss.Style {
	if lipgloss.HasDarkBackground() {
		return s.Faint(true)
	}
	return s
}

func breytaListStyles() list.Styles {
	s := list.DefaultStyles()

	s.TitleBar = lipgloss.NewStyle().Padding(0, 0, 1, 0)
	s.Title = lipgloss.NewStyle().Bold(true).Foreground(breytaTextColor).UnsetBackground()

	s.Spinner = lipgloss.NewStyle().Foreground(breytaMuted)

	s.FilterPrompt = lipgloss.NewStyle().Foreground(breytaAccent)
	s.FilterCursor = lipgloss.NewStyle().Foreground(breytaAccent)
	s.DefaultFilterCharacterMatch = lipgloss.NewStyle().Underline(true)

	s.StatusBar = lipgloss.NewStyle().Foreground(breytaMuted).Padding(0, 0, 1, 0)
	s.StatusEmpty = lipgloss.NewStyle().Foreground(breytaMuted)
	s.StatusBarActiveFilter = lipgloss.NewStyle().Foreground(breytaTextColor)
	s.StatusBarFilterCount = lipgloss.NewStyle().Foreground(breytaMuted)

	s.NoItems = lipgloss.NewStyle().Foreground(breytaMuted)
	s.PaginationStyle = lipgloss.NewStyle().PaddingLeft(0)
	s.HelpStyle = lipgloss.NewStyle().Padding(1, 0, 0, 0).Foreground(breytaMuted)

	s.ActivePaginationDot = lipgloss.NewStyle().Foreground(breytaAccent).SetString("•")
	s.InactivePaginationDot = lipgloss.NewStyle().Foreground(breytaMuted).SetString("•")
	s.DividerDot = lipgloss.NewStyle().Foreground(breytaMuted).SetString(" • ")

	return s
}

func breytaDefaultItemStyles() list.DefaultItemStyles {
	s := list.NewDefaultItemStyles()

	s.NormalTitle = lipgloss.NewStyle().
		Foreground(breytaTextColor).
		Padding(0, 0, 0, 2)

	s.NormalDesc = lipgloss.NewStyle().
		Foreground(breytaMuted).
		Padding(0, 0, 0, 2)

	s.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(breytaAccent).
		Foreground(breytaTextColor).
		Bold(true).
		Padding(0, 0, 0, 1)

	s.SelectedDesc = s.SelectedTitle.Copy().
		Bold(false).
		Foreground(breytaTextColor)

	s.DimmedTitle = lipgloss.NewStyle().
		Foreground(breytaMuted).
		Padding(0, 0, 0, 2)

	s.DimmedDesc = lipgloss.NewStyle().
		Foreground(breytaBorder).
		Padding(0, 0, 0, 2)

	s.FilterMatch = lipgloss.NewStyle().Underline(true)

	return s
}
