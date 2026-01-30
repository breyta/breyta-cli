package tui

import (
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/buildinfo"
)

func buildInfoInline() string {
	parts := []string{buildinfo.DisplayVersion()}
	if c := shortCommit(buildinfo.Commit); c != "" {
		parts = append(parts, c)
	}
	if d := shortDate(buildinfo.Date); d != "" {
		parts = append(parts, d)
	}
	return strings.Join(parts, " · ")
}

func shortCommit(commit string) string {
	c := strings.TrimSpace(commit)
	if c == "" || c == "none" {
		return ""
	}
	if len(c) <= 7 {
		return c
	}
	return c[:7]
}

func shortDate(date string) string {
	d := strings.TrimSpace(date)
	if d == "" || d == "unknown" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, d); err == nil {
		return t.Format("2006-01-02")
	}
	if len(d) >= 10 {
		return d[:10]
	}
	return d
}
