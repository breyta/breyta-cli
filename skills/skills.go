package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const BreytaSkillSlug = "breyta"

type Provider string

const (
	ProviderCodex  Provider = "codex"
	ProviderCursor Provider = "cursor"
	ProviderClaude Provider = "claude"
)

type InstallTarget struct {
	Provider Provider
	Dir      string
	File     string
}

func Target(homeDir string, provider Provider) (InstallTarget, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return InstallTarget{}, errors.New("missing home dir")
	}
	switch provider {
	case ProviderCodex, ProviderCursor, ProviderClaude:
		return targetForProvider(homeDir, provider), nil
	default:
		return InstallTarget{}, fmt.Errorf("unknown provider %q (expected: codex|cursor|claude)", provider)
	}
}

func targetForProvider(homeDir string, provider Provider) InstallTarget {
	switch provider {
	case ProviderClaude:
		dir := filepath.Join(homeDir, ".claude", "skills", BreytaSkillSlug)
		return InstallTarget{Provider: provider, Dir: dir, File: filepath.Join(dir, "SKILL.md")}
	case ProviderCursor:
		dir := filepath.Join(homeDir, ".cursor", "rules", BreytaSkillSlug)
		return InstallTarget{Provider: provider, Dir: dir, File: filepath.Join(dir, "RULE.md")}
	default: // codex
		dir := filepath.Join(homeDir, ".codex", "skills", BreytaSkillSlug)
		return InstallTarget{Provider: ProviderCodex, Dir: dir, File: filepath.Join(dir, "SKILL.md")}
	}
}

func installToTarget(content []byte, t InstallTarget) error {
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return err
	}
	// Intentionally overwrite: this is a managed artifact.
	return os.WriteFile(t.File, content, 0o644)
}

func sanitizeRelPath(rel string) (string, bool) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", false
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", false
	}
	return cleaned, true
}

// InstallBreytaSkillFiles installs the breyta skill bundle from provided relative file contents.
// Required key: "SKILL.md". Additional files are installed under the provider-specific skill directory.
func InstallBreytaSkillFiles(homeDir string, provider Provider, files map[string][]byte) ([]string, error) {
	t, err := Target(homeDir, provider)
	if err != nil {
		return nil, err
	}
	main, ok := files["SKILL.md"]
	if !ok || len(main) == 0 {
		return nil, errors.New("missing required skill file: SKILL.md")
	}
	if err := installToTarget(main, t); err != nil {
		return nil, err
	}
	written := []string{t.File}

	keys := make([]string, 0, len(files))
	for rel := range files {
		if rel == "SKILL.md" {
			continue
		}
		keys = append(keys, rel)
	}
	sort.Strings(keys)

	for _, rel := range keys {
		safeRel, ok := sanitizeRelPath(rel)
		if !ok {
			return nil, fmt.Errorf("invalid skill file path %q", rel)
		}
		dest := filepath.Join(t.Dir, filepath.FromSlash(safeRel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dest, files[rel], 0o644); err != nil {
			return nil, err
		}
		written = append(written, dest)
	}
	return written, nil
}

type skillManifest struct {
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
}

// ExtractManifestPaths extracts manifest file paths from a docs API manifest payload.
func ExtractManifestPaths(manifestJSON []byte) ([]string, error) {
	var payload struct {
		Data skillManifest `json:"data"`
	}
	if err := json.Unmarshal(manifestJSON, &payload); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(payload.Data.Files))
	for _, f := range payload.Data.Files {
		out = append(out, f.Path)
	}
	return out, nil
}
