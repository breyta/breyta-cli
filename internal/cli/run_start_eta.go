package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// avgDurationMsFromRunData reads the flow's historical average completed
// runtime (in milliseconds) from a run-start success response. The server
// places it inside `data.avgDurationMs` and OMITS it when there is no run
// history. The response is decoded loosely as map[string]any, so the number
// may arrive as float64 (encoding/json default), json.Number, or an integer
// type; handle each defensively. Returns 0 when missing or non-positive.
func avgDurationMsFromRunData(out map[string]any) int64 {
	if out == nil {
		return 0
	}
	data, ok := out["data"].(map[string]any)
	if !ok || data == nil {
		return 0
	}
	raw, ok := data["avgDurationMs"]
	if !ok || raw == nil {
		return 0
	}
	var ms int64
	switch v := raw.(type) {
	case float64:
		ms = int64(v)
	case float32:
		ms = int64(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			f, ferr := v.Float64()
			if ferr != nil {
				return 0
			}
			n = int64(f)
		}
		ms = n
	case int64:
		ms = v
	case int:
		ms = int64(v)
	case int32:
		ms = int64(v)
	default:
		return 0
	}
	if ms <= 0 {
		return 0
	}
	return ms
}

// humanizeAvgDurationMs renders a millisecond duration as a short,
// human-readable phrase for the run-start ETA notice:
//   - <1000ms       -> "less than 1 second"
//   - <60s          -> "N second(s)"
//   - <60m          -> "N minute(s)"
//   - otherwise     -> "N.N hours" (one decimal)
func humanizeAvgDurationMs(ms int64) string {
	if ms < 1000 {
		return "less than 1 second"
	}
	seconds := ms / 1000
	if seconds < 60 {
		return fmt.Sprintf("%d %s", seconds, plural(seconds, "second", "seconds"))
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%d %s", minutes, plural(minutes, "minute", "minutes"))
	}
	hours := float64(ms) / float64(3600*1000)
	return fmt.Sprintf("%.1f hours", hours)
}

func plural(n int64, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// printRunStartETA prints an immediate, human-readable runtime estimate to
// STDERR (never stdout, so --pretty/JSON output stays parseable) right after a
// successful run start. It is a no-op when the server did not include a
// positive avgDurationMs. When waiting is true, the notice also signals that
// the CLI is now polling for completion.
func printRunStartETA(cmd *cobra.Command, out map[string]any, waiting bool) {
	if cmd == nil {
		return
	}
	ms := avgDurationMsFromRunData(out)
	if ms <= 0 {
		return
	}
	human := humanizeAvgDurationMs(ms)
	if waiting {
		fmt.Fprintf(cmd.ErrOrStderr(), "Runs of this flow usually finish in about %s. Waiting for completion...\n", human)
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Runs of this flow usually finish in about %s.\n", human)
}

// addRunStartETAHint surfaces the runtime estimate to JSON/--pretty consumers
// by adding `avgDurationMs` (raw number) and `expectedRuntime` (human string)
// to the response meta map. It is a no-op when avgDurationMs is missing or
// non-positive, and it does not clobber existing meta values.
func addRunStartETAHint(out map[string]any) {
	ms := avgDurationMsFromRunData(out)
	if ms <= 0 {
		return
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	if _, exists := meta["avgDurationMs"]; !exists {
		meta["avgDurationMs"] = ms
	}
	if existing, exists := meta["expectedRuntime"]; !exists || strings.TrimSpace(scalarString(existing)) == "" {
		meta["expectedRuntime"] = humanizeAvgDurationMs(ms)
	}
}
