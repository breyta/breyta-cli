package updatecheck

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Cache struct {
	LatestTag string    `json:"latestTag,omitempty"`
	ETag      string    `json:"etag,omitempty"`
	CheckedAt time.Time `json:"checkedAt,omitempty"`
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("missing user cache dir")
	}
	return filepath.Join(dir, "breyta", "update.json"), nil
}

func loadCache() (Cache, error) {
	p, err := cachePath()
	if err != nil {
		return Cache{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Cache{}, err
	}
	var c Cache
	if err := json.Unmarshal(b, &c); err != nil {
		return Cache{}, err
	}
	c.LatestTag = strings.TrimSpace(c.LatestTag)
	c.ETag = strings.TrimSpace(c.ETag)
	return c, nil
}

func saveCache(c Cache) error {
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
