package browseropen

import (
	"errors"
	"reflect"
	"testing"
)

func TestOpenFor_Darwin_UsesOpen(t *testing.T) {
	restore := stubStartCommand(t, map[string]error{
		"open": nil,
	})
	defer restore()

	if err := openFor("darwin", false, "", "https://example.com"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	got := drainCalls()
	want := []call{{name: "open", args: []string{"https://example.com"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestOpenFor_Windows_TriesCandidatesUntilSuccess(t *testing.T) {
	restore := stubStartCommand(t, map[string]error{
		"rundll32":   errors.New("no"),
		"cmd":        nil,
		"powershell": nil,
		"explorer":   nil,
	})
	defer restore()

	if err := openFor("windows", false, "", "https://example.com"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	got := drainCalls()
	// Should stop at cmd.
	want := []call{
		{name: "rundll32", args: []string{"url.dll,FileProtocolHandler", "https://example.com"}},
		{name: "cmd", args: []string{"/c", "start", "", "https://example.com"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestOpenFor_LinuxWSL_TriesWSLOpenersFirst(t *testing.T) {
	restore := stubStartCommand(t, map[string]error{
		"wslview":        errors.New("missing"),
		"cmd.exe":        nil,
		"powershell.exe": nil,
		"explorer.exe":   nil,
		"xdg-open":       nil,
	})
	defer restore()

	if err := openFor("linux", true, "", "https://example.com"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	got := drainCalls()
	want := []call{
		{name: "wslview", args: []string{"https://example.com"}},
		{name: "cmd.exe", args: []string{"/c", "start", "", "https://example.com"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestOpenFor_Linux_RespectsBrowserEnv(t *testing.T) {
	restore := stubStartCommand(t, map[string]error{
		"br1":      errors.New("no"),
		"br2":      nil,
		"xdg-open": nil,
	})
	defer restore()

	// br1 gets URL appended; br2 uses placeholder replacement.
	browserEnv := "br1 --flag:br2 --arg=%s"
	if err := openFor("linux", false, browserEnv, "https://example.com"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	got := drainCalls()
	want := []call{
		{name: "br1", args: []string{"--flag", "https://example.com"}},
		{name: "br2", args: []string{"--arg=https://example.com"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestOpenFor_Linux_FallsBackToXDGOpen(t *testing.T) {
	restore := stubStartCommand(t, map[string]error{
		"br":       errors.New("no"),
		"xdg-open": nil,
	})
	defer restore()

	if err := openFor("linux", false, "br", "https://example.com"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	got := drainCalls()
	want := []call{
		{name: "br", args: []string{"https://example.com"}},
		{name: "xdg-open", args: []string{"https://example.com"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

type call struct {
	name string
	args []string
}

var (
	calls []call
)

func drainCalls() []call {
	out := append([]call(nil), calls...)
	calls = nil
	return out
}

func stubStartCommand(t *testing.T, results map[string]error) func() {
	t.Helper()
	prev := startCommand
	calls = nil
	startCommand = func(name string, args ...string) error {
		calls = append(calls, call{name: name, args: append([]string(nil), args...)})
		if err, ok := results[name]; ok {
			return err
		}
		return nil
	}
	return func() {
		startCommand = prev
		calls = nil
	}
}
