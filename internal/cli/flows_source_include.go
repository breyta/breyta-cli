package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const flowIncludeTag = "#flow/include"

func isClojureWhitespaceOrComma(ch byte) bool {
	switch ch {
	case ' ', '\t', '\r', '\n', ',':
		return true
	default:
		return false
	}
}

func readCommentEnd(src string, start int) int {
	i := start
	for i < len(src) && src[i] != '\n' {
		i++
	}
	if i < len(src) {
		i++
	}
	return i
}

func isClojureTokenDelimiter(ch byte) bool {
	switch ch {
	case ' ', '\t', '\r', '\n', ',', '(', ')', '[', ']', '{', '}', '"', ';', '\'', '`', '~', '@', '^':
		return true
	default:
		return false
	}
}

func readClojureTokenEnd(src string, start int) int {
	i := start
	for i < len(src) && !isClojureTokenDelimiter(src[i]) {
		i++
	}
	return i
}

func readDelimitedFormEnd(src string, start int, closeCh byte) (int, error) {
	for i := start + 1; i < len(src); {
		for i < len(src) && isClojureWhitespaceOrComma(src[i]) {
			i++
		}
		if i >= len(src) {
			return 0, fmt.Errorf("unterminated collection")
		}
		if src[i] == ';' {
			i = readCommentEnd(src, i)
			continue
		}
		if src[i] == closeCh {
			return i + 1, nil
		}
		next, err := readClojureFormEnd(src, i)
		if err != nil {
			return 0, err
		}
		i = next
	}
	return 0, fmt.Errorf("unterminated collection")
}

func readClojureFormEnd(src string, start int) (int, error) {
	i := start
	for {
		for i < len(src) && isClojureWhitespaceOrComma(src[i]) {
			i++
		}
		if i >= len(src) {
			return 0, fmt.Errorf("expected Clojure form")
		}
		if src[i] == ';' {
			i = readCommentEnd(src, i)
			continue
		}
		break
	}

	switch src[i] {
	case '"':
		_, _, next, err := readClojureStringToken(src, i)
		return next, err
	case '(':
		return readDelimitedFormEnd(src, i, ')')
	case '[':
		return readDelimitedFormEnd(src, i, ']')
	case '{':
		return readDelimitedFormEnd(src, i, '}')
	case '\'', '`', '@':
		return readClojureFormEnd(src, i+1)
	case '~':
		if i+1 < len(src) && src[i+1] == '@' {
			return readClojureFormEnd(src, i+2)
		}
		return readClojureFormEnd(src, i+1)
	case '^':
		metaEnd, err := readClojureFormEnd(src, i+1)
		if err != nil {
			return 0, err
		}
		return readClojureFormEnd(src, metaEnd)
	case '#':
		if strings.HasPrefix(src[i:], "#_") {
			return readClojureFormEnd(src, i+2)
		}
		if i+1 >= len(src) {
			return 0, fmt.Errorf("incomplete reader macro")
		}
		switch src[i+1] {
		case '{':
			return readDelimitedFormEnd(src, i+1, '}')
		case '(':
			return readDelimitedFormEnd(src, i+1, ')')
		case '"':
			_, _, next, err := readClojureStringToken(src, i+1)
			return next, err
		case '?':
			if i+2 < len(src) && src[i+2] == '@' {
				return readClojureFormEnd(src, i+3)
			}
			return readClojureFormEnd(src, i+2)
		default:
			tagEnd := readClojureTokenEnd(src, i+1)
			if tagEnd == i+1 {
				return 0, fmt.Errorf("unsupported reader macro")
			}
			return readClojureFormEnd(src, tagEnd)
		}
	default:
		return readClojureTokenEnd(src, i), nil
	}
}

func expandFlowSourceIncludes(sourcePath, flowLiteral string) (string, error) {
	baseDir := "."
	if trimmed := strings.TrimSpace(sourcePath); trimmed != "" {
		baseDir = filepath.Dir(trimmed)
	}
	baseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve flow source base dir: %w", err)
	}
	rootDir, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve flow source root dir: %w", err)
		}
		rootDir = baseDir
	}
	cache := map[string]string{}
	return expandFlowSourceIncludesFrom(baseDir, rootDir, flowLiteral, nil, cache)
}

func pathWithinRoot(rootDir, candidate string) bool {
	rel, err := filepath.Rel(rootDir, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func resolveFlowIncludePath(baseDir, rootDir, includePath string) (string, error) {
	if filepath.IsAbs(includePath) {
		return "", fmt.Errorf("absolute include paths are not allowed: %q", includePath)
	}
	includeAbs := filepath.Clean(filepath.Join(baseDir, includePath))
	includeAbs, err := filepath.Abs(includeAbs)
	if err != nil {
		return "", fmt.Errorf("resolve include path %q: %w", includePath, err)
	}
	includeReal, err := filepath.EvalSymlinks(includeAbs)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve include path %q: %w", includePath, err)
		}
		includeReal = includeAbs
	}
	if !pathWithinRoot(rootDir, includeReal) {
		return "", fmt.Errorf("flow source include %q escapes the flow source root", includePath)
	}
	return includeReal, nil
}

func expandFlowSourceIncludesFrom(baseDir, rootDir, src string, stack []string, cache map[string]string) (string, error) {
	var out strings.Builder
	inString := false
	inComment := false
	escapeNext := false
	readerDiscardPending := false

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

		if readerDiscardPending {
			if isClojureWhitespaceOrComma(ch) {
				out.WriteByte(ch)
				i++
				continue
			}
			if ch == ';' {
				next := readCommentEnd(src, i)
				out.WriteString(src[i:next])
				i = next
				continue
			}
			next, err := readClojureFormEnd(src, i)
			if err != nil {
				return "", fmt.Errorf("parse discarded form near byte %d: %w", i, err)
			}
			out.WriteString(src[i:next])
			i = next
			readerDiscardPending = false
			continue
		}

		if strings.HasPrefix(src[i:], "#_") {
			out.WriteString("#_")
			i += 2
			readerDiscardPending = true
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
			includeAbs, err := resolveFlowIncludePath(baseDir, rootDir, includePath)
			if err != nil {
				return "", err
			}
			expanded, err := readAndExpandFlowInclude(includeAbs, rootDir, stack, cache)
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

func readAndExpandFlowInclude(path, rootDir string, stack []string, cache map[string]string) (string, error) {
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
	expanded, err := expandFlowSourceIncludesFrom(filepath.Dir(absPath), rootDir, string(b), append(stack, absPath), cache)
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
