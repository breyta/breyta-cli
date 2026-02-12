package cli

import (
	"bytes"
	"errors"
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
