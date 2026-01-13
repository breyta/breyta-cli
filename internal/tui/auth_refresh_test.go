package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/breyta/breyta-cli/internal/authstore"
)

func TestStoreAuthRecord_PersistsRefreshTokenAndExpiry(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	if err := storeAuthRecord("https://example.test", "tok-1", "ref-1", "3600"); err != nil {
		t.Fatalf("storeAuthRecord: %v", err)
	}

	st, err := authstore.Load(storePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rec, ok := st.GetRecord("https://example.test")
	if !ok {
		t.Fatalf("expected record present")
	}
	if rec.Token != "tok-1" || rec.RefreshToken != "ref-1" {
		t.Fatalf("unexpected record: token=%q refresh=%q", rec.Token, rec.RefreshToken)
	}
	if rec.ExpiresAt.IsZero() || time.Until(rec.ExpiresAt) < 59*time.Minute {
		t.Fatalf("expected ExpiresAt ~1h in future, got %v", rec.ExpiresAt)
	}
}

func TestResolveTokenForAPI_RefreshesNearExpiryToken(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/refresh" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":       true,
			"token":         "tok-2",
			"refresh_token": "ref-2",
			"expires_in":    "3600",
		})
	}))
	defer srv.Close()

	storePath := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	st := &authstore.Store{
		Tokens: map[string]authstore.Record{
			srv.URL: {
				Token:        "tok-1",
				RefreshToken: "ref-1",
				ExpiresAt:    time.Now().UTC().Add(30 * time.Second),
				UpdatedAt:    time.Now().UTC(),
			},
		},
	}
	if err := authstore.SaveAtomic(storePath, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	got, refreshed, err := resolveTokenForAPI(srv.URL, "")
	if err != nil {
		t.Fatalf("resolveTokenForAPI: %v", err)
	}
	if !refreshed {
		t.Fatalf("expected refreshed=true")
	}
	if got != "tok-2" {
		t.Fatalf("expected tok-2, got %q", got)
	}
	rt, _ := gotPayload["refreshToken"].(string)
	rs, _ := gotPayload["refresh_token"].(string)
	if strings.TrimSpace(rt) != "ref-1" || strings.TrimSpace(rs) != "ref-1" {
		t.Fatalf("expected both refreshToken and refresh_token sent, got: %+v", gotPayload)
	}

	loaded, err := authstore.Load(storePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rec, ok := loaded.GetRecord(srv.URL)
	if !ok {
		t.Fatalf("expected record present")
	}
	if rec.Token != "tok-2" || rec.RefreshToken != "ref-2" {
		t.Fatalf("expected updated record tok-2/ref-2, got token=%q refresh=%q", rec.Token, rec.RefreshToken)
	}
	if rec.ExpiresAt.IsZero() || time.Until(rec.ExpiresAt) < 59*time.Minute {
		t.Fatalf("expected ExpiresAt ~1h in future, got %v", rec.ExpiresAt)
	}
}
