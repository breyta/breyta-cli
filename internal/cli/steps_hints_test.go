package cli

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
)

func TestClientWithRequestTimeoutPreservesTransport(t *testing.T) {
	transport := http.DefaultTransport
	client := api.Client{HTTP: &http.Client{Timeout: 5 * time.Minute, Transport: transport}}

	configured := clientWithRequestTimeout(client, stepSidecarRequestTimeout)
	if configured.HTTP == client.HTTP {
		t.Fatal("expected sidecar timeout to use a client copy")
	}
	if configured.HTTP.Timeout != stepSidecarRequestTimeout {
		t.Fatalf("expected sidecar timeout %s, got %s", stepSidecarRequestTimeout, configured.HTTP.Timeout)
	}
	if configured.HTTP.Transport != transport {
		t.Fatal("expected sidecar timeout copy to preserve the configured transport")
	}
}

func TestRenderNextActionsBlock_FromHints(t *testing.T) {
	out := map[string]any{
		"ok": true,
		"_hints": []any{
			"breyta steps record --flow my-flow --type code --id make-output --params '{...}'",
			"breyta steps docs set my-flow make-output --markdown '...'",
		},
	}
	block := renderNextActionsBlock(out, 4)
	if block == "" {
		t.Fatalf("expected non-empty block")
	}
	if want := "Next actions:"; len(block) < len(want) || block[:len(want)] != want {
		t.Fatalf("expected block to start with %q, got %q", want, block)
	}
	if !strings.Contains(block, "breyta steps record") {
		t.Fatalf("expected block to include record hint, got %q", block)
	}
}

func TestRenderNextActionsBlock_Max(t *testing.T) {
	out := map[string]any{
		"ok": true,
		"_hints": []any{
			"a",
			"b",
			"c",
		},
	}
	block := renderNextActionsBlock(out, 2)
	if strings.Contains(block, "\n  - c") || strings.HasSuffix(block, "  - c") {
		t.Fatalf("expected block to be truncated, got %q", block)
	}
}
