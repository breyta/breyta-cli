package skillsync

import (
	"errors"
	"os"
	"testing"

	"github.com/breyta/breyta-cli/skills"
)

func TestSyncProvidersContinuesAfterProviderFailure(t *testing.T) {
	home := t.TempDir()

	codexTarget, err := skills.Target(home, skills.ProviderCodex)
	if err != nil {
		t.Fatalf("codex target: %v", err)
	}
	if err := os.MkdirAll(codexTarget.Dir, 0o755); err != nil {
		t.Fatalf("mkdir codex target: %v", err)
	}
	if err := os.WriteFile(codexTarget.File, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed codex skill file: %v", err)
	}

	files := map[string][]byte{
		"SKILL.md": []byte("new"),
	}

	origInstall := installBreytaSkillFiles
	t.Cleanup(func() {
		installBreytaSkillFiles = origInstall
	})
	installBreytaSkillFiles = func(home string, provider skills.Provider, files map[string][]byte) ([]string, error) {
		if provider == skills.ProviderCodex {
			return nil, errors.New("codex install failed")
		}
		return skills.InstallBreytaSkillFiles(home, provider, files)
	}

	synced, syncErr := syncProviders(home, []skills.Provider{skills.ProviderCodex, skills.ProviderCursor}, files)
	if syncErr == nil {
		t.Fatalf("expected sync error when one provider fails")
	}
	if len(synced) != 1 || synced[0] != skills.ProviderCursor {
		t.Fatalf("expected only cursor to sync successfully, got %v", synced)
	}

	cursorTarget, err := skills.Target(home, skills.ProviderCursor)
	if err != nil {
		t.Fatalf("cursor target: %v", err)
	}
	content, err := os.ReadFile(cursorTarget.File)
	if err != nil {
		t.Fatalf("read cursor skill file: %v", err)
	}
	if string(content) != "new" {
		t.Fatalf("unexpected cursor skill content: %q", string(content))
	}
}

