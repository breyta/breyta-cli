package parenrepair

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type Fix struct {
	Kind string `json:"kind"`
	At   int    `json:"at"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type Report struct {
	Changed         bool  `json:"changed"`
	UnclosedCount   int   `json:"unclosedCount"`
	DroppedCloses   int   `json:"droppedCloses"`
	ReplacedCloses  int   `json:"replacedCloses"`
	AppendedCloses  int   `json:"appendedCloses"`
	Fixes           []Fix `json:"fixes,omitempty"`
	UnterminatedStr bool  `json:"unterminatedString"`
}

var ErrUnterminatedString = errors.New("unterminated string literal")

func Check(s string) error {
	_, _, err := Repair(s, false)
	return err
}

func Repair(s string, includeFixes bool) (string, Report, error) {
	var report Report
	var out strings.Builder
	out.Grow(len(s) + 32)

	stack := make([]byte, 0, 64) // open delimiters: ( [ {

	inString := false
	inComment := false
	escape := false

	addFix := func(f Fix) {
		if includeFixes {
			report.Fixes = append(report.Fixes, f)
		}
	}

	for i := 0; i < len(s); i++ {
		b := s[i]

		if inComment {
			out.WriteByte(b)
			if b == '\n' {
				inComment = false
			}
			continue
		}

		if inString {
			out.WriteByte(b)
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}

		switch b {
		case ';':
			inComment = true
			out.WriteByte(b)
			continue
		case '"':
			inString = true
			out.WriteByte(b)
			continue
		case '(', '[', '{':
			stack = append(stack, b)
			out.WriteByte(b)
			continue
		case ')', ']', '}':
			if len(stack) == 0 {
				// Best effort: drop unexpected close. This is safer than inserting opens.
				report.Changed = true
				report.DroppedCloses++
				addFix(Fix{Kind: "drop-unexpected-close", At: i, From: string([]byte{b})})
				continue
			}
			want := closerFor(stack[len(stack)-1])
			if b != want {
				report.Changed = true
				report.ReplacedCloses++
				addFix(Fix{Kind: "replace-mismatched-close", At: i, From: string([]byte{b}), To: string([]byte{want})})
				b = want
			}
			stack = stack[:len(stack)-1]
			out.WriteByte(b)
			continue
		default:
			out.WriteByte(b)
		}
	}

	if inString {
		report.UnterminatedStr = true
		return s, report, ErrUnterminatedString
	}

	if len(stack) == 0 {
		return out.String(), report, nil
	}

	report.UnclosedCount = len(stack)
	report.AppendedCloses = len(stack)
	report.Changed = true

	outStr := out.String()
	trimmed, suffix := splitTrailingWhitespace(outStr)

	var closes strings.Builder
	closes.Grow(len(stack))
	for i := len(stack) - 1; i >= 0; i-- {
		closes.WriteByte(closerFor(stack[i]))
	}
	addFix(Fix{Kind: "append-missing-closes", At: len(trimmed), To: closes.String()})

	// If the file already ends with whitespace (especially newlines), keep it at the end.
	return trimmed + closes.String() + suffix, report, nil
}

func closerFor(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	default:
		// This should be impossible; treat as a hard error if it ever happens.
		panic(fmt.Sprintf("unexpected opener: %q", open))
	}
}

func splitTrailingWhitespace(s string) (trimmed string, suffix string) {
	// Keep trailing whitespace stable (newlines at end of file), but append repairs before it.
	i := len(s)
	for i > 0 {
		r := rune(s[i-1])
		if r <= unicode.MaxASCII {
			if !unicode.IsSpace(r) {
				break
			}
			i--
			continue
		}
		// For non-ASCII, fall back to a conservative approach: stop trimming.
		break
	}
	return s[:i], s[i:]
}
