package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAgentDashboardIsRegistered(t *testing.T) {
	cmd := newAgentCmd(&App{})
	found, _, err := cmd.Find([]string{"dashboard", "show"})
	if err != nil {
		t.Fatalf("find dashboard show: %v", err)
	}
	if found.Name() != "show" {
		t.Fatalf("found command = %q", found.Name())
	}
}

func TestAgentDashboardReadCommands(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		command string
		payload map[string]any
	}{
		{
			name:    "show",
			args:    []string{"show"},
			command: "overview.dashboard.get",
			payload: map[string]any{},
		},
		{
			name:    "get alias",
			args:    []string{"get"},
			command: "overview.dashboard.get",
			payload: map[string]any{},
		},
		{
			name:    "catalog",
			args:    []string{"catalog"},
			command: "overview.dashboard.catalog",
			payload: map[string]any{},
		},
		{
			name:    "history default",
			args:    []string{"history"},
			command: "overview.dashboard.history",
			payload: map[string]any{"limit": agentDashboardDefaultHistoryLimit},
		},
		{
			name:    "history custom limit",
			args:    []string{"history", "--limit", "7"},
			command: "overview.dashboard.history",
			payload: map[string]any{"limit": 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, command, payload := captureAgentCommand(t)
			cmd := newAgentDashboardCmd(app)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if *command != tt.command {
				t.Fatalf("command = %q, want %q", *command, tt.command)
			}
			if !reflect.DeepEqual(*payload, tt.payload) {
				t.Fatalf("payload = %#v, want %#v", *payload, tt.payload)
			}
		})
	}
}

func TestAgentDashboardApplyBuildsInlineManifestPayload(t *testing.T) {
	app, command, payload := captureAgentCommand(t)
	cmd := newAgentDashboardApplyCmd(app)
	cmd.SetArgs([]string{
		"--expected-revision", "3",
		"--manifest", `{"schemaVersion":1,"title":"Marketing Hub","tabs":[]}`,
		"--change-summary", "  Add the SEO tab  ",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *command != "overview.dashboard.apply" {
		t.Fatalf("command = %q", *command)
	}
	want := map[string]any{
		"expectedRevision": 3,
		"manifest": map[string]any{
			"schemaVersion": float64(1),
			"title":         "Marketing Hub",
			"tabs":          []any{},
		},
		"changeSummary": "Add the SEO tab",
	}
	if !reflect.DeepEqual(*payload, want) {
		t.Fatalf("payload = %#v, want %#v", *payload, want)
	}
}

func TestAgentDashboardApplyAllowsExplicitZeroRevision(t *testing.T) {
	app, command, payload := captureAgentCommand(t)
	cmd := newAgentDashboardApplyCmd(app)
	cmd.SetArgs([]string{
		"--expected-revision", "0",
		"--manifest", `{"schemaVersion":1,"title":"Marketing Hub","tabs":[]}`,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *command != "overview.dashboard.apply" {
		t.Fatalf("command = %q", *command)
	}
	if (*payload)["expectedRevision"] != 0 {
		t.Fatalf("expectedRevision = %#v", (*payload)["expectedRevision"])
	}
}

func TestAgentDashboardApplyHelpExplainsInitialRevision(t *testing.T) {
	cmd := newAgentDashboardApplyCmd(&App{})
	flag := cmd.Flags().Lookup("expected-revision")
	if flag == nil {
		t.Fatal("missing --expected-revision flag")
	}
	if !strings.Contains(flag.Usage, "0 to create the first Marketing Hub") {
		t.Fatalf("usage = %q", flag.Usage)
	}
}

func TestAgentDashboardApplyReadsManifestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"title":"Marketing Hub","tabs":[]}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	app, command, payload := captureAgentCommand(t)
	cmd := newAgentDashboardApplyCmd(app)
	cmd.SetArgs([]string{
		"--expected-revision", "2",
		"--manifest-file", path,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *command != "overview.dashboard.apply" {
		t.Fatalf("command = %q", *command)
	}
	if (*payload)["expectedRevision"] != 2 {
		t.Fatalf("expectedRevision = %#v", (*payload)["expectedRevision"])
	}
	manifest, ok := (*payload)["manifest"].(map[string]any)
	if !ok || manifest["title"] != "Marketing Hub" {
		t.Fatalf("manifest = %#v", (*payload)["manifest"])
	}
	if _, ok := (*payload)["changeSummary"]; ok {
		t.Fatalf("empty change summary must be omitted: %#v", *payload)
	}
}

func TestAgentDashboardApplyRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{
			name:    "missing expected revision",
			args:    []string{"--manifest", `{}`},
			message: "missing --expected-revision",
		},
		{
			name:    "negative expected revision",
			args:    []string{"--expected-revision", "-1", "--manifest", `{}`},
			message: "--expected-revision must be zero or a positive integer",
		},
		{
			name:    "missing manifest",
			args:    []string{"--expected-revision", "1"},
			message: "provide exactly one of --manifest or --manifest-file",
		},
		{
			name:    "both manifest forms",
			args:    []string{"--expected-revision", "1", "--manifest", `{}`, "--manifest-file", "dashboard.json"},
			message: "--manifest and --manifest-file cannot be combined",
		},
		{
			name:    "both manifest flags with empty inline value",
			args:    []string{"--expected-revision", "1", "--manifest", ``, "--manifest-file", "dashboard.json"},
			message: "--manifest and --manifest-file cannot be combined",
		},
		{
			name:    "malformed manifest",
			args:    []string{"--expected-revision", "1", "--manifest", `{`},
			message: "invalid Marketing Hub manifest JSON",
		},
		{
			name:    "manifest is not an object",
			args:    []string{"--expected-revision", "1", "--manifest", `[]`},
			message: "Marketing Hub manifest must be a JSON object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, command, _ := captureAgentCommand(t)
			cmd := newAgentDashboardApplyCmd(app)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("error = %q, want %q", err, tt.message)
			}
			if *command != "" {
				t.Fatalf("API command must not run after validation failure: %q", *command)
			}
		})
	}
}

