package skillsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	if !strings.Contains(syncErr.Error(), "codex install failed") {
		t.Fatalf("expected install error to be wrapped, got %q", syncErr.Error())
	}
	if strings.Contains(syncErr.Error(), "%!w(<nil>)") {
		t.Fatalf("unexpected nil-wrapped sync error: %q", syncErr.Error())
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

func TestMaybeSyncInstalledSavesCacheAfterPartialSyncError(t *testing.T) {
	origSync := syncInstalledNow
	origSave := saveCacheFile
	t.Cleanup(func() {
		syncInstalledNow = origSync
		saveCacheFile = origSave
	})

	syncInstalledNow = func(ctx context.Context, apiURL, token string) (SyncResult, error) {
		return SyncResult{SyncedProviders: []skills.Provider{skills.ProviderCodex}}, errors.New("one provider failed")
	}

	saved := false
	saveCacheFile = func(c cacheFile) error {
		saved = true
		if c.LastSyncedVersion != "v1.2.3" {
			t.Fatalf("unexpected cached version: %q", c.LastSyncedVersion)
		}
		if c.SyncedAt.IsZero() {
			t.Fatalf("expected non-zero syncedAt")
		}
		return nil
	}

	if err := MaybeSyncInstalled("v1.2.3", "https://api.example.com", "token"); err != nil {
		t.Fatalf("MaybeSyncInstalled returned error: %v", err)
	}
	if !saved {
		t.Fatalf("expected cache to be saved on partial sync success")
	}
}

func TestSyncInstalledNowReportsDuplicateBreytaSkillNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	codexTarget, err := skills.Target(home, skills.ProviderCodex)
	if err != nil {
		t.Fatalf("codex target: %v", err)
	}
	if err := os.MkdirAll(codexTarget.Dir, 0o755); err != nil {
		t.Fatalf("mkdir codex target: %v", err)
	}
	if err := os.WriteFile(codexTarget.File, []byte("---\nname: breyta\n---\n# Current Breyta skill\n"), 0o644); err != nil {
		t.Fatalf("seed codex skill file: %v", err)
	}

	duplicatePath := filepath.Join(home, ".codex", "skills", "legacy-breyta", "SKILL.md")
	duplicateContent := []byte("---\nname: breyta\n---\n# Legacy Breyta skill\n")
	if err := os.MkdirAll(filepath.Dir(duplicatePath), 0o755); err != nil {
		t.Fatalf("mkdir duplicate skill dir: %v", err)
	}
	if err := os.WriteFile(duplicatePath, duplicateContent, 0o644); err != nil {
		t.Fatalf("seed duplicate skill file: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/skills/breyta/manifest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"schemaVersion": 1,
					"skillSlug":     "breyta",
					"version":       "test",
					"minCliVersion": "0.0.0",
					"keyId":         "test",
					"signature":     "",
					"files": []map[string]any{
						{"path": "SKILL.md", "sha256": "", "bytes": 0, "contentType": "text/markdown"},
					},
				},
			})
		case "/api/docs/skills/breyta/files/SKILL.md":
			_, _ = w.Write([]byte("---\nname: breyta\n---\n# Synced Breyta skill\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := SyncInstalledNow(context.Background(), srv.URL, "")
	if err != nil {
		t.Fatalf("sync installed: %v", err)
	}
	if len(res.DuplicateSkills) != 1 || res.DuplicateSkills[0].File != duplicatePath {
		t.Fatalf("expected duplicate path %q, got %#v", duplicatePath, res.DuplicateSkills)
	}
	if len(res.Warnings) != 1 ||
		!strings.Contains(res.Warnings[0], "frontmatter name \"breyta\"") ||
		!strings.Contains(res.Warnings[0], duplicatePath) ||
		!strings.Contains(res.Warnings[0], "left it untouched") {
		t.Fatalf("expected duplicate warning, got %#v", res.Warnings)
	}

	gotContent, err := os.ReadFile(duplicatePath)
	if err != nil {
		t.Fatalf("read duplicate skill file: %v", err)
	}
	if string(gotContent) != string(duplicateContent) {
		t.Fatalf("duplicate skill file was modified:\nwant %q\ngot  %q", string(duplicateContent), string(gotContent))
	}
}
