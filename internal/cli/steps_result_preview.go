package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	stepResultPreviewDefaultDepth  = 3
	stepResultPreviewDefaultItems  = 8
	stepResultPreviewDefaultRunes  = 1200
	stepResultPreviewStringRunes   = 160
	stepResultPreviewKeyLimit      = 20
	stepResultPreviewTestsItemHint = "Step test values are compact. Use --full for params/expected/actual."
)

type stepResultPreviewOptions struct {
	Full       bool
	Path       string
	MaxDepth   int
	MaxItems   int
	MaxRunes   int
	ResultFile string
}

func normalizeStepResultPreviewOptions(opts stepResultPreviewOptions) stepResultPreviewOptions {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = stepResultPreviewDefaultDepth
	}
	if opts.MaxItems <= 0 {
		opts.MaxItems = stepResultPreviewDefaultItems
	}
	if opts.MaxRunes <= 0 {
		opts.MaxRunes = stepResultPreviewDefaultRunes
	}
	opts.Path = strings.TrimSpace(opts.Path)
	opts.ResultFile = strings.TrimSpace(opts.ResultFile)
	return opts
}

func compactStepsRunResult(out map[string]any, stepID string, opts stepResultPreviewOptions) error {
	opts = normalizeStepResultPreviewOptions(opts)
	result, ok := stepsRunResultValue(out)
	if !ok {
		return nil
	}
	if opts.ResultFile != "" {
		if err := writeStepResultFile(opts.ResultFile, result); err != nil {
			return err
		}
		meta := ensureMeta(out)
		if meta != nil {
			meta["resultFile"] = opts.ResultFile
			meta["resultFileHint"] = "Full data.result was written locally; inspect focused paths from the file instead of printing --full output."
		}
	}
	if opts.Full {
		return nil
	}

	data := mapStringAny(out["data"])
	if data == nil {
		return nil
	}
	preview := buildStepResultPreview(stepID, result, opts)
	data["resultPreview"] = preview
	delete(data, "result")

	meta := ensureMeta(out)
	if meta != nil {
		meta["outputView"] = "compact"
		if _, exists := meta["hint"]; !exists {
			meta["hint"] = "Step result is compact. Use --result-path, --result-file, or --full for more."
		}
	}
	return nil
}

func compactStepsTestsVerifyResult(out map[string]any, opts stepResultPreviewOptions) {
	opts = normalizeStepResultPreviewOptions(opts)
	if opts.Full {
		return
	}
	data := mapStringAny(out["data"])
	items := sliceAny(data["items"])
	if len(items) == 0 {
		return
	}
	changed := false
	for _, itemAny := range items {
		item := mapStringAny(itemAny)
		if item == nil {
			continue
		}
		for _, key := range []string{"params", "expected", "actual"} {
			value, exists := item[key]
			if !exists {
				continue
			}
			item[key+"Preview"] = buildNamedValuePreview(key, value, opts)
			delete(item, key)
			changed = true
		}
	}
	if !changed {
		return
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	meta["outputView"] = "compact"
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = stepResultPreviewTestsItemHint
	}
}

func stepsRunResultValue(out map[string]any) (any, bool) {
	data := mapStringAny(out["data"])
	if data == nil {
		return nil, false
	}
	result, exists := data["result"]
	return result, exists
}

func writeStepResultFile(path string, result any) error {
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode --result-file: %w", err)
	}
	if dir := strings.TrimSpace(filepath.Dir(path)); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create --result-file directory: %w", err)
		}
	}
	return os.WriteFile(path, append(payload, '\n'), 0o600)
}

func buildStepResultPreview(stepID string, result any, opts stepResultPreviewOptions) map[string]any {
	binding := strings.TrimSpace(stepID)
	if binding == "" {
		binding = "result"
	}
	selected, path, found := selectStepResultPath(result, opts.Path)
	if !found {
		selected = nil
	}
	preview := buildNamedValuePreview(binding, selected, opts)
	if len(path) > 0 {
		preview["path"] = path
		preview["pathFound"] = found
	}
	preview["hint"] = "Preview shows the Clojure-shaped value to bind from this step, not the JSON envelope."
	if !found {
		preview["hint"] = "Requested result path was not found. Re-run with a different --result-path or --full if the shape is unclear."
	}
	return preview
}

