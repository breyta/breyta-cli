package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocsHintForCommand(t *testing.T) {
	root := NewRootCmd()

	flowsCmd, _, err := root.Find([]string{"flows"})
	if err != nil {
		t.Fatalf("find flows command: %v", err)
	}
	pushCmd, _, err := root.Find([]string{"flows", "push"})
	if err != nil {
		t.Fatalf("find flows push command: %v", err)
	}
	docsCmd, _, err := root.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("find docs command: %v", err)
	}

	if got := docsHintForCommand(root); got != "breyta docs find \"<topic>\"" {
		t.Fatalf("root docs hint = %q, want %q", got, "breyta docs find \"<topic>\"")
	}
	if got := docsHintForCommand(flowsCmd); got != "breyta docs find \"flows\"" {
		t.Fatalf("flows docs hint = %q, want %q", got, "breyta docs find \"flows\"")
	}
	if got := docsHintForCommand(pushCmd); got != "breyta docs find \"flows push\"" {
		t.Fatalf("flows push docs hint = %q, want %q", got, "breyta docs find \"flows push\"")
	}
	if got := docsHintForCommand(docsCmd); got != "breyta docs find \"<topic>\"" {
		t.Fatalf("docs command docs hint = %q, want %q", got, "breyta docs find \"<topic>\"")
	}
}

func TestHelpOutputIncludesDocsHint(t *testing.T) {
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"flows", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute help: %v\nstderr:\n%s", err, errOut.String())
	}

	help := out.String()
	if !strings.Contains(help, "Docs: breyta docs find \"flows\"") {
		t.Fatalf("help missing docs hint:\n%s", help)
	}
	if !strings.Contains(help, "Help: breyta help flows") {
		t.Fatalf("help missing command help hint:\n%s", help)
	}
}

func TestFlowsHelpHidesLegacyLifecycleCommands(t *testing.T) {
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"flows", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute help: %v\nstderr:\n%s", err, errOut.String())
	}

	help := out.String()
	for _, hiddenCmd := range []string{"\n  activate", "\n  deploy", "\n  draft", "\n  installations"} {
		if strings.Contains(help, hiddenCmd) {
			t.Fatalf("flows help leaked legacy command %q:\n%s", strings.TrimSpace(hiddenCmd), help)
		}
	}
	if !strings.Contains(help, "\n  release") || !strings.Contains(help, "\n  install") {
		t.Fatalf("flows help missing canonical lifecycle commands:\n%s", help)
	}
}

func TestInstallHelpOmitsLegacyAliasesButLegacyCommandWorks(t *testing.T) {
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"flows", "install", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install help: %v\nstderr:\n%s", err, errOut.String())
	}
	help := out.String()
	if strings.Contains(help, "Aliases:") {
		t.Fatalf("install help should not advertise legacy aliases:\n%s", help)
	}

	legacy := NewRootCmd()
	legacyOut := new(bytes.Buffer)
	legacyErr := new(bytes.Buffer)
	legacy.SetOut(legacyOut)
	legacy.SetErr(legacyErr)
	legacy.SetArgs([]string{"flows", "installations", "--help"})
	if err := legacy.Execute(); err != nil {
		t.Fatalf("execute legacy install help: %v\nstderr:\n%s", err, legacyErr.String())
	}
	if !strings.Contains(legacyOut.String(), "breyta flows installations [command]") {
		t.Fatalf("legacy installations command should remain executable:\n%s", legacyOut.String())
	}
}

func TestFlowsRunHelpHighlightsDefaultVsAdvancedTargeting(t *testing.T) {
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"flows", "run", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute flows run help: %v\nstderr:\n%s", err, errOut.String())
	}

	help := out.String()
	if !strings.Contains(help, "Default:") {
		t.Fatalf("flows run help missing default section:\n%s", help)
	}
	if !strings.Contains(help, "Advanced targeting:") {
		t.Fatalf("flows run help missing advanced section:\n%s", help)
	}
	if !strings.Contains(help, "Advanced: installation scope override") {
		t.Fatalf("flows run help missing advanced scope flag guidance:\n%s", help)
	}
}

