package skillsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/skilldocs"
	"github.com/breyta/breyta-cli/skills"
)

const (
	syncRequestTimeout       = 5 * time.Second
	statusWarningCachePeriod = 24 * time.Hour
)

var installBreytaSkillFiles = skills.InstallBreytaSkillFiles
var syncInstalledNow = SyncInstalledNow
var saveCacheFile = saveCache

func enabled() bool {
	return strings.TrimSpace(os.Getenv("BREYTA_NO_SKILL_SYNC")) == ""
}

type cacheFile struct {
	LastSyncedVersion       string    `json:"lastSyncedVersion,omitempty"`
	SyncedAt                time.Time `json:"syncedAt,omitempty"`
	LastStatusCheckedAt     time.Time `json:"lastStatusCheckedAt,omitempty"`
	LastStatusWarnings      []string  `json:"lastStatusWarnings,omitempty"`
	LastStatusBundleVersion string    `json:"lastStatusBundleVersion,omitempty"`
}

type SyncResult struct {
	InstalledProviders []skills.Provider                `json:"installedProviders,omitempty"`
	SyncedProviders    []skills.Provider                `json:"syncedProviders,omitempty"`
	DuplicateSkills    []skills.DuplicateInstalledSkill `json:"duplicateSkills,omitempty"`
	Warnings           []string                         `json:"warnings,omitempty"`
}

type ProviderStatus struct {
	Provider      skills.Provider `json:"provider"`
	Installed     bool            `json:"installed"`
	UpToDate      bool            `json:"upToDate,omitempty"`
	Outdated      bool            `json:"outdated,omitempty"`
	Directory     string          `json:"directory,omitempty"`
	File          string          `json:"file,omitempty"`
	MissingFiles  []string        `json:"missingFiles,omitempty"`
	StaleFiles    []string        `json:"staleFiles,omitempty"`
	UpdateCommand string          `json:"updateCommand,omitempty"`
}

