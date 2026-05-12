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

func TestDetectInstallMethodForPathRecognizesExplicitGoBin(t *testing.T) {
	home := t.TempDir()
	gobin := filepath.Join(home, "custom", "bin")
	t.Setenv("HOME", home)
	t.Setenv("GOPATH", filepath.Join(home, "go"))
	t.Setenv("GOBIN", gobin)

	path := filepath.Join(gobin, "breyta")
	if got := detectInstallMethodForPath(path); got != InstallMethodGo {
		t.Fatalf("expected go install method for explicit GOBIN path %s, got %q", path, got)
	}
}

func TestDetectInstallMethodForPathDoesNotUseGoPathBinWhenGoBinSet(t *testing.T) {
	home := t.TempDir()
	gopath := filepath.Join(home, "go")
	gobin := filepath.Join(home, "custom", "bin")
	t.Setenv("HOME", home)
	t.Setenv("GOPATH", gopath)
	t.Setenv("GOBIN", gobin)

	path := filepath.Join(gopath, "bin", "breyta")
	if got := detectInstallMethodForPath(path); got != InstallMethodUnknown {
		t.Fatalf("expected unknown install method for GOPATH/bin while GOBIN is set, got %q", got)
	}
}

func TestDetectInstallMethodForPathRecognizesGoPathBinWhenGoBinUnset(t *testing.T) {
	home := t.TempDir()
	gopath := filepath.Join(home, "go")
	t.Setenv("HOME", filepath.Join(home, "home"))
	t.Setenv("GOPATH", gopath)
	t.Setenv("GOBIN", "")

	path := filepath.Join(gopath, "bin", "breyta")
	if got := detectInstallMethodForPath(path); got != InstallMethodGo {
		t.Fatalf("expected go install method for GOPATH/bin when GOBIN is unset, got %q", got)
	}
}

func TestDetectInstallMethodForPathRejectsUnrelatedPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOPATH", filepath.Join(home, "go"))
	t.Setenv("GOBIN", filepath.Join(home, "custom", "bin"))

	path := filepath.Join(home, "other", "bin", "breyta")
	if got := detectInstallMethodForPath(path); got != InstallMethodUnknown {
		t.Fatalf("expected unknown install method for unrelated path, got %q", got)
	}
}
