package authstore

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStore_SetGet_TrimsAndValidates(t *testing.T) {
	s := &Store{}
	s.Set("  http://localhost:8090/ ", "  tok  ")

	got, ok := s.Get("http://localhost:8090")
	if !ok {
		t.Fatalf("expected token present")
	}
	if got != "tok" {
		t.Fatalf("expected trimmed token, got %q", got)
	}

	// Missing token should not be returned.
	s.Tokens["http://x"] = Record{Token: "   ", UpdatedAt: s.Tokens["http://localhost:8090"].UpdatedAt}
	if _, ok := s.Get("http://x"); ok {
		t.Fatalf("expected missing token")
	}

	// Blank inputs should be ignored.
	s.Set("", "tok2")
	s.Set("http://y", "")
	if _, ok := s.Get("http://y"); ok {
		t.Fatalf("expected not set")
	}
}

func TestStore_SaveAtomicAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	s := &Store{}
	s.Set("http://localhost:8090", "tok")
	if err := SaveAtomic(path, s); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("expected 0600 perms, got %o", st.Mode().Perm())
		}
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
	if loaded.Tokens == nil {
		t.Fatalf("expected non-nil tokens map")
	}
	got, ok := loaded.Get("http://localhost:8090/")
	if !ok || got != "tok" {
		t.Fatalf("expected token tok, got %q (ok=%v)", got, ok)
	}
}

func TestStore_Load_EmptyTokensMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	st, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st.Tokens == nil {
		t.Fatalf("expected tokens map initialized")
	}
}

func TestStore_UpdateAtomicSerializesWithSaveAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	initial := &Store{}
	initial.Set("https://example.test", "initial")
	if err := SaveAtomic(path, initial); err != nil {
		t.Fatalf("SaveAtomic initial: %v", err)
	}

	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- withStoreLock(path, func() error {
			close(lockHeld)
			<-releaseLock
			return nil
		})
	}()
	<-lockHeld

	saveDone := make(chan error, 1)
	go func() {
		next := &Store{}
		next.Set("https://example.test", "next")
		saveDone <- SaveAtomic(path, next)
	}()

	select {
	case err := <-saveDone:
		t.Fatalf("SaveAtomic completed before the store lock was released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseLock)
	if err := <-lockDone; err != nil {
		t.Fatalf("holding store lock: %v", err)
	}
	if err := <-saveDone; err != nil {
		t.Fatalf("SaveAtomic next: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if token, ok := loaded.Get("https://example.test"); !ok || token != "next" {
		t.Fatalf("expected serialized save to persist next token, got %q (ok=%v)", token, ok)
	}
}