type StatusResult struct {
	LatestVersion      string                           `json:"latestVersion,omitempty"`
	InstalledProviders []skills.Provider                `json:"installedProviders,omitempty"`
	Providers          []ProviderStatus                 `json:"providers"`
	DuplicateSkills    []skills.DuplicateInstalledSkill `json:"duplicateSkills,omitempty"`
	Warnings           []string                         `json:"warnings,omitempty"`
	Hint               string                           `json:"hint,omitempty"`
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

func saveCacheMutation(mutate func(*cacheFile)) error {
	c, _ := loadCache()
	mutate(&c)
	return saveCacheFile(c)
}

func installedProviders(home string) []skills.Provider {
	out := []skills.Provider{}
	for _, p := range AllProviders() {
		t, err := skills.Target(home, p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(t.File); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func AllProviders() []skills.Provider {
	return []skills.Provider{skills.ProviderCodex, skills.ProviderCursor, skills.ProviderClaude, skills.ProviderGemini}
}

func localPathForManifestFile(target skills.InstallTarget, rel string) string {
	if rel == "SKILL.md" {
		return target.File
	}
	return filepath.Join(target.Dir, filepath.FromSlash(rel))
}

func providerStatus(home string, provider skills.Provider, files map[string][]byte) (ProviderStatus, error) {
	target, err := skills.Target(home, provider)
	if err != nil {
		return ProviderStatus{}, err
	}
	status := ProviderStatus{
		Provider:      provider,
		Directory:     target.Dir,
		File:          target.File,
		UpdateCommand: fmt.Sprintf("breyta skills install --provider %s", provider),
	}
	if _, err := os.Stat(target.File); err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return status, err
	}
	status.Installed = true

	paths := make([]string, 0, len(files))
	for rel := range files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		want := files[rel]
		localPath := localPathForManifestFile(target, rel)
		got, err := os.ReadFile(localPath)
		if err != nil {
			if os.IsNotExist(err) {
				status.MissingFiles = append(status.MissingFiles, rel)
				continue
			}
			return status, err
		}
		if string(got) != string(want) {
			status.StaleFiles = append(status.StaleFiles, rel)
		}
	}
	status.UpToDate = len(status.MissingFiles) == 0 && len(status.StaleFiles) == 0
	status.Outdated = !status.UpToDate
	return status, nil
}

func outdatedSkillWarning(status ProviderStatus) string {
	if !status.Installed || !status.Outdated {
		return ""
	}
	parts := []string{}
	if len(status.StaleFiles) > 0 {
		parts = append(parts, "stale files: "+strings.Join(status.StaleFiles, ", "))
	}
	if len(status.MissingFiles) > 0 {
		parts = append(parts, "missing files: "+strings.Join(status.MissingFiles, ", "))
	}
	detail := strings.Join(parts, "; ")
	if detail == "" {
		detail = "installed files differ from the current bundle"
	}
	return fmt.Sprintf("warning: installed %s Breyta skill is outdated (%s). Update with `%s`.", status.Provider, detail, status.UpdateCommand)
}

func missingSkillWarning(status ProviderStatus) string {
	if status.Installed {
		return ""
	}
	return fmt.Sprintf("warning: %s Breyta skill is not installed. Install it with `%s`.", status.Provider, status.UpdateCommand)
}

func noInstalledSkillWarning() string {
	return "warning: Breyta agent skill is not installed for any supported agent. Agents may miss current flow guidance. Install it with `breyta skills install --provider all` and add repo guidance with `breyta init --agents-md`."
}

func cachedStatusWarnings(c cacheFile, now time.Time) ([]string, bool) {
	if c.LastStatusCheckedAt.IsZero() {
		return nil, false
	}
	if now.Sub(c.LastStatusCheckedAt) > statusWarningCachePeriod {
		return nil, false
	}
	return append([]string{}, c.LastStatusWarnings...), true
}

func syncProviders(home string, providers []skills.Provider, files map[string][]byte) ([]skills.Provider, error) {
	desiredMain := files["SKILL.md"]
	if len(desiredMain) == 0 {
		return nil, errors.New("missing required skill file: SKILL.md")
	}

	synced := make([]skills.Provider, 0, len(providers))
	var firstErr error
	for _, p := range providers {
		t, err := skills.Target(home, p)
		if err != nil {
			continue
		}
		backup, backedUp := backupCopyIfModified(t.File, desiredMain)
		if _, installErr := installBreytaSkillFiles(home, p, files); installErr == nil {
			synced = append(synced, p)
			continue
		} else if backedUp {
			// Best-effort rollback: restore the original file contents if install fails.
			_ = os.WriteFile(t.File, backup, 0o644)
			if firstErr == nil {
				firstErr = fmt.Errorf("provider %s sync failed: %w", p, installErr)
			}
		} else if firstErr == nil {
			firstErr = fmt.Errorf("provider %s sync failed: %w", p, installErr)
		}
	}
	return synced, firstErr
}

func duplicateBreytaSkills(home string, providers []skills.Provider) []skills.DuplicateInstalledSkill {
	duplicates := []skills.DuplicateInstalledSkill{}
	for _, provider := range providers {
		found, err := skills.FindDuplicateBreytaSkills(home, provider)
		if err != nil {
			continue
		}
		duplicates = append(duplicates, found...)
	}
	return duplicates
}

func duplicateSkillWarnings(duplicates []skills.DuplicateInstalledSkill) []string {
	if len(duplicates) == 0 {
		return nil
	}
	byProvider := map[skills.Provider][]skills.DuplicateInstalledSkill{}
	providers := []skills.Provider{}
	for _, duplicate := range duplicates {
		if _, ok := byProvider[duplicate.Provider]; !ok {
			providers = append(providers, duplicate.Provider)
		}
		byProvider[duplicate.Provider] = append(byProvider[duplicate.Provider], duplicate)
	}
	warnings := make([]string, 0, len(providers))
	for _, provider := range providers {
		if warning := skills.DuplicateBreytaSkillWarning(provider, byProvider[provider]); warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

// StatusInstalled compares installed Breyta skill bundles against the current docs API bundle.
// If providers is empty, only detected installed providers are checked.
func StatusInstalled(ctx context.Context, apiURL, token string, providers []skills.Provider) (StatusResult, error) {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return StatusResult{}, errors.New("missing api base url")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return StatusResult{}, err
	}
	if len(providers) == 0 {
		providers = installedProviders(home)
	}
	if len(providers) == 0 {
		return StatusResult{
			Providers: []ProviderStatus{},
			Hint:      "No installed Breyta skill bundle was found. Install one with `breyta skills install --provider all`.",
		}, nil
	}

	httpClient := &http.Client{Timeout: syncRequestTimeout}
	manifest, files, err := skilldocs.FetchBundle(ctx, httpClient, apiURL, token, skills.BreytaSkillSlug)
	if err != nil {
		return StatusResult{}, err
	}
	files = skilldocs.ApplyCLIOverrides(skills.BreytaSkillSlug, files)

	statuses := make([]ProviderStatus, 0, len(providers))
	warnings := []string{}
	for _, provider := range providers {
		status, err := providerStatus(home, provider, files)
		if err != nil {
			return StatusResult{}, err
		}
		statuses = append(statuses, status)
		if warning := missingSkillWarning(status); warning != "" {
			warnings = append(warnings, warning)
		} else if warning := outdatedSkillWarning(status); warning != "" {
			warnings = append(warnings, warning)
		}
	}
	duplicates := duplicateBreytaSkills(home, providers)
	warnings = append(warnings, duplicateSkillWarnings(duplicates)...)

	result := StatusResult{
		LatestVersion:      strings.TrimSpace(manifest.Version),
		InstalledProviders: installedProviders(home),
		Providers:          statuses,
		DuplicateSkills:    duplicates,
		Warnings:           warnings,
	}
	if len(statuses) == 0 {
		result.Hint = "No installed Breyta skill bundle was found. Install one with `breyta skills install --provider all`."
	}
	return result, nil
}

func ClearCachedStatusWarnings() {
	_ = saveCacheMutation(func(c *cacheFile) {
		c.LastStatusCheckedAt = time.Time{}
		c.LastStatusWarnings = nil
		c.LastStatusBundleVersion = ""
	})
}

// MaybeWarnMissingOrOutdatedInstalled checks whether Breyta agent guidance is
// installed and fresh at most once per cache period, then returns cached warnings
// on subsequent calls until the installed bundle is refreshed.
func MaybeWarnMissingOrOutdatedInstalled(ctx context.Context, apiURL, token string) []string {
	if !enabled() {
		return nil
	}
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	providers := installedProviders(home)

	now := time.Now()
	if c, err := loadCache(); err == nil {
		if warnings, ok := cachedStatusWarnings(c, now); ok {
			return warnings
		}
	}

	if len(providers) == 0 {
		warnings := []string{noInstalledSkillWarning()}
		_ = saveCacheMutation(func(c *cacheFile) {
			c.LastStatusCheckedAt = now
			c.LastStatusWarnings = warnings
			c.LastStatusBundleVersion = ""
		})
		return warnings
	}

	if strings.TrimSpace(token) == "" {
		return nil
	}

	res, err := StatusInstalled(ctx, apiURL, token, providers)
	if err != nil {
		return nil
	}
	warnings := append([]string{}, res.Warnings...)
	_ = saveCacheMutation(func(c *cacheFile) {
		c.LastStatusCheckedAt = now
		c.LastStatusWarnings = warnings
		c.LastStatusBundleVersion = strings.TrimSpace(res.LatestVersion)
	})
	return warnings
}

// MaybeWarnOutdatedInstalled is kept for older callers inside this module.
func MaybeWarnOutdatedInstalled(ctx context.Context, apiURL, token string) []string {
	return MaybeWarnMissingOrOutdatedInstalled(ctx, apiURL, token)
}

// SyncInstalledNow refreshes already-installed Breyta skills for all detected providers.
func SyncInstalledNow(ctx context.Context, apiURL, token string) (SyncResult, error) {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return SyncResult{}, errors.New("missing api base url")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return SyncResult{}, err
	}
	providers := installedProviders(home)
	if len(providers) == 0 {
		return SyncResult{}, nil
	}

	httpClient := &http.Client{Timeout: syncRequestTimeout}
	_, files, err := skilldocs.FetchBundle(ctx, httpClient, apiURL, token, skills.BreytaSkillSlug)
	if err != nil {
		return SyncResult{}, err
	}
	files = skilldocs.ApplyCLIOverrides(skills.BreytaSkillSlug, files)

	synced, err := syncProviders(home, providers, files)
	duplicates := duplicateBreytaSkills(home, providers)
	warnings := duplicateSkillWarnings(duplicates)
	if err != nil {
		return SyncResult{
			InstalledProviders: providers,
			SyncedProviders:    synced,
			DuplicateSkills:    duplicates,
			Warnings:           warnings,
		}, err
	}
	if len(synced) > 0 {
		ClearCachedStatusWarnings()
	}
	return SyncResult{
		InstalledProviders: providers,
		SyncedProviders:    synced,
		DuplicateSkills:    duplicates,
		Warnings:           warnings,
	}, nil
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

	ctx, cancel := context.WithTimeout(context.Background(), syncRequestTimeout)
	defer cancel()
	res, err := syncInstalledNow(ctx, apiURL, token)
	if len(res.SyncedProviders) > 0 {
		_ = saveCacheMutation(func(c *cacheFile) {
			c.LastSyncedVersion = currentVersion
			c.SyncedAt = time.Now()
			c.LastStatusCheckedAt = time.Now()
			c.LastStatusWarnings = nil
			c.LastStatusBundleVersion = ""
		})
	}
	if err != nil {
		return nil
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
