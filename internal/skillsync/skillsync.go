package skillsync

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/skills"
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

func MaybeSyncInstalled(currentVersion string) error {
	if !enabled() {
		return nil
	}
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion == "" || currentVersion == "dev" {
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

	embeddedMD, err := skills.BreytaSkillMarkdown()
	if err != nil {
		return nil
	}

	anySynced := false
	for _, p := range installedProviders {
		t, err := skills.Target(home, p)
		if err != nil {
			continue
		}
		backupIfModified(t.File, embeddedMD)
		if _, err := skills.InstallBreytaSkill(home, p); err == nil {
			anySynced = true
		}
	}

	if anySynced {
		_ = saveCache(cacheFile{LastSyncedVersion: currentVersion, SyncedAt: time.Now()})
	}
	return nil
}

func backupIfModified(path string, desired []byte) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if string(b) == string(desired) {
		return
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	backup := path + ".bak-" + ts
	_ = os.Rename(path, backup)
}
