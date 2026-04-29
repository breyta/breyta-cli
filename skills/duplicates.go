package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type DuplicateInstalledSkill struct {
	Provider Provider `json:"provider"`
	Name     string   `json:"name"`
	File     string   `json:"file"`
}

func FindDuplicateBreytaSkills(homeDir string, provider Provider) ([]DuplicateInstalledSkill, error) {
	return FindDuplicateInstalledSkillsByName(homeDir, provider, BreytaSkillSlug)
}

func FindDuplicateInstalledSkillsByName(homeDir string, provider Provider, name string) ([]DuplicateInstalledSkill, error) {
	target, err := Target(homeDir, provider)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}

	root := filepath.Dir(target.Dir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	mainFileName := filepath.Base(target.File)
	duplicates := []DuplicateInstalledSkill{}
	for _, entry := range entries {
		candidate := filepath.Join(root, entry.Name(), mainFileName)
		if samePath(candidate, target.File) {
			continue
		}
		content, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		declaredName, ok := frontmatterName(content)
		if !ok || !strings.EqualFold(declaredName, name) {
			continue
		}
		duplicates = append(duplicates, DuplicateInstalledSkill{
			Provider: provider,
			Name:     declaredName,
			File:     candidate,
		})
	}

	sort.Slice(duplicates, func(i, j int) bool {
		return duplicates[i].File < duplicates[j].File
	})
	return duplicates, nil
}

func DuplicateBreytaSkillWarning(provider Provider, duplicates []DuplicateInstalledSkill) string {
	if len(duplicates) == 0 {
		return ""
	}
	if len(duplicates) == 1 {
		return fmt.Sprintf("warning: found another installed %s skill file with frontmatter name %q at %s; Breyta CLI left it untouched. Remove or rename the duplicate manually if your agent loads the wrong skill.", provider, BreytaSkillSlug, duplicates[0].File)
	}

	paths := make([]string, 0, len(duplicates))
	for _, duplicate := range duplicates {
		paths = append(paths, "- "+duplicate.File)
	}
	return fmt.Sprintf("warning: found %d other installed %s skill files with frontmatter name %q; Breyta CLI left them untouched. Remove or rename duplicates manually if your agent loads the wrong skill:\n%s", len(duplicates), provider, BreytaSkillSlug, strings.Join(paths, "\n"))
}

func frontmatterName(content []byte) (string, bool) {
	body := strings.TrimLeft(string(content), "\ufeff \t\r\n")
	if body == "" {
		return "", false
	}

	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return "", false
	}

	if strings.TrimSpace(lines[0]) == "---" {
		for _, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if trimmed == "---" || trimmed == "..." {
				return "", false
			}
			if value, ok := parseNameLine(trimmed); ok {
				return value, true
			}
		}
		return "", false
	}

	limit := len(lines)
	if limit > 20 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if i == 0 {
				continue
			}
			return "", false
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			return "", false
		}
		if value, ok := parseNameLine(trimmed); ok {
			return value, true
		}
	}
	return "", false
}

func parseNameLine(line string) (string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok || strings.TrimSpace(key) != "name" {
		return "", false
	}
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if value == "" {
		return "", false
	}
	return value, true
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil {
		a = absA
	}
	if errB == nil {
		b = absB
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
