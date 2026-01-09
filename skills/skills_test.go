package skills

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBreytaFlowsCLISkillMarkdown_Loads(t *testing.T) {
	b, err := BreytaFlowsCLISkillMarkdown()
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("expected non-empty SKILL.md")
	}
	if !strings.Contains(string(b), "name: "+BreytaFlowsCLISlug) {
		t.Fatalf("expected SKILL.md to include slug %q", BreytaFlowsCLISlug)
	}
}

func TestInstallBreytaFlowsCLI_WritesFiles(t *testing.T) {
	home := t.TempDir()

	paths, err := InstallBreytaFlowsCLI(home, ProviderCodex)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 install path, got %d", len(paths))
	}

	wantCodex := filepath.Join(home, ".codex", "skills", BreytaFlowsCLISlug, "SKILL.md")

	found := map[string]bool{}
	for _, p := range paths {
		found[p] = true
	}
	if !found[wantCodex] {
		t.Fatalf("missing installed path %q (got: %#v)", wantCodex, paths)
	}
}

func TestTarget_ProviderPaths(t *testing.T) {
	home := "/tmp/home"

	if got, err := Target(home, ProviderCodex); err != nil || got.File != filepath.Join(home, ".codex", "skills", BreytaFlowsCLISlug, "SKILL.md") {
		t.Fatalf("codex target mismatch: %+v (err=%v)", got, err)
	}
	if got, err := Target(home, ProviderCursor); err != nil || got.File != filepath.Join(home, ".cursor", "rules", BreytaFlowsCLISlug, "RULE.md") {
		t.Fatalf("cursor target mismatch: %+v (err=%v)", got, err)
	}
	if got, err := Target(home, ProviderClaude); err != nil || got.File != filepath.Join(home, ".claude", "skills", BreytaFlowsCLISlug, "SKILL.md") {
		t.Fatalf("claude target mismatch: %+v (err=%v)", got, err)
	}
}
