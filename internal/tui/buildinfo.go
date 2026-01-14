package tui

import (
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/buildinfo"
)

func buildInfoInline() string {
	parts := []string{versionDisplay()}
	if c := shortCommit(buildinfo.Commit); c != "" {
		parts = append(parts, c)
	}
	if d := shortDate(buildinfo.Date); d != "" {
		parts = append(parts, d)
	}
	return strings.Join(parts, " · ")
}

func versionDisplay() string {
	v := strings.TrimSpace(buildinfo.Version)
	if v == "" {
		return "dev"
	}
	if v == "dev" {
		return v
	}
	if len(v) > 0 && v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
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

