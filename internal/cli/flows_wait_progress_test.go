package cli

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

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

func TestStartRunWaitProgressIsIndependentOfPolling(t *testing.T) {
	var stderr synchronizedBuffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	var status atomic.Value
	status.Store("running")

	cancel := startRunWaitProgress(
		context.Background(),
		cmd,
		"wf-install-2",
		time.Now().Add(-time.Second),
		5*time.Millisecond,
		&status,
	)
	defer cancel()

	deadline := time.Now().Add(250 * time.Millisecond)
	for !strings.Contains(stderr.String(), "wf-install-2 is still running") &&
		time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if !strings.Contains(stderr.String(), "wf-install-2 is still running") {
		t.Fatalf("independent progress ticker produced no update:\n%s", stderr.String())
	}
}
