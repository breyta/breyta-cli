package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/breyta/breyta-cli/internal/skilldocs"
	"github.com/breyta/breyta-cli/skills"
	"github.com/spf13/cobra"
)

func TestRequireSkillBundleCLI(t *testing.T) {
	originalVersion := buildinfo.Version
	t.Cleanup(func() { buildinfo.Version = originalVersion })
	buildinfo.Version = "2026.8.2"

	if err := requireSkillBundleCLI(skilldocs.Manifest{MinCLIVersion: "v2026.8.2"}); err != nil {
		t.Fatalf("equal version should be accepted: %v", err)
	}
	if err := requireSkillBundleCLI(skilldocs.Manifest{MinCLIVersion: "0.0.0"}); err != nil {
		t.Fatalf("unrestricted bundle should be accepted: %v", err)
	}
	if err := requireSkillBundleCLI(skilldocs.Manifest{MinCLIVersion: "v2026.8.3"}); err == nil || !strings.Contains(err.Error(), "upgrade the CLI") {
		t.Fatalf("older CLI should be rejected with an upgrade hint: %v", err)
	}
}

func TestInstallSkillProvidersContinuesAfterFailure(t *testing.T) {
	originalInstall := installProviderSkillFiles
	t.Cleanup(func() { installProviderSkillFiles = originalInstall })
	attempted := []skills.Provider{}
	installProviderSkillFiles = func(_ string, provider skills.Provider, _ map[string][]byte) ([]string, error) {
		attempted = append(attempted, provider)
		if provider == skills.ProviderCodex {
			return nil, errors.New("read only")
		}
		return []string{"SKILL.md"}, nil
	}

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := installSkillProviders(cmd, t.TempDir(), []skills.Provider{skills.ProviderCodex, skills.ProviderCursor}, map[string][]byte{"SKILL.md": []byte("test")}, false)
	if err == nil || !strings.Contains(err.Error(), "provider codex install") {
		t.Fatalf("expected first provider error, got %v", err)
	}
	if len(attempted) != 2 || attempted[1] != skills.ProviderCursor {
		t.Fatalf("expected every provider to be attempted, got %v", attempted)
	}
}
