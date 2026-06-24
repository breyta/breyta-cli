package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestHumanizeAvgDurationMs(t *testing.T) {
	cases := []struct {
		name string
		ms   int64
		want string
	}{
		{"sub-second", 500, "less than 1 second"},
		{"just-under-1s", 999, "less than 1 second"},
		{"exactly-1s", 1000, "1 second"},
		{"one-and-a-half-seconds", 1500, "1 second"},
		{"plural-seconds", 5000, "5 seconds"},
		{"just-under-a-minute", 59000, "59 seconds"},
		{"exactly-one-minute", 60000, "1 minute"},
		{"ninety-seconds", 90000, "1 minute"},
		{"plural-minutes", 125000, "2 minutes"},
		{"just-under-an-hour", 3540000, "59 minutes"},
		{"exactly-one-hour", 3600000, "1.0 hours"},
		{"ninety-minutes", 5400000, "1.5 hours"},
		{"two-hours", 7200000, "2.0 hours"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanizeAvgDurationMs(tc.ms); got != tc.want {
				t.Fatalf("humanizeAvgDurationMs(%d) = %q, want %q", tc.ms, got, tc.want)
			}
		})
	}
}

func TestAvgDurationMsFromRunData(t *testing.T) {
	cases := []struct {
		name string
		out  map[string]any
		want int64
	}{
		{
			name: "float64-default-decode",
			out:  map[string]any{"data": map[string]any{"avgDurationMs": float64(125000)}},
			want: 125000,
		},
		{
			name: "json-number-decode",
			out:  map[string]any{"data": map[string]any{"avgDurationMs": json.Number("125000")}},
			want: 125000,
		},
		{
			name: "int-decode",
			out:  map[string]any{"data": map[string]any{"avgDurationMs": 90000}},
			want: 90000,
		},
		{
			name: "missing-field",
			out:  map[string]any{"data": map[string]any{"started": true}},
			want: 0,
		},
		{
			name: "nil-value",
			out:  map[string]any{"data": map[string]any{"avgDurationMs": nil}},
			want: 0,
		},
		{
			name: "zero-value",
			out:  map[string]any{"data": map[string]any{"avgDurationMs": float64(0)}},
			want: 0,
		},
		{
			name: "negative-value",
			out:  map[string]any{"data": map[string]any{"avgDurationMs": float64(-5)}},
			want: 0,
		},
		{
			name: "missing-data",
			out:  map[string]any{"ok": true},
			want: 0,
		},
		{
			name: "nil-out",
			out:  nil,
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := avgDurationMsFromRunData(tc.out); got != tc.want {
				t.Fatalf("avgDurationMsFromRunData() = %d, want %d", got, tc.want)
			}
		})
	}
}

func newETATestCmd() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "test"}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	return cmd, &stdout, &stderr
}

func TestPrintRunStartETAEmittedWhenPresent(t *testing.T) {
	cmd, stdout, stderr := newETATestCmd()
	out := map[string]any{"data": map[string]any{"avgDurationMs": float64(125000)}}

	printRunStartETA(cmd, out, true)

	if got := stdout.String(); got != "" {
		t.Fatalf("expected clean stdout, got %q", got)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "usually finish in about 2 minutes") {
		t.Fatalf("stderr missing duration phrase: %q", errOut)
	}
	if !strings.Contains(errOut, "Waiting for completion...") {
		t.Fatalf("waiting notice missing the waiting suffix: %q", errOut)
	}
}

func TestPrintRunStartETANonWaitOmitsWaitingSuffix(t *testing.T) {
	cmd, _, stderr := newETATestCmd()
	out := map[string]any{"data": map[string]any{"avgDurationMs": float64(5000)}}

	printRunStartETA(cmd, out, false)

	errOut := stderr.String()
	if !strings.Contains(errOut, "usually finish in about 5 seconds.") {
		t.Fatalf("stderr missing duration phrase: %q", errOut)
	}
	if strings.Contains(errOut, "Waiting for completion") {
		t.Fatalf("non-wait notice should not mention waiting: %q", errOut)
	}
}

func TestPrintRunStartETASilentWhenAbsent(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  map[string]any
	}{
		{"missing", map[string]any{"data": map[string]any{"started": true}}},
		{"zero", map[string]any{"data": map[string]any{"avgDurationMs": float64(0)}}},
		{"no-data", map[string]any{"ok": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, stdout, stderr := newETATestCmd()
			printRunStartETA(cmd, tc.out, true)
			if got := stdout.String(); got != "" {
				t.Fatalf("expected clean stdout, got %q", got)
			}
			if got := stderr.String(); got != "" {
				t.Fatalf("expected silent stderr, got %q", got)
			}
		})
	}
}

func TestAddRunStartETAHintSetsMeta(t *testing.T) {
	out := map[string]any{"ok": true, "data": map[string]any{"avgDurationMs": float64(125000)}}

	addRunStartETAHint(out)

	meta, ok := out["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta map, got %#v", out["meta"])
	}
	if got := meta["avgDurationMs"]; got != int64(125000) {
		t.Fatalf("meta avgDurationMs = %#v, want int64(125000)", got)
	}
	if got := meta["expectedRuntime"]; got != "2 minutes" {
		t.Fatalf("meta expectedRuntime = %#v, want %q", got, "2 minutes")
	}
}

func TestAddRunStartETAHintNoClobber(t *testing.T) {
	out := map[string]any{
		"ok":   true,
		"data": map[string]any{"avgDurationMs": float64(125000)},
		"meta": map[string]any{"avgDurationMs": int64(999), "expectedRuntime": "preset"},
	}

	addRunStartETAHint(out)

	meta := out["meta"].(map[string]any)
	if got := meta["avgDurationMs"]; got != int64(999) {
		t.Fatalf("existing meta avgDurationMs clobbered: %#v", got)
	}
	if got := meta["expectedRuntime"]; got != "preset" {
		t.Fatalf("existing meta expectedRuntime clobbered: %#v", got)
	}
}

func TestAddRunStartETAHintNoOpWhenAbsent(t *testing.T) {
	out := map[string]any{"ok": true, "data": map[string]any{"started": true}}

	addRunStartETAHint(out)

	if _, ok := out["meta"]; ok {
		t.Fatalf("meta should not be created when avgDurationMs is absent: %#v", out["meta"])
	}
}
