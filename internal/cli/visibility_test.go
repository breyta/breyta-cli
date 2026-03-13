package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestDefaultRootHelpIncludesFeedback(t *testing.T) {
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root help: %v\nstderr:\n%s", err, errOut.String())
	}

	help := out.String()
	if !strings.Contains(help, "Send feedback and issue reports") {
		t.Fatalf("root help missing feedback command:\n%s", help)
	}
}

func TestHelpSubcommandHidesMockSurface(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute help subcommand: %v\nstderr:\n%s", err, errOut.String())
	}

	help := out.String()
	for _, hidden := range []string{
		"\n  analytics",
		"\n  creator",
		"\n  demand",
		"\n  entitlements",
		"\n  payouts",
		"\n  pricing",
		"\n  purchases",
		"\n  registry",
		"\n  revenue",
		"--state string",
	} {
		if strings.Contains(help, hidden) {
			t.Fatalf("root help leaked hidden surface %q:\n%s", strings.TrimSpace(hidden), help)
		}
	}
}

func TestMockSurfaceCommandsRequireDevMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"demand", "top"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected demand top without --dev to fail")
	}
	if !strings.Contains(errOut.String(), "not part of the public CLI surface") {
		t.Fatalf("expected public-surface error, got stderr:\n%s", errOut.String())
	}
}
