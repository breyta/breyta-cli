package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
	if got, err := Target(home, ProviderGemini); err != nil || got.File != filepath.Join(home, ".gemini", "skills", BreytaSkillSlug, "SKILL.md") {
		t.Fatalf("gemini target mismatch: %+v (err=%v)", got, err)
	}
}

func TestInstallBreytaSkillFiles_CursorTargetAndReferences(t *testing.T) {
	home := t.TempDir()
	paths, err := InstallBreytaSkillFiles(home, ProviderCursor, map[string][]byte{
		"SKILL.md":                      []byte("name: breyta\n"),
		"references/reference-index.md": []byte("# Ref\n"),
	})
	if err != nil {
		t.Fatalf("install files: %v", err)
	}
	wantRule := filepath.Join(home, ".cursor", "rules", BreytaSkillSlug, "RULE.md")
	wantRef := filepath.Join(home, ".cursor", "rules", BreytaSkillSlug, "references", "reference-index.md")
	if _, err := os.Stat(wantRule); err != nil {
		t.Fatalf("expected RULE.md to exist: %v", err)
	}
	if _, err := os.Stat(wantRef); err != nil {
		t.Fatalf("expected reference doc to exist: %v", err)
	}
	found := map[string]bool{}
	for _, p := range paths {
		found[p] = true
	}
	if !found[wantRule] || !found[wantRef] {
		t.Fatalf("missing written paths. got=%#v", paths)
	}
}

func TestInstallBreytaSkillFiles_LeavesDuplicateNamedSkillUntouched(t *testing.T) {
	home := t.TempDir()
	duplicatePath := filepath.Join(home, ".codex", "skills", "legacy-breyta", "SKILL.md")
	duplicateContent := []byte("---\nname: breyta\n---\n# Legacy Breyta skill\n")
	if err := os.MkdirAll(filepath.Dir(duplicatePath), 0o755); err != nil {
		t.Fatalf("mkdir duplicate skill dir: %v", err)
	}
	if err := os.WriteFile(duplicatePath, duplicateContent, 0o644); err != nil {
		t.Fatalf("seed duplicate skill file: %v", err)
	}

	if _, err := InstallBreytaSkillFiles(home, ProviderCodex, map[string][]byte{
		"SKILL.md": []byte("---\nname: breyta\n---\n# Managed Breyta skill\n"),
	}); err != nil {
		t.Fatalf("install files: %v", err)
	}

	gotContent, err := os.ReadFile(duplicatePath)
	if err != nil {
		t.Fatalf("read duplicate skill file: %v", err)
	}
	if string(gotContent) != string(duplicateContent) {
		t.Fatalf("duplicate skill file was modified:\nwant %q\ngot  %q", string(duplicateContent), string(gotContent))
	}

	duplicates, err := FindDuplicateBreytaSkills(home, ProviderCodex)
	if err != nil {
		t.Fatalf("find duplicates: %v", err)
	}
	if len(duplicates) != 1 || duplicates[0].File != duplicatePath {
		t.Fatalf("expected duplicate path %q, got %#v", duplicatePath, duplicates)
	}
}

func TestInstallBreytaSkillFiles_TightensExistingManagedPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on windows")
	}
	home := t.TempDir()
	target, err := Target(home, ProviderCodex)
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	refPath := filepath.Join(target.Dir, "references", "reference-index.md")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatalf("mkdir refs: %v", err)
	}
	if err := os.WriteFile(target.File, []byte("old skill\n"), 0o644); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	if err := os.WriteFile(refPath, []byte("old ref\n"), 0o644); err != nil {
		t.Fatalf("seed ref: %v", err)
	}

	if _, err := InstallBreytaSkillFiles(home, ProviderCodex, map[string][]byte{
		"SKILL.md":                      []byte("new skill\n"),
		"references/reference-index.md": []byte("new ref\n"),
	}); err != nil {
		t.Fatalf("install files: %v", err)
	}

	for _, path := range []string{target.Dir, filepath.Dir(refPath)} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat dir %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != skillDirMode {
			t.Fatalf("expected dir %s perms %o, got %o", path, skillDirMode, got)
		}
	}
	for _, path := range []string{target.File, refPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat file %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != skillFileMode {
			t.Fatalf("expected file %s perms %o, got %o", path, skillFileMode, got)
		}
	}
}

func TestSanitizeRelPath_RejectsAbsoluteAndWindowsPaths(t *testing.T) {
	t.Parallel()

	cases := []string{
		"/etc/passwd",
		"\\windows\\system32\\drivers\\etc\\hosts",
		"C:/windows/system32",
		"C:\\windows\\system32",
		"//server/share/file.txt",
		"../escape.txt",
	}
	for _, tc := range cases {
		if got, ok := sanitizeRelPath(tc); ok {
			t.Fatalf("expected %q to be rejected, got %q", tc, got)
		}
	}
}
