package format

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// WriteEDN writes a strict EDN representation.
//
// Implementation note: we intentionally target a safe subset that covers our CLI
// payloads (maps, vectors, strings, numbers, booleans, nil). For structs, we
// first marshal/unmarshal through JSON so we use existing json tags / field
// naming.
func WriteEDN(w io.Writer, v any, pretty bool) error {
	// Convert structs -> map[string]any using JSON tags.
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var x any
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}

	var buf bytes.Buffer
	enc := ednEncoder{pretty: pretty, indent: 2}
	enc.writeAny(&buf, x, 0)
	buf.WriteByte('\n')
	_, err = w.Write(buf.Bytes())
	return err
}

type ednEncoder struct {
	pretty bool
	indent int
}

func (e ednEncoder) writeAny(buf *bytes.Buffer, v any, level int) {
	switch t := v.(type) {
	case nil:
		buf.WriteString("nil")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		buf.WriteString(strconv.Quote(t))
	case float64:
		// JSON numbers become float64 in interface{}.
		// If it looks like an int, print as int.
		if float64(int64(t)) == t {
			buf.WriteString(strconv.FormatInt(int64(t), 10))
			return
		}
		buf.WriteString(strconv.FormatFloat(t, 'f', -1, 64))
	case []any:
		e.writeVec(buf, t, level)
	case map[string]any:
		e.writeMap(buf, t, level)
	default:
		// Fallback: stringify.
		buf.WriteString(strconv.Quote(fmt.Sprintf("%v", v)))
	}
}

func (e ednEncoder) writeVec(buf *bytes.Buffer, xs []any, level int) {
	buf.WriteByte('[')
	if len(xs) == 0 {
		buf.WriteByte(']')
		return
	}
	if e.pretty {
		buf.WriteByte('\n')
	}
	for i, it := range xs {
		if e.pretty {
			buf.WriteString(strings.Repeat(" ", (level+1)*e.indent))
		}
		e.writeAny(buf, it, level+1)
		if i != len(xs)-1 {
			if e.pretty {
				buf.WriteByte('\n')
			} else {
				buf.WriteByte(' ')
			}
		}
	}
	if e.pretty {
		buf.WriteByte('\n')
		buf.WriteString(strings.Repeat(" ", level*e.indent))
	}
	buf.WriteByte(']')
}

func (e ednEncoder) writeMap(buf *bytes.Buffer, m map[string]any, level int) {
	buf.WriteByte('{')
	if len(m) == 0 {
		buf.WriteByte('}')
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if e.pretty {
		buf.WriteByte('\n')
	}
	for i, k := range keys {
		if e.pretty {
			buf.WriteString(strings.Repeat(" ", (level+1)*e.indent))
		}
		// Represent JSON keys as EDN keywords.
		buf.WriteByte(':')
		buf.WriteString(ednKeyword(k))
		buf.WriteByte(' ')
		e.writeAny(buf, m[k], level+1)
		if i != len(keys)-1 {
			if e.pretty {
				buf.WriteByte('\n')
			} else {
				buf.WriteByte(' ')
			}
		}
	}
	if e.pretty {
		buf.WriteByte('\n')
		buf.WriteString(strings.Repeat(" ", level*e.indent))
	}
	buf.WriteByte('}')
}

func ednKeyword(s string) string {
	// Keep it simple: allow common JSON field name chars.
	// Replace spaces just in case.
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
