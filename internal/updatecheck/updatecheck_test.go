package updatecheck

import (
	"context"
	"testing"
	"time"
)

func TestCheckNow_TestOverrides(t *testing.T) {
	t.Setenv(testLatestTagEnv, "v3000.12.9999")
	t.Setenv(testInstallMethodEnv, "brew")
	t.Setenv(testBrewAvailableEnv, "1")

	n, err := CheckNow(context.Background(), "v2026.2.1", 24*time.Hour)
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if n == nil || !n.Available {
		t.Fatalf("expected available notice, got %+v", n)
	}
	if n.InstallMethod != InstallMethodBrew {
		t.Fatalf("expected brew install method, got %q", n.InstallMethod)
	}
	if len(n.Upgrade) == 0 || n.Upgrade[0] != "brew" {
		t.Fatalf("expected brew upgrade command, got %v", n.Upgrade)
	}
	if n.ReleaseURL != ReleasePageURL {
		t.Fatalf("expected release url %q, got %q", ReleasePageURL, n.ReleaseURL)
	}
	if n.FixCommand != DefaultFixCommand {
		t.Fatalf("expected fix command %q, got %q", DefaultFixCommand, n.FixCommand)
	}
}
