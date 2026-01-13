package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/breyta/breyta-cli/internal/authstore"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func httpJSON(status int, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(b))),
	}, nil
}

func TestRequireAPI_RefreshesTokenFromStoreWhenNotExplicit(t *testing.T) {
	var refreshCalls atomic.Int32
	var gotRefreshToken string

	authRefreshHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/auth/refresh" || r.Method != http.MethodPost {
				return httpJSON(404, map[string]any{"success": false, "error": "not found"})
			}
			refreshCalls.Add(1)
			payloadBytes, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(payloadBytes, &payload)
			if v, _ := payload["refreshToken"].(string); strings.TrimSpace(v) != "" {
				gotRefreshToken = strings.TrimSpace(v)
			}
			return httpJSON(200, map[string]any{
				"success":      true,
				"token":        "tok-2",
				"refreshToken": "ref-2",
				"expiresIn":    3600,
			})
		}),
	}
	t.Cleanup(func() { authRefreshHTTPClient = nil })

	baseURL := "https://example.test"

	storePath := filepath.Join(t.TempDir(), "auth.json")
	st := &authstore.Store{
		Tokens: map[string]authstore.Record{
			baseURL: {
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
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	app := &App{
		APIURL:        baseURL,
		Token:         "tok-1",
		TokenExplicit: false,
	}

	if err := requireAPI(app); err != nil {
		t.Fatalf("requireAPI: %v", err)
	}
	if app.Token != "tok-2" {
		t.Fatalf("expected refreshed token tok-2, got %q", app.Token)
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("expected refresh called once, got %d", refreshCalls.Load())
	}
	if gotRefreshToken != "ref-1" {
		t.Fatalf("expected refreshToken ref-1, got %q", gotRefreshToken)
	}

	loaded, err := authstore.Load(storePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rec, ok := loaded.GetRecord(baseURL)
	if !ok {
		t.Fatalf("expected stored record")
	}
	if rec.Token != "tok-2" || rec.RefreshToken != "ref-2" {
		t.Fatalf("expected store updated with tok-2/ref-2, got token=%q refresh=%q", rec.Token, rec.RefreshToken)
	}
}

func TestRequireAPI_DoesNotRefreshWhenTokenExplicit(t *testing.T) {
	var refreshCalls atomic.Int32

	authRefreshHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/auth/refresh" {
				refreshCalls.Add(1)
			}
			return httpJSON(404, map[string]any{"success": false, "error": "not found"})
		}),
	}
	t.Cleanup(func() { authRefreshHTTPClient = nil })

	storePath := filepath.Join(t.TempDir(), "auth.json")
	st := &authstore.Store{
		Tokens: map[string]authstore.Record{
			"https://example.test": {
				Token:        "tok-store",
				RefreshToken: "ref-store",
				ExpiresAt:    time.Now().UTC().Add(-30 * time.Second),
				UpdatedAt:    time.Now().UTC(),
			},
		},
	}
	if err := authstore.SaveAtomic(storePath, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	app := &App{
		APIURL:        "https://example.test",
		Token:         "tok-explicit",
		TokenExplicit: true,
	}
	if err := requireAPI(app); err != nil {
		t.Fatalf("requireAPI: %v", err)
	}
	if app.Token != "tok-explicit" {
		t.Fatalf("expected explicit token preserved, got %q", app.Token)
	}
	if refreshCalls.Load() != 0 {
		t.Fatalf("expected refresh not called, got %d", refreshCalls.Load())
	}
}
