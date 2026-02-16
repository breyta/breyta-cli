package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestDefaultRootHelpIncludesFeedback(t *testing.T) {
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root help: %v\nstderr:\n%s", err, errOut.String())
	}

	help := out.String()
	if !strings.Contains(help, "Send feedback and issue reports") {
		t.Fatalf("root help missing feedback command:\n%s", help)
	}
}
