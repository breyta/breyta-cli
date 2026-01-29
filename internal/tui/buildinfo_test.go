package tui

import (
	"testing"

	"github.com/breyta/breyta-cli/internal/buildinfo"
)

func TestBuildInfoInline(t *testing.T) {
	origVersion := buildinfo.Version
	origCommit := buildinfo.Commit
	origDate := buildinfo.Date
	defer func() {
		buildinfo.Version = origVersion
		buildinfo.Commit = origCommit
		buildinfo.Date = origDate
	}()

	buildinfo.Version = "2026.1.2"
	buildinfo.Commit = "abcdef1234567890"
	buildinfo.Date = "2026-01-14T11:36:49Z"
	if got := buildInfoInline(); got != "v2026.1.2 · abcdef1 · 2026-01-14" {
		t.Fatalf("unexpected build info: %q", got)
	}

	buildinfo.Version = "dev"
	buildinfo.Commit = "none"
	buildinfo.Date = "unknown"
	if got := buildInfoInline(); got != "dev" {
		t.Fatalf("unexpected build info: %q", got)
	}
}
