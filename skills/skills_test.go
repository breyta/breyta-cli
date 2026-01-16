package skills

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBreytaFlowsCLISkillMarkdown_Loads(t *testing.T) {
	b, err := BreytaSkillMarkdown()
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("expected non-empty SKILL.md")
	}
	if !strings.Contains(string(b), "name: "+BreytaSkillSlug) {
		t.Fatalf("expected SKILL.md to include slug %q", BreytaSkillSlug)
	}
}

func TestInstallBreytaFlowsCLI_WritesFiles(t *testing.T) {
	home := t.TempDir()

	paths, err := InstallBreytaSkill(home, ProviderCodex)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(paths) < 3 {
		t.Fatalf("expected multiple install paths, got %d", len(paths))
	}

	wantCodex := filepath.Join(home, ".codex", "skills", BreytaSkillSlug, "SKILL.md")
	wantAuthoring := filepath.Join(home, ".codex", "skills", BreytaSkillSlug, "docs", "authoring-reference.md")
	wantHttp := filepath.Join(home, ".codex", "skills", BreytaSkillSlug, "docs", "steps", "http.md")

	found := map[string]bool{}
	for _, p := range paths {
		found[p] = true
	}
	if !found[wantCodex] {
		t.Fatalf("missing installed path %q (got: %#v)", wantCodex, paths)
	}
	if !found[wantAuthoring] {
		t.Fatalf("missing installed path %q (got: %#v)", wantAuthoring, paths)
	}
	if !found[wantHttp] {
		t.Fatalf("missing installed path %q (got: %#v)", wantHttp, paths)
	}
}

func TestTarget_ProviderPaths(t *testing.T) {
	home := "/tmp/home"

	if got, err := Target(home, ProviderCodex); err != nil || got.File != filepath.Join(home, ".codex", "skills", BreytaSkillSlug, "SKILL.md") {
		t.Fatalf("codex target mismatch: %+v (err=%v)", got, err)
	}
	if got, err := Target(home, ProviderCursor); err != nil || got.File != filepath.Join(home, ".cursor", "rules", BreytaSkillSlug, "RULE.md") {
		t.Fatalf("cursor target mismatch: %+v (err=%v)", got, err)
	}
	if got, err := Target(home, ProviderClaude); err != nil || got.File != filepath.Join(home, ".claude", "skills", BreytaSkillSlug, "SKILL.md") {
		t.Fatalf("claude target mismatch: %+v (err=%v)", got, err)
	}
}
