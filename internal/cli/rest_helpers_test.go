package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

func TestWriteREST_ErrorEnvelopeIncludesStatusMeta(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	app := &App{WorkspaceID: "ws-acme"}

	err := writeREST(cmd, app, 401, map[string]any{
		"ok":          false,
		"workspaceId": "ws-acme",
		"error":       "message='Missing webhook signature in header: X-Signature'",
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	var got map[string]any
	if decodeErr := json.Unmarshal(out.Bytes(), &got); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}

	meta, _ := got["meta"].(map[string]any)
	if meta == nil {
		t.Fatalf("expected meta, got %v", got)
	}
	if status, _ := meta["status"].(float64); status != 401 {
		t.Fatalf("expected meta.status=401, got %v (full=%v)", meta["status"], got)
	}
}
