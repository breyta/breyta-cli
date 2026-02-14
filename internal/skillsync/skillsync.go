package skillsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/skilldocs"
	"github.com/breyta/breyta-cli/skills"
)

const (
	syncRequestTimeout = 5 * time.Second
)

func enabled() bool {
	return strings.TrimSpace(os.Getenv("BREYTA_NO_SKILL_SYNC")) == ""
}

type cacheFile struct {
	LastSyncedVersion string    `json:"lastSyncedVersion,omitempty"`
	SyncedAt          time.Time `json:"syncedAt,omitempty"`
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("missing user cache dir")
	}
	return filepath.Join(dir, "breyta", "skillsync.json"), nil
}

func loadCache() (cacheFile, error) {
	p, err := cachePath()
	if err != nil {
		return cacheFile{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return cacheFile{}, err
	}
	var c cacheFile
	if err := json.Unmarshal(b, &c); err != nil {
		return cacheFile{}, err
	}
	c.LastSyncedVersion = strings.TrimSpace(c.LastSyncedVersion)
	return c, nil
}

func saveCache(c cacheFile) error {
	p, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func MaybeSyncInstalled(currentVersion, apiURL, token string) error {
	if !enabled() {
		return nil
	}
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion == "" || currentVersion == "dev" {
		return nil
	}
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return nil
	}

	cc, _ := loadCache()
	if strings.TrimSpace(cc.LastSyncedVersion) == currentVersion {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	installedProviders := []skills.Provider{}
	for _, p := range []skills.Provider{skills.ProviderCodex, skills.ProviderCursor, skills.ProviderClaude} {
		t, err := skills.Target(home, p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(t.File); err == nil {
			installedProviders = append(installedProviders, p)
		}
	}
	if len(installedProviders) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), syncRequestTimeout)
	defer cancel()
	httpClient := &http.Client{Timeout: syncRequestTimeout}
	_, files, err := skilldocs.FetchBundle(ctx, httpClient, apiURL, token, skills.BreytaSkillSlug)
	if err != nil {
		return nil
	}
	desiredMain := files["SKILL.md"]
	if len(desiredMain) == 0 {
		return nil
	}

	anySynced := false
	for _, p := range installedProviders {
		t, err := skills.Target(home, p)
		if err != nil {
			continue
		}
		backup, backedUp := backupCopyIfModified(t.File, desiredMain)
		if _, err := skills.InstallBreytaSkillFiles(home, p, files); err == nil {
			anySynced = true
		} else if backedUp {
			// Best-effort rollback: restore the original file contents if the install failed.
			// This avoids leaving users with a missing or corrupted skill file.
			_ = os.WriteFile(t.File, backup, 0o644)
		}
	}

	if anySynced {
		_ = saveCache(cacheFile{LastSyncedVersion: currentVersion, SyncedAt: time.Now()})
	}
	return nil
}

// MaybeSyncInstalledAsync performs best-effort sync without blocking command startup.
func MaybeSyncInstalledAsync(currentVersion, apiURL, token string) {
	go func() {
		_ = MaybeSyncInstalled(currentVersion, apiURL, token)
	}()
}

func backupCopyIfModified(path string, desired []byte) ([]byte, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if string(b) == string(desired) {
		return nil, false
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	backup := path + ".bak-" + ts
	// Best-effort: keep a copy for manual rollback.
	_ = os.WriteFile(backup, b, 0o644)
	return b, true
}
