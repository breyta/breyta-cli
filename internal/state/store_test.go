package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeedDefault_Basics(t *testing.T) {
	st := SeedDefault("ws-acme")
	if st == nil || st.Workspaces == nil {
		t.Fatalf("expected state with workspaces")
	}
	ws := st.Workspaces["ws-acme"]
	if ws == nil {
		t.Fatalf("expected workspace ws-acme")
	}
	if strings.TrimSpace(ws.ID) != "ws-acme" {
		t.Fatalf("unexpected workspace id: %q", ws.ID)
	}
	if ws.Flows == nil || ws.Runs == nil || ws.Registry == nil {
		t.Fatalf("expected non-nil maps on workspace")
	}
	if ws.Flows["subscription-renewal"] == nil {
		t.Fatalf("expected seeded flow subscription-renewal")
	}
}

func TestSaveAtomicAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	st := SeedDefault("ws-acme")
	if err := SaveAtomic(path, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasSuffix(string(b), "\n") {
		t.Fatalf("expected trailing newline")
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Workspaces == nil || loaded.Workspaces["ws-acme"] == nil {
		t.Fatalf("expected workspace present after load")
	}
}