func buildNamedValuePreview(name string, value any, opts stepResultPreviewOptions) map[string]any {
	rendered, truncated := renderStepClojurePreview(value, opts)
	return compactNonEmptyFields(map[string]any{
		"binding":         strings.TrimSpace(name),
		"format":          "clojure-value-preview",
		"shape":           valueShape(value),
		"keys":            objectPreviewKeys(value, stepResultPreviewKeyLimit),
		"items":           arrayPreviewCount(value),
		"value":           rendered,
		"truncated":       truncated,
		"previewRunes":    len([]rune(rendered)),
		"maxDepth":        opts.MaxDepth,
		"maxItems":        opts.MaxItems,
		"maxPreviewRunes": opts.MaxRunes,
	})
}

func selectStepResultPath(value any, rawPath string) (any, []string, bool) {
	parts := parseStepResultPath(rawPath)
	if len(parts) == 0 {
		return value, nil, true
	}
	current := value
	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			key := strings.TrimPrefix(part, ":")
			next, exists := v[key]
			if !exists && key != part {
				next, exists = v[part]
			}
			if !exists {
				return nil, parts, false
			}
			current = next
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, parts, false
			}
			current = v[idx]
		default:
			return nil, parts, false
		}
	}
	return current, parts, true
}

func parseStepResultPath(raw string) []string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return nil
	}
	if strings.HasPrefix(path, "[") && strings.HasSuffix(path, "]") {
		path = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(path, "["), "]"))
		fields := strings.Fields(path)
		parts := make([]string, 0, len(fields))
		for _, field := range fields {
			field = strings.Trim(field, `"'`)
			field = strings.TrimPrefix(field, ":")
			if field != "" {
				parts = append(parts, field)
			}
		}
		return parts
	}
	path = strings.TrimPrefix(path, ".")
	rawParts := strings.Split(path, ".")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, ":")
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func renderStepClojurePreview(value any, opts stepResultPreviewOptions) (string, bool) {
	rendered, truncated := renderStepClojureValue(value, opts.MaxDepth, opts.MaxItems)
	preview, charTruncated := truncateRunesWithFlag(rendered, opts.MaxRunes)
	return preview, truncated || charTruncated
}

func renderStepClojureValue(value any, depth int, maxItems int) (string, bool) {
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			return "{}", false
		}
		if depth <= 0 {
			return "{...}", true
		}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		truncated := false
		if len(keys) > maxItems {
			keys = keys[:maxItems]
			truncated = true
		}
		parts := make([]string, 0, len(keys)+1)
		for _, key := range keys {
			rendered, childTruncated := renderStepClojureValue(v[key], depth-1, maxItems)
			truncated = truncated || childTruncated
			parts = append(parts, renderStepMapKey(key)+" "+rendered)
		}
		if truncated && len(v) > len(keys) {
			parts = append(parts, "...")
		}
		return "{" + strings.Join(parts, ", ") + "}", truncated
	case []any:
		if len(v) == 0 {
			return "[]", false
		}
		if depth <= 0 {
			return "[...]", true
		}
		limit := len(v)
		truncated := false
		if limit > maxItems {
			limit = maxItems
			truncated = true
		}
		parts := make([]string, 0, limit+1)
		for i := 0; i < limit; i++ {
			rendered, childTruncated := renderStepClojureValue(v[i], depth-1, maxItems)
			truncated = truncated || childTruncated
			parts = append(parts, rendered)
		}
		if truncated && len(v) > limit {
			parts = append(parts, "...")
		}
		return "[" + strings.Join(parts, " ") + "]", truncated
	case string:
		preview, truncated := truncateRunesWithFlag(v, stepResultPreviewStringRunes)
		return strconv.Quote(preview), truncated
	case nil:
		return "nil", false
	case bool:
		if v {
			return "true", false
		}
		return "false", false
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), false
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), false
	case int:
		return strconv.Itoa(v), false
	case int64:
		return strconv.FormatInt(v, 10), false
	case int32:
		return strconv.FormatInt(int64(v), 10), false
	case json.Number:
		return v.String(), false
	default:
		b, err := json.Marshal(v)
		if err == nil {
			return string(b), false
		}
		return strconv.Quote(fmt.Sprintf("%v", v)), false
	}
}

func renderStepMapKey(key string) string {
	if isSafeClojureKeywordKey(key) {
		return ":" + key
	}
	return strconv.Quote(key)
}

func isSafeClojureKeywordKey(key string) bool {
	if strings.TrimSpace(key) == "" {
		return false
	}
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '-', '_', '?', '!', '*', '+', '/', '.', '<', '>', '=', '$', '%', '&':
			continue
		default:
			return false
		}
	}
	return true
}
