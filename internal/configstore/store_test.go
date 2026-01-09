package configstore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveAtomicAndLoad_RoundTripAndTrim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	st := &Store{APIURL: "  http://localhost:8090  ", WorkspaceID: "  ws-acme  "}
	if err := SaveAtomic(path, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("expected 0600 perms, got %o", fi.Mode().Perm())
		}
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.APIURL != "http://localhost:8090" {
		t.Fatalf("expected trimmed api url, got %q", loaded.APIURL)
	}
	if loaded.WorkspaceID != "ws-acme" {
		t.Fatalf("expected trimmed workspace id, got %q", loaded.WorkspaceID)
	}
}

func TestSaveAtomic_Validations(t *testing.T) {
	if err := SaveAtomic("", &Store{}); err == nil {
		t.Fatalf("expected error for missing path")
	}
	if err := SaveAtomic("x.json", nil); err == nil {
		t.Fatalf("expected error for missing store")
	}
}

func TestLoad_Validations(t *testing.T) {
	if _, err := Load(""); err == nil {
		t.Fatalf("expected error for missing path")
	}
}
