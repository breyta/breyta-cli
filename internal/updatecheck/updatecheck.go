package updatecheck

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type InstallMethod string

const (
	InstallMethodUnknown InstallMethod = "unknown"
	InstallMethodBrew    InstallMethod = "brew"
)

const (
	testLatestTagEnv       = "BREYTA_UPDATE_TEST_LATEST_TAG"
	testForceUpdateEnv     = "BREYTA_UPDATE_TEST_FORCE"
	testInstallMethodEnv   = "BREYTA_UPDATE_TEST_INSTALL_METHOD" // "brew"|"unknown"
	testBrewAvailableEnv   = "BREYTA_UPDATE_TEST_BREW_AVAILABLE" // "1"|"0"
	defaultForcedLatestTag = "v3000.12.9999"
)

type Notice struct {
	Available      bool          `json:"available"`
	CurrentVersion string        `json:"currentVersion,omitempty"`
	LatestVersion  string        `json:"latestVersion,omitempty"`
	CheckedAt      time.Time     `json:"checkedAt,omitempty"`
	InstallMethod  InstallMethod `json:"installMethod,omitempty"`
	Upgrade        []string      `json:"upgrade,omitempty"`
}

func DetectInstallMethod() InstallMethod {
	if v := strings.TrimSpace(os.Getenv(testInstallMethodEnv)); v != "" {
		switch strings.ToLower(v) {
		case "brew":
			return InstallMethodBrew
		case "unknown":
			return InstallMethodUnknown
		}
	}

	exe, err := os.Executable()
	if err != nil || strings.TrimSpace(exe) == "" {
		return InstallMethodUnknown
	}
	resolved := exe
	if p, err := filepath.EvalSymlinks(exe); err == nil && strings.TrimSpace(p) != "" {
		resolved = p
	}
	// Homebrew installs formulas under .../Cellar/<name>/<version>/...
	sep := string(filepath.Separator)
	if strings.Contains(resolved, sep+"Cellar"+sep+"breyta"+sep) {
		return InstallMethodBrew
	}
	return InstallMethodUnknown
}

func BrewAvailable() bool {
	if v := strings.TrimSpace(os.Getenv(testBrewAvailableEnv)); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		}
	}
	_, err := exec.LookPath("brew")
	return err == nil
}

func CachedNotice(currentVersion string) *Notice {
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion == "" || currentVersion == "dev" {
		return nil
	}

	if latest, ok := testLatestTagOverride(); ok {
		newer, err := isUpdateAvailable(currentVersion, latest)
		if err != nil || !newer {
			return nil
		}
		n := &Notice{
			Available:      true,
			CurrentVersion: currentVersion,
			LatestVersion:  latest,
			CheckedAt:      time.Now(),
			InstallMethod:  DetectInstallMethod(),
		}
		if n.InstallMethod == InstallMethodBrew {
			n.Upgrade = []string{"brew", "upgrade", "breyta"}
		}
		return n
	}

	c, err := loadCache()
	if err != nil {
		return nil
	}
	if strings.TrimSpace(c.LatestTag) == "" {
		return nil
	}
	newer, err := isUpdateAvailable(currentVersion, c.LatestTag)
	if err != nil || !newer {
		return nil
	}
	n := &Notice{
		Available:      true,
		CurrentVersion: currentVersion,
		LatestVersion:  c.LatestTag,
		CheckedAt:      c.CheckedAt,
		InstallMethod:  DetectInstallMethod(),
	}
	if n.InstallMethod == InstallMethodBrew {
		n.Upgrade = []string{"brew", "upgrade", "breyta"}
	}
	return n
}

func CheckNow(ctx context.Context, currentVersion string, maxAge time.Duration) (*Notice, error) {
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion == "" || currentVersion == "dev" {
		return nil, nil
	}

	if _, ok := testLatestTagOverride(); ok {
		return CachedNotice(currentVersion), nil
	}

	c, _ := loadCache()
	if !c.CheckedAt.IsZero() && time.Since(c.CheckedAt) <= maxAge && strings.TrimSpace(c.LatestTag) != "" {
		if newer, err := isUpdateAvailable(currentVersion, c.LatestTag); err == nil && newer {
			n := CachedNotice(currentVersion)
			if n != nil {
				return n, nil
			}
		}
		return nil, nil
	}

	client := defaultHTTPClient()
	tag, etag, notModified, err := fetchLatestReleaseTag(ctx, client, c.ETag)
	if err != nil {
		return nil, err
	}
	if notModified {
		c.CheckedAt = time.Now()
		if strings.TrimSpace(etag) != "" {
			c.ETag = strings.TrimSpace(etag)
		}
		_ = saveCache(c)
		return CachedNotice(currentVersion), nil
	}

	c.LatestTag = tag
	if strings.TrimSpace(etag) != "" {
		c.ETag = etag
	}
	c.CheckedAt = time.Now()
	_ = saveCache(c)
	return CachedNotice(currentVersion), nil
}

func isUpdateAvailable(currentVersion, latestTag string) (bool, error) {
	cur, err := ParseCalVer(currentVersion)
	if err != nil {
		return false, err
	}
	lat, err := ParseCalVer(latestTag)
	if err != nil {
		return false, err
	}
	return cur.Compare(lat) < 0, nil
}

func testLatestTagOverride() (string, bool) {
	if v := strings.TrimSpace(os.Getenv(testLatestTagEnv)); v != "" {
		return v, true
	}
	if strings.TrimSpace(os.Getenv(testForceUpdateEnv)) != "" {
		return defaultForcedLatestTag, true
	}
	return "", false
}
