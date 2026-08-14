package configstore

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := &Store{APIURL: "https://engine.example.test", WorkspaceID: "ws-acme", RunConfigID: "profile-1"}
	if err := SaveAtomic(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.APIURL != want.APIURL || got.WorkspaceID != want.WorkspaceID || got.RunConfigID != want.RunConfigID {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}
