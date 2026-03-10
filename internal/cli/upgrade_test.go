package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/breyta/breyta-cli/internal/skillsync"
	"github.com/breyta/breyta-cli/skills"
)

func TestUpgradeCommand_ChecksAndReturnsNotice(t *testing.T) {
	origVersion := buildinfo.Version
	buildinfo.Version = "v2026.1.1"
	defer func() { buildinfo.Version = origVersion }()

	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_UPDATE_TEST_LATEST_TAG", "v3000.12.9999")
	t.Setenv("BREYTA_UPDATE_TEST_INSTALL_METHOD", "brew")
	t.Setenv("BREYTA_UPDATE_TEST_BREW_AVAILABLE", "1")

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"upgrade", "--pretty"})

	if err := root.Execute(); err != nil {
		t.Fatalf("upgrade command failed: %v\nstderr:\n%s\nstdout:\n%s", err, errOut.String(), out.String())
	}

	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out.String())
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %#v", env["data"])
	}
	update, ok := data["update"].(map[string]any)
	if !ok {
		t.Fatalf("missing data.update: %#v", data["update"])
	}
	if got, _ := update["releaseUrl"].(string); got != "https://github.com/breyta/breyta-cli/releases/latest" {
		t.Fatalf("unexpected releaseUrl: %q", got)
	}
	if got, _ := update["available"].(bool); !got {
		t.Fatalf("expected available=true, got %#v", update["available"])
	}
	if got, _ := update["fixCommand"].(string); got != "breyta upgrade --all --yes" {
		t.Fatalf("unexpected fixCommand: %q", got)
	}
}

func TestUpgradeCommand_ApplyUsesUpgradeCommand(t *testing.T) {
	origVersion := buildinfo.Version
	buildinfo.Version = "v2026.1.1"
	defer func() { buildinfo.Version = origVersion }()

	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_UPDATE_TEST_LATEST_TAG", "v3000.12.9999")
	t.Setenv("BREYTA_UPDATE_TEST_INSTALL_METHOD", "brew")
	t.Setenv("BREYTA_UPDATE_TEST_BREW_AVAILABLE", "1")

	orig := runUpgradeCommand
	defer func() { runUpgradeCommand = orig }()
	origSync := syncInstalledSkills
	defer func() { syncInstalledSkills = origSync }()

	var called bool
	var got []string
	runUpgradeCommand = func(_ctx context.Context, argv []string, _out io.Writer, _errOut io.Writer) error {
		called = true
		got = append([]string{}, argv...)
		return nil
	}
	syncInstalledSkills = func(_ context.Context, _ string, _ string) (skillsync.SyncResult, error) {
		return skillsync.SyncResult{}, nil
	}

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"upgrade", "--apply", "--pretty"})

	if err := root.Execute(); err != nil {
		t.Fatalf("upgrade --apply failed: %v\nstderr:\n%s\nstdout:\n%s", err, errOut.String(), out.String())
	}
	if !called {
		t.Fatalf("expected upgrade command to be executed")
	}
	if len(got) != 3 || got[0] != "brew" || got[1] != "upgrade" || got[2] != "breyta" {
		t.Fatalf("unexpected upgrade command: %v", got)
	}
}

func TestUpgradeCommand_AllRequiresYes(t *testing.T) {
	origVersion := buildinfo.Version
	buildinfo.Version = "v2026.1.1"
	defer func() { buildinfo.Version = origVersion }()

	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_UPDATE_TEST_LATEST_TAG", "v3000.12.9999")

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"upgrade", "--all", "--pretty"})

	if err := root.Execute(); err == nil {
		t.Fatalf("expected error for missing --yes")
	}
}

