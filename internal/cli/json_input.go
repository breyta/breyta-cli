package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseJSONSource(raw string, filePath string, label string) (any, error) {
	raw = strings.TrimSpace(raw)
	filePath = strings.TrimSpace(filePath)
	if raw != "" && filePath != "" {
		return nil, fmt.Errorf("use either --%s or --%s-file, not both", label, label)
	}
	if filePath != "" {
		contents, err := readExplicitFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read %s file: %w", label, err)
		}
		raw = strings.TrimSpace(string(contents))
	}
	if raw == "" {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("invalid %s json: %w", label, err)
	}
	return value, nil
}

func parseAnyJSONInput(raw string, filePath string, label string) (any, error) {
	return parseJSONSource(raw, filePath, label)
}

func parseJSONObjectJSONInput(raw string, filePath string, label string) (map[string]any, error) {
	value, err := parseJSONSource(raw, filePath, label)
	if err != nil || value == nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	return object, nil
}

func parseJSONObjectFlag(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	value, err := parseJSONObjectJSONInput(raw, "", "input")
	if err != nil {
		return nil, err
	}
	return value, nil
}

func parseJSONObjectInputFlags(raw string, filePath string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	filePath = strings.TrimSpace(filePath)
	if raw != "" && filePath != "" {
		return nil, fmt.Errorf("--input and --input-file cannot be combined")
	}
	if filePath != "" {
		contents, err := readExplicitFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read --input-file: %w", err)
		}
		if strings.TrimSpace(string(contents)) == "" {
			return nil, fmt.Errorf("--input-file is empty; write a JSON object such as {}")
		}
		return parseJSONObjectFlag(string(contents))
	}
	return parseJSONObjectFlag(raw)
}

func parseJSONArrayJSONInput(raw string, filePath string, label string) ([]any, error) {
	value, err := parseJSONSource(raw, filePath, label)
	if err != nil || value == nil {
		return nil, err
	}
	if items, ok := value.([]any); ok {
		return items, nil
	}
	return []any{value}, nil
}

func stringSlice(value any) []string {
	var result []string
	items := sliceAny(value)
	if direct, ok := value.([]string); ok {
		items = make([]any, len(direct))
		for index, item := range direct {
			items[index] = item
		}
	}
	for _, item := range items {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}

func appendUniqueStrings(existing []string, values []string) []string {
	seen := make(map[string]bool, len(existing)+len(values))
	for _, value := range existing {
		seen[value] = true
	}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			existing = append(existing, value)
			seen[value] = true
		}
	}
	return existing
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func normalizeInstallTarget(target string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(target))
	if normalized == "" {
		return "draft", nil
	}
	if normalized == "draft" || normalized == "live" {
		return normalized, nil
	}
	return "", fmt.Errorf("invalid --target (expected draft or live)")
}