func TestInstallPromoteHelpIsMarkedAdvanced(t *testing.T) {
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"flows", "install", "promote", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute flows install promote help: %v\nstderr:\n%s", err, errOut.String())
	}

	help := out.String()
	if !strings.Contains(help, "Advanced rollout command.") {
		t.Fatalf("flows install promote help missing advanced rollout description:\n%s", help)
	}
	if !strings.Contains(help, "Default path:") {
		t.Fatalf("flows install promote help missing default path context:\n%s", help)
	}
}

func TestWriteErrIncludesGuidance(t *testing.T) {
	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"--dev", "--api", "http://localhost:9999", "--token", "dev-user", "--workspace", "ws-acme", "steps", "run"})

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error")
	}

	stderr := errOut.String()
	if !strings.Contains(stderr, "missing --type") {
		t.Fatalf("stderr missing original error:\n%s", stderr)
	}
	if !strings.Contains(stderr, "Hint: run `breyta help steps run` for usage or `breyta docs find \"steps run\"` for docs.") {
		t.Fatalf("stderr missing guidance hint:\n%s", stderr)
	}

	// Also verify writeErr hint directly on a synthetic command path.
	customRoot := NewRootCmd()
	customCmd, _, findErr := customRoot.Find([]string{"flows", "create"})
	if findErr != nil {
		t.Fatalf("find flows create command: %v", findErr)
	}
	buf := new(bytes.Buffer)
	customCmd.SetErr(buf)
	_ = writeErr(customCmd, errors.New("boom"))
	if !strings.Contains(buf.String(), "Hint: run `breyta help flows create` for usage or `breyta docs find \"flows create\"` for docs.") {
		t.Fatalf("direct writeErr missing hint:\n%s", buf.String())
	}
}

func TestAPIErrorsIncludeGuidance(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error":{"message":"flow not found","hintRefs":[{"kind":"page","slug":"reference-cli-commands"},{"kind":"find","query":"flows show"}]}}`))
	}))
	defer srv.Close()

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{
		"--dev",
		"--api", srv.URL,
		"--token", "dev-user",
		"--workspace", "ws-acme",
		"flows", "show", "missing-flow",
	})

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error")
	}

	stderr := errOut.String()
	if !strings.Contains(stderr, "api error (status=400): flow not found") {
		t.Fatalf("stderr missing api error details:\n%s", stderr)
	}
	if !strings.Contains(stderr, "Docs: breyta docs show reference-cli-commands") {
		t.Fatalf("stderr missing docs page hint:\n%s", stderr)
	}
	if !strings.Contains(stderr, "Docs: breyta docs find \"flows show\"") {
		t.Fatalf("stderr missing docs find hint:\n%s", stderr)
	}
	if !strings.Contains(stderr, "Hint: run `breyta help flows show` for usage or `breyta docs find \"flows show\"` for docs.") {
		t.Fatalf("stderr missing guidance hint:\n%s", stderr)
	}
}

func TestRootNoArgsShowsHelp(t *testing.T) {
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute with no args: %v\nstderr:\n%s", err, errOut.String())
	}

	help := out.String()
	if !strings.Contains(help, "Usage:") {
		t.Fatalf("expected help output, got:\n%s", help)
	}
	if !strings.Contains(help, "auth") || !strings.Contains(help, "skills") || !strings.Contains(help, "workspaces") || !strings.Contains(help, "upgrade") {
		t.Fatalf("help missing expected command groups:\n%s", help)
	}
}

func TestRootHasCLIOnboardingCommands(t *testing.T) {
	root := NewRootCmd()

	if _, _, err := root.Find([]string{"auth", "login"}); err != nil {
		t.Fatalf("missing auth login command: %v", err)
	}
	if _, _, err := root.Find([]string{"skills", "install"}); err != nil {
		t.Fatalf("missing skills install command: %v", err)
	}
	if _, _, err := root.Find([]string{"workspaces", "use"}); err != nil {
		t.Fatalf("missing workspaces use command: %v", err)
	}
}
