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
	InstallMethodGo      InstallMethod = "go"
)

const (
	testLatestTagEnv       = "BREYTA_UPDATE_TEST_LATEST_TAG"
	testForceUpdateEnv     = "BREYTA_UPDATE_TEST_FORCE"
	testInstallMethodEnv   = "BREYTA_UPDATE_TEST_INSTALL_METHOD" // "brew"|"go"|"unknown"
	testBrewAvailableEnv   = "BREYTA_UPDATE_TEST_BREW_AVAILABLE" // "1"|"0"
	defaultForcedLatestTag = "v3000.12.9999"
)

type Notice struct {
	Available      bool          `json:"available"`
	CurrentVersion string        `json:"currentVersion,omitempty"`
	LatestVersion  string        `json:"latestVersion,omitempty"`
	CheckedAt      time.Time     `json:"checkedAt,omitempty"`
	InstallMethod  InstallMethod `json:"installMethod,omitempty"`
	InstallPath    string        `json:"installPath,omitempty"`
	ReleaseURL     string        `json:"releaseUrl,omitempty"`
	Upgrade        []string      `json:"upgrade,omitempty"`
	FixCommand     string        `json:"fixCommand,omitempty"`
}

const DefaultFixCommand = "breyta upgrade --all --yes"
const ManualFixCommand = "breyta upgrade --open"
const GoInstallPackage = "github.com/breyta/breyta-cli/cmd/breyta@latest"

func DetectInstallMethod() InstallMethod {
	if v := strings.TrimSpace(os.Getenv(testInstallMethodEnv)); v != "" {
		switch strings.ToLower(v) {
		case "brew":
			return InstallMethodBrew
		case "go", "go-install", "goinstall":
			return InstallMethodGo
		case "unknown":
			return InstallMethodUnknown
		}
	}
	return detectInstallMethodForPath(DetectInstallPath())
}

func DetectInstallPath() string {
	exe, err := os.Executable()
	if err != nil || strings.TrimSpace(exe) == "" {
		return ""
	}
	resolved := exe
	if p, err := filepath.EvalSymlinks(exe); err == nil && strings.TrimSpace(p) != "" {
		resolved = p
	}
	return resolved
}

func detectInstallMethodForPath(resolved string) InstallMethod {
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return InstallMethodUnknown
	}
	// Homebrew installs formulas under .../Cellar/<name>/<version>/...
	sep := string(filepath.Separator)
	if strings.Contains(resolved, sep+"Cellar"+sep+"breyta"+sep) {
		return InstallMethodBrew
	}
	if isGoInstallPath(resolved) {
		return InstallMethodGo
	}
	return InstallMethodUnknown
}

func isGoInstallPath(resolved string) bool {
	resolved = filepath.Clean(strings.TrimSpace(resolved))
	if resolved == "." {
		return false
	}
	dir := filepath.Dir(resolved)
	for _, candidate := range goBinDirs() {
		if candidate != "" && dir == candidate {
			return true
		}
	}
	return false
}

func goBinDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		dir = filepath.Clean(dir)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	add(os.Getenv("GOBIN"))
	for _, gp := range filepath.SplitList(os.Getenv("GOPATH")) {
		if strings.TrimSpace(gp) != "" {
			add(filepath.Join(gp, "bin"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		add(filepath.Join(home, "go", "bin"))
	}
	return dirs
}

func upgradeCommandForInstallMethod(method InstallMethod) []string {
	switch method {
	case InstallMethodBrew:
		return []string{"brew", "upgrade", "breyta"}
	case InstallMethodGo:
		return []string{"go", "install", GoInstallPackage}
	default:
		return nil
	}
}

func fixCommandForInstallMethod(method InstallMethod) string {
	if len(upgradeCommandForInstallMethod(method)) > 0 {
		return DefaultFixCommand
	}
	return ManualFixCommand
}

func fillNoticeUpgrade(n *Notice) {
	if n == nil {
		return
	}
	n.Upgrade = upgradeCommandForInstallMethod(n.InstallMethod)
	n.FixCommand = fixCommandForInstallMethod(n.InstallMethod)
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
			InstallPath:    DetectInstallPath(),
			ReleaseURL:     ReleasePageURL,
		}
		fillNoticeUpgrade(n)
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
		InstallPath:    DetectInstallPath(),
		ReleaseURL:     ReleasePageURL,
	}
	fillNoticeUpgrade(n)
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
