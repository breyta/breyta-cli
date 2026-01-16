package skills

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed breyta/SKILL.md breyta/references/* breyta/references/steps/*
var embedded embed.FS

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

func BreytaSkillMarkdown() ([]byte, error) {
	return embedded.ReadFile("breyta/SKILL.md")
}

func BreytaSkillDocs() ([]string, error) {
	entries, err := embedded.ReadDir("breyta/references")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join("breyta", "references", entry.Name()))
	}
	return paths, nil
}

func BreytaSkillStepDocs() ([]string, error) {
	entries, err := embedded.ReadDir("breyta/references/steps")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join("breyta", "references", "steps", entry.Name()))
	}
	return paths, nil
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

func InstallBreytaSkill(homeDir string, provider Provider) ([]string, error) {
	md, err := BreytaSkillMarkdown()
	if err != nil {
		return nil, err
	}
	docPaths, err := BreytaSkillDocs()
	if err != nil {
		return nil, err
	}
	stepPaths, err := BreytaSkillStepDocs()
	if err != nil {
		return nil, err
	}

	switch provider {
	case ProviderCodex, ProviderCursor, ProviderClaude:
		t := targetForProvider(homeDir, provider)
		if err := installToTarget(md, t); err != nil {
			return nil, err
		}
		written := []string{t.File}
		if written, err = installDocsToTarget(docPaths, t, written); err != nil {
			return nil, err
		}
		if written, err = installDocsToTarget(stepPaths, t, written); err != nil {
			return nil, err
		}
		return written, nil

	default:
		return nil, fmt.Errorf("unknown provider %q (expected: codex|cursor|claude)", provider)
	}
}

func installToTarget(content []byte, t InstallTarget) error {
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return err
	}
	// Intentionally overwrite: this is a managed artifact.
	return os.WriteFile(t.File, content, 0o644)
}

func installDocsToTarget(paths []string, t InstallTarget, written []string) ([]string, error) {
	for _, path := range paths {
		content, err := embedded.ReadFile(path)
		if err != nil {
			return nil, err
		}
		rel := strings.TrimPrefix(path, "breyta/")
		dest := filepath.Join(t.Dir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return nil, err
		}
		written = append(written, dest)
	}
	return written, nil
}