func TestAgentDashboardHistoryRejectsOutOfRangeLimit(t *testing.T) {
	for _, limit := range []string{"0", "51"} {
		t.Run(limit, func(t *testing.T) {
			app, command, _ := captureAgentCommand(t)
			cmd := newAgentDashboardHistoryCmd(app)
			cmd.SetArgs([]string{"--limit", limit})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--limit must be between 1 and 50") {
				t.Fatalf("error = %v", err)
			}
			if *command != "" {
				t.Fatalf("API command must not run after validation failure: %q", *command)
			}
		})
	}
}

func TestAgentDashboardRestoreBuildsPayload(t *testing.T) {
	app, command, payload := captureAgentCommand(t)
	cmd := newAgentDashboardRestoreCmd(app)
	cmd.SetArgs([]string{
		"--expected-revision", "8",
		"--revision", "5",
		"--change-summary", "  Restore the campaign view  ",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *command != "overview.dashboard.restore" {
		t.Fatalf("command = %q", *command)
	}
	want := map[string]any{
		"expectedRevision": 8,
		"revision":         5,
		"changeSummary":    "Restore the campaign view",
	}
	if !reflect.DeepEqual(*payload, want) {
		t.Fatalf("payload = %#v, want %#v", *payload, want)
	}
}

func TestAgentDashboardRestoreRejectsNonPositiveRevisions(t *testing.T) {
	tests := [][]string{
		{"--revision", "2"},
		{"--expected-revision", "3"},
		{"--expected-revision", "-1", "--revision", "2"},
		{"--expected-revision", "3", "--revision", "0"},
	}
	for _, args := range tests {
		app, command, _ := captureAgentCommand(t)
		cmd := newAgentDashboardRestoreCmd(app)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected an error for args %#v", args)
		}
		if *command != "" {
			t.Fatalf("API command must not run after validation failure: %q", *command)
		}
	}
}