func TestUpgradeCommand_AllRunsSkillsAndCLI(t *testing.T) {
	origVersion := buildinfo.Version
	buildinfo.Version = "v2026.1.1"
	defer func() { buildinfo.Version = origVersion }()

	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_UPDATE_TEST_LATEST_TAG", "v3000.12.9999")
	t.Setenv("BREYTA_UPDATE_TEST_INSTALL_METHOD", "brew")
	t.Setenv("BREYTA_UPDATE_TEST_BREW_AVAILABLE", "1")

	origUpgrade := runUpgradeCommand
	defer func() { runUpgradeCommand = origUpgrade }()
	origSync := syncInstalledSkills
	defer func() { syncInstalledSkills = origSync }()

	var upgradeCalled bool
	runUpgradeCommand = func(_ctx context.Context, argv []string, _out io.Writer, _errOut io.Writer) error {
		upgradeCalled = true
		if len(argv) != 3 || argv[0] != "brew" || argv[1] != "upgrade" || argv[2] != "breyta" {
			t.Fatalf("unexpected upgrade command argv: %v", argv)
		}
		return nil
	}

	var syncCalled bool
	syncInstalledSkills = func(_ctx context.Context, _apiURL string, _token string) (skillsync.SyncResult, error) {
		syncCalled = true
		return skillsync.SyncResult{
			InstalledProviders: []skills.Provider{skills.ProviderCodex},
			SyncedProviders:    []skills.Provider{skills.ProviderCodex},
		}, nil
	}

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"upgrade", "--all", "--yes", "--pretty"})

	if err := root.Execute(); err != nil {
		t.Fatalf("upgrade --all --yes failed: %v\nstderr:\n%s\nstdout:\n%s", err, errOut.String(), out.String())
	}
	if !syncCalled {
		t.Fatalf("expected skills sync to be executed")
	}
	if !upgradeCalled {
		t.Fatalf("expected cli upgrade to be executed")
	}
}

func TestUpgradeCommand_AllWithUnknownInstallDoesNotFail(t *testing.T) {
	origVersion := buildinfo.Version
	buildinfo.Version = "v2026.1.1"
	defer func() { buildinfo.Version = origVersion }()

	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_UPDATE_TEST_LATEST_TAG", "v3000.12.9999")
	t.Setenv("BREYTA_UPDATE_TEST_INSTALL_METHOD", "unknown")

	origSync := syncInstalledSkills
	defer func() { syncInstalledSkills = origSync }()
	syncInstalledSkills = func(_ctx context.Context, _apiURL, _token string) (skillsync.SyncResult, error) {
		return skillsync.SyncResult{}, errors.New("skills sync unavailable")
	}

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"upgrade", "--all", "--yes", "--pretty"})

	if err := root.Execute(); err != nil {
		t.Fatalf("upgrade --all --yes should not fail for unknown install method: %v\nstderr:\n%s\nstdout:\n%s", err, errOut.String(), out.String())
	}

	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out.String())
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data object: %#v", env["data"])
	}
	cliData, ok := data["cli"].(map[string]any)
	if !ok {
		t.Fatalf("missing cli data: %#v", data["cli"])
	}
	if got, _ := cliData["reason"].(string); got != "manual_upgrade_required" {
		t.Fatalf("expected manual_upgrade_required reason, got %q", got)
	}
}

func TestUpgradeCommand_OpenUsesReleasePage(t *testing.T) {
	origVersion := buildinfo.Version
	buildinfo.Version = "v2026.1.1"
	defer func() { buildinfo.Version = origVersion }()

	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_UPDATE_TEST_LATEST_TAG", "v3000.12.9999")

	orig := openReleasePage
	defer func() { openReleasePage = orig }()

	var opened string
	openReleasePage = func(u string) error {
		opened = u
		return nil
	}

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"upgrade", "--open", "--pretty"})

	if err := root.Execute(); err != nil {
		t.Fatalf("upgrade --open failed: %v\nstderr:\n%s\nstdout:\n%s", err, errOut.String(), out.String())
	}
	if opened != "https://github.com/breyta/breyta-cli/releases/latest" {
		t.Fatalf("expected release page URL, got %q", opened)
	}
}
