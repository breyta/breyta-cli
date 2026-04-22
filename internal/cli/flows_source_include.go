package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const flowIncludeTag = "#flow/include"

func expandFlowSourceIncludes(sourcePath, flowLiteral string) (string, error) {
	baseDir := "."
	if trimmed := strings.TrimSpace(sourcePath); trimmed != "" {
		baseDir = filepath.Dir(trimmed)
	}
	baseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve flow source base dir: %w", err)
	}
	cache := map[string]string{}
	return expandFlowSourceIncludesFrom(baseDir, flowLiteral, nil, cache)
}

func expandFlowSourceIncludesFrom(baseDir, src string, stack []string, cache map[string]string) (string, error) {
	var out strings.Builder
	inString := false
	inComment := false
	escapeNext := false

	for i := 0; i < len(src); {
		ch := src[i]

		if inComment {
			out.WriteByte(ch)
			i++
			if ch == '\n' {
				inComment = false
			}
			continue
		}

		if inString {
			out.WriteByte(ch)
			i++
			if escapeNext {
				escapeNext = false
				continue
			}
			switch ch {
			case '\\':
				escapeNext = true
			case '"':
				inString = false
			}
			continue
		}

		if strings.HasPrefix(src[i:], flowIncludeTag) {
			j := i + len(flowIncludeTag)
			for j < len(src) {
				switch src[j] {
				case ' ', '\t', '\r', '\n', ',':
					j++
				default:
					goto includePath
				}
			}
		includePath:
			if j >= len(src) || src[j] != '"' {
				return "", fmt.Errorf("malformed %s form near byte %d: expected string path", flowIncludeTag, i)
			}
			token, includePath, next, err := readClojureStringToken(src, j)
			if err != nil {
				return "", fmt.Errorf("parse %s path near byte %d: %w", flowIncludeTag, i, err)
			}
			_ = token
			includeAbs := includePath
			if !filepath.IsAbs(includeAbs) {
				includeAbs = filepath.Join(baseDir, includeAbs)
			}
			includeAbs = filepath.Clean(includeAbs)
			expanded, err := readAndExpandFlowInclude(includeAbs, stack, cache)
			if err != nil {
				return "", err
			}
			out.WriteString(expanded)
			i = next
			continue
		}

		out.WriteByte(ch)
		i++
		switch ch {
		case ';':
			inComment = true
		case '"':
			inString = true
		}
	}

	return out.String(), nil
}

func readAndExpandFlowInclude(path string, stack []string, cache map[string]string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve include path %q: %w", path, err)
	}
	if expanded, ok := cache[absPath]; ok {
		return expanded, nil
	}
	for _, active := range stack {
		if active == absPath {
			chain := append(append([]string{}, stack...), absPath)
			return "", fmt.Errorf("flow source include cycle: %s", strings.Join(chain, " -> "))
		}
	}
	b, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read flow source include %q: %w", absPath, err)
	}
	expanded, err := expandFlowSourceIncludesFrom(filepath.Dir(absPath), string(b), append(stack, absPath), cache)
	if err != nil {
		return "", err
	}
	cache[absPath] = expanded
	return expanded, nil
}

func readClojureStringToken(src string, start int) (token string, value string, next int, err error) {
	if start < 0 || start >= len(src) || src[start] != '"' {
		return "", "", start, fmt.Errorf("expected opening quote")
	}
	escaped := false
	for i := start + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			escaped = !escaped
		case '"':
			if escaped {
				escaped = false
				continue
			}
			token = src[start : i+1]
			value, err = strconv.Unquote(token)
			if err != nil {
				return "", "", start, fmt.Errorf("invalid include path string %s: %w", token, err)
			}
			return token, value, i + 1, nil
		default:
			escaped = false
		}
	}
	return "", "", start, fmt.Errorf("unterminated string literal")
}
