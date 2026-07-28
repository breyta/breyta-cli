package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestPrintRunWaitProgress(t *testing.T) {
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)

	printRunWaitProgress(cmd, "wf-install-1", "running", 0)
	printRunWaitProgress(cmd, "wf-install-1", "waiting-for-repair", 32*time.Second+900*time.Millisecond)

	got := stderr.String()
	for _, want := range []string{
		"Run wf-install-1 is running; waiting for completion...",
		"Run wf-install-1 is still waiting-for-repair after 32s; continuing to wait...",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("progress output missing %q:\n%s", want, got)
		}
	}
}
