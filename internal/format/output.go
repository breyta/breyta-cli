package format

import (
        "encoding/json"
        "fmt"
        "io"
)

// Write writes output in the requested format.
//
// Supported formats:
// - json (default)
// - edn
func Write(w io.Writer, v any, format string, pretty bool) error {
        switch format {
        case "", "json":
                return WriteJSON(w, v, pretty)
        case "edn":
                return WriteEDN(w, v, pretty)
        default:
                return fmt.Errorf("unknown format: %s", format)
        }
}

// WriteJSON writes strict JSON output for CLI commands.
//
// NOTE: We intentionally keep output strict JSON only. If you need to
// communicate how to fetch more data, use a `meta` object or `_hint` fields.
func WriteJSON(w io.Writer, v any, pretty bool) error {
        var b []byte
        var err error
        if pretty {
                b, err = json.MarshalIndent(v, "", "  ")
        } else {
                b, err = json.Marshal(v)
        }
        if err != nil {
                return err
        }

        _, err = fmt.Fprintln(w, string(b))
        return err
}
