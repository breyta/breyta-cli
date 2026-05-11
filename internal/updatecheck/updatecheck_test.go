package updatecheck

import (
	"context"
	"path/filepath"
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

func TestCheckNow_GoInstallOverride(t *testing.T) {
	t.Setenv(testLatestTagEnv, "v3000.12.9999")
	t.Setenv(testInstallMethodEnv, "go")

	n, err := CheckNow(context.Background(), "v2026.2.1", 24*time.Hour)
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if n == nil || !n.Available {
		t.Fatalf("expected available notice, got %+v", n)
	}
	if n.InstallMethod != InstallMethodGo {
		t.Fatalf("expected go install method, got %q", n.InstallMethod)
	}
	if len(n.Upgrade) != 3 || n.Upgrade[0] != "go" || n.Upgrade[1] != "install" || n.Upgrade[2] != GoInstallPackage {
		t.Fatalf("expected go install upgrade command, got %v", n.Upgrade)
	}
	if n.FixCommand != DefaultFixCommand {
		t.Fatalf("expected automatic fix command %q, got %q", DefaultFixCommand, n.FixCommand)
	}
}

func TestCheckNow_UnknownInstallUsesManualFixCommand(t *testing.T) {
	t.Setenv(testLatestTagEnv, "v3000.12.9999")
	t.Setenv(testInstallMethodEnv, "unknown")

	n, err := CheckNow(context.Background(), "v2026.2.1", 24*time.Hour)
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if n == nil || !n.Available {
		t.Fatalf("expected available notice, got %+v", n)
	}
	if n.InstallMethod != InstallMethodUnknown {
		t.Fatalf("expected unknown install method, got %q", n.InstallMethod)
	}
	if len(n.Upgrade) != 0 {
		t.Fatalf("expected no automatic upgrade command, got %v", n.Upgrade)
	}
	if n.FixCommand != ManualFixCommand {
		t.Fatalf("expected manual fix command %q, got %q", ManualFixCommand, n.FixCommand)
	}
}

func TestDetectInstallMethodForPathRecognizesGoBin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOPATH", "")
	t.Setenv("GOBIN", "")

	path := filepath.Join(home, "go", "bin", "breyta")
	if got := detectInstallMethodForPath(path); got != InstallMethodGo {
		t.Fatalf("expected go install method for %s, got %q", path, got)
	}
}
