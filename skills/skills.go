package skills

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed breyta-flows-cli/SKILL.md
var embedded embed.FS

const BreytaFlowsCLISlug = "breyta-flows-cli"

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

func BreytaFlowsCLISkillMarkdown() ([]byte, error) {
	return embedded.ReadFile("breyta-flows-cli/SKILL.md")
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
		dir := filepath.Join(homeDir, ".claude", "skills", "user", BreytaFlowsCLISlug)
		return InstallTarget{Provider: provider, Dir: dir, File: filepath.Join(dir, "SKILL.md")}
	case ProviderCursor:
		dir := filepath.Join(homeDir, ".codex", "cursor", "skills", BreytaFlowsCLISlug)
		return InstallTarget{Provider: provider, Dir: dir, File: filepath.Join(dir, "SKILL.md")}
	default: // codex
		dir := filepath.Join(homeDir, ".codex", "skills", BreytaFlowsCLISlug)
		return InstallTarget{Provider: ProviderCodex, Dir: dir, File: filepath.Join(dir, "SKILL.md")}
	}
}

func InstallBreytaFlowsCLI(homeDir string, provider Provider) ([]string, error) {
	md, err := BreytaFlowsCLISkillMarkdown()
	if err != nil {
		return nil, err
	}

	switch provider {
	case ProviderCodex, ProviderCursor, ProviderClaude:
		t := targetForProvider(homeDir, provider)
		if err := installToTarget(md, t); err != nil {
			return nil, err
		}
		return []string{t.File}, nil

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
