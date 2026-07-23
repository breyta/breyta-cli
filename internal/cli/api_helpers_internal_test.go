package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
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

func TestRequireAPI_RefreshesTokenWellBeforeExpiry(t *testing.T) {
	var refreshCalls atomic.Int32

	authRefreshHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/auth/refresh" || r.Method != http.MethodPost {
				return httpJSON(404, map[string]any{"success": false, "error": "not found"})
			}
			refreshCalls.Add(1)
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
				ExpiresAt:    time.Now().UTC().Add(10 * time.Minute),
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
}

func TestRequireAPI_DefinitiveRefreshRejectionInvalidatesStoredCredentials(t *testing.T) {
	var refreshCalls atomic.Int32

	authRefreshHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/auth/refresh" || r.Method != http.MethodPost {
				return httpJSON(404, map[string]any{"success": false, "error": "not found"})
			}
			refreshCalls.Add(1)
			return httpJSON(401, map[string]any{"success": false, "error": "refresh failed"})
		}),
	}
	t.Cleanup(func() { authRefreshHTTPClient = nil })

	baseURL := "https://example.test"
	storePath := filepath.Join(t.TempDir(), "auth.json")
	st := &authstore.Store{
		Tokens: map[string]authstore.Record{
			baseURL: {
				Token:        "expired-token",
				RefreshToken: "revoked-refresh-token",
				ExpiresAt:    time.Now().UTC().Add(-time.Minute),
				UpdatedAt:    time.Now().UTC(),
			},
		},
	}
	if err := authstore.SaveAtomic(storePath, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	for invocation := 1; invocation <= 2; invocation++ {
		app := &App{APIURL: baseURL}
		if err := requireAPI(app); err == nil {
			t.Fatalf("invocation %d: expected rejected credentials to fail locally", invocation)
		}
	}

	if refreshCalls.Load() != 1 {
		t.Fatalf("expected only the first invocation to call refresh, got %d", refreshCalls.Load())
	}
	loaded, err := authstore.Load(storePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := loaded.GetRecord(baseURL); ok {
		t.Fatal("expected definitively rejected credential to be removed")
	}
}

func TestRequireAPI_DefinitiveRefreshRejectionPreservesRotatedCredentials(t *testing.T) {
	baseURL := "https://example.test"
	storePath := filepath.Join(t.TempDir(), "auth.json")
	rejected := authstore.Record{
		Token:        "expired-token",
		RefreshToken: "rejected-refresh-token",
		ExpiresAt:    time.Now().UTC().Add(-time.Minute),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := authstore.SaveAtomic(storePath, &authstore.Store{
		Tokens: map[string]authstore.Record{baseURL: rejected},
	}); err != nil {
		t.Fatalf("SaveAtomic rejected credentials: %v", err)
	}
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	rotated := authstore.Record{
		Token:        "fresh-token",
		RefreshToken: "fresh-refresh-token",
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
		UpdatedAt:    time.Now().UTC(),
	}
	authRefreshHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			if err := authstore.SaveAtomic(storePath, &authstore.Store{
				Tokens: map[string]authstore.Record{baseURL: rotated},
			}); err != nil {
				t.Fatalf("SaveAtomic rotated credentials: %v", err)
			}
			return httpJSON(401, map[string]any{"success": false, "error": "refresh failed"})
		}),
	}
	t.Cleanup(func() { authRefreshHTTPClient = nil })

	app := &App{APIURL: baseURL}
	if err := requireAPI(app); err != nil {
		t.Fatalf("expected concurrently rotated credentials to remain usable: %v", err)
	}
	if app.Token != rotated.Token {
		t.Fatalf("expected rotated token %q, got %q", rotated.Token, app.Token)
	}

	loaded, err := authstore.Load(storePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rec, ok := loaded.GetRecord(baseURL)
	if !ok {
		t.Fatal("expected rotated credentials to remain stored")
	}
	if rec.Token != rotated.Token || rec.RefreshToken != rotated.RefreshToken {
		t.Fatalf("expected rotated credentials, got token=%q refresh=%q", rec.Token, rec.RefreshToken)
	}
}

func TestRequireAPI_TransientRefreshFailureKeepsStoredCredentials(t *testing.T) {
	tests := []struct {
		name      string
		transport roundTripperFunc
	}{
		{
			name: "service unavailable",
			transport: func(*http.Request) (*http.Response, error) {
				return httpJSON(503, map[string]any{"success": false, "error": "temporarily unavailable"})
			},
		},
		{
			name: "transport failure",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection reset")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var refreshCalls atomic.Int32
			authRefreshHTTPClient = &http.Client{
				Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					refreshCalls.Add(1)
					return tt.transport(r)
				}),
			}
			t.Cleanup(func() { authRefreshHTTPClient = nil })

			baseURL := "https://example.test"
			storePath := filepath.Join(t.TempDir(), "auth.json")
			st := &authstore.Store{
				Tokens: map[string]authstore.Record{
					baseURL: {
						Token:        "still-valid-token",
						RefreshToken: "refresh-later",
						ExpiresAt:    time.Now().UTC().Add(10 * time.Minute),
						UpdatedAt:    time.Now().UTC(),
					},
				},
			}
			if err := authstore.SaveAtomic(storePath, st); err != nil {
				t.Fatalf("SaveAtomic: %v", err)
			}
			t.Setenv("BREYTA_AUTH_STORE", storePath)

			for invocation := 1; invocation <= 2; invocation++ {
				app := &App{APIURL: baseURL}
				if err := requireAPI(app); err != nil {
					t.Fatalf("invocation %d: expected existing token to remain usable: %v", invocation, err)
				}
				if app.Token != "still-valid-token" {
					t.Fatalf("invocation %d: expected existing token, got %q", invocation, app.Token)
				}
			}

			if refreshCalls.Load() != 2 {
				t.Fatalf("expected transient failure to remain retryable, got %d calls", refreshCalls.Load())
			}
			loaded, err := authstore.Load(storePath)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if _, ok := loaded.GetRecord(baseURL); !ok {
				t.Fatal("expected transient failure to retain stored credentials")
			}
		})
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

func TestRequireAPI_StillRequiresTokenForNonLoopbackAPI(t *testing.T) {
	app := &App{APIURL: "https://flows.breyta.ai", TokenExplicit: true}
	err := requireAPI(app)
	if err == nil {
		t.Fatalf("expected missing-token error")
	}
	if !strings.Contains(err.Error(), "missing token") {
		t.Fatalf("expected missing-token error, got %v", err)
	}
}

func TestRunFailureShouldUseDraftBindingsIncludesRunStep(t *testing.T) {
	out := map[string]any{
		"ok": false,
		"error": map[string]any{
			"code": "profile_bindings_incomplete",
		},
	}
	if !runFailureShouldUseDraftBindings("flows.run_step", map[string]any{
		"flowSlug": "draft-flow",
		"stepId":   "draft-platform-posts",
		"target":   "draft",
	}, out) {
		t.Fatal("expected draft run-step binding failures to use draft binding guidance")
	}
	if runFailureShouldUseDraftBindings("flows.run_step", map[string]any{
		"flowSlug":  "live-flow",
		"stepId":    "draft-platform-posts",
		"profileId": "prof-live",
	}, out) {
		t.Fatal("did not expect profile-targeted run-step failures to use draft binding guidance")
	}
}

func TestPublicAppWebURL_UsesActiveAPIEnvironment(t *testing.T) {
	cases := []struct {
		name string
		app  *App
		want string
	}{
		{
			name: "production marketing host",
			app:  &App{APIURL: "https://flows.breyta.ai"},
			want: "https://breyta.ai/apps/my-flow",
		},
		{
			name: "custom API origin",
			app:  &App{APIURL: "https://flows-staging.example.test/api"},
			want: "https://flows-staging.example.test/apps/my-flow",
		},
		{
			name: "local API origin",
			app:  &App{APIURL: "http://127.0.0.1:30639"},
			want: "http://127.0.0.1:30639/apps/my-flow",
		},
		{
			name: "https localhost API origin",
			app:  &App{APIURL: "https://localhost:30639/api?debug=true"},
			want: "http://localhost:30639/apps/my-flow",
		},
		{
			name: "https loopback API origin",
			app:  &App{APIURL: "https://127.0.0.1:30639/api"},
			want: "http://127.0.0.1:30639/apps/my-flow",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := publicAppWebURL(tc.app, "my-flow"); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestRefreshTokenViaAPI_ToleratesSnakeCase(t *testing.T) {
	var gotRefreshToken string
	var gotRefreshTokenSnake string

	authRefreshHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/auth/refresh" || r.Method != http.MethodPost {
				return httpJSON(404, map[string]any{"success": false, "error": "not found"})
			}
			payloadBytes, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(payloadBytes, &payload)
			if v, _ := payload["refreshToken"].(string); strings.TrimSpace(v) != "" {
				gotRefreshToken = strings.TrimSpace(v)
			}
			if v, _ := payload["refresh_token"].(string); strings.TrimSpace(v) != "" {
				gotRefreshTokenSnake = strings.TrimSpace(v)
			}
			return httpJSON(200, map[string]any{
				"success":       true,
				"token":         "tok-2",
				"refresh_token": "ref-2",
				"expires_in":    "3600",
			})
		}),
	}
	t.Cleanup(func() { authRefreshHTTPClient = nil })

	rec, err := refreshTokenViaAPI("https://example.test", "ref-1")
	if err != nil {
		t.Fatalf("refreshTokenViaAPI: %v", err)
	}
	if rec.Token != "tok-2" || rec.RefreshToken != "ref-2" {
		t.Fatalf("expected tok-2/ref-2, got token=%q refresh=%q", rec.Token, rec.RefreshToken)
	}
	if rec.ExpiresAt.IsZero() || time.Until(rec.ExpiresAt) < 59*time.Minute {
		t.Fatalf("expected ExpiresAt ~1h in future, got %v", rec.ExpiresAt)
	}
	if gotRefreshToken != "ref-1" || gotRefreshTokenSnake != "ref-1" {
		t.Fatalf("expected refresh token sent in both fields, got refreshToken=%q refresh_token=%q", gotRefreshToken, gotRefreshTokenSnake)
	}
}

func TestEnsureErrorRecoveryActions_NoWorkspaceSkipsSynthesizedRunURLs(t *testing.T) {
	t.Parallel()

	out := map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    "profile_missing",
			"message": "Flow requires a profile before running.",
			"details": map[string]any{
				"flowSlug": "demo-flow",
			},
		},
	}

	actions := ensureErrorRecoveryActions(&App{
		APIURL: "https://flows.breyta.ai",
	}, out)

	if len(actions) != 0 {
		t.Fatalf("did not expect synthesized actions without workspace, got %#v", actions)
	}

	errMap := mapStringAny(out["error"])
	if got := sliceAny(errMap["actions"]); len(got) != 0 {
		t.Fatalf("did not expect serialized actions without workspace, got %#v", got)
	}

	meta := mapStringAny(out["meta"])
	if meta != nil && !reflect.DeepEqual(meta, map[string]any{}) {
		t.Fatalf("did not expect metadata side effects without workspace, got %#v", meta)
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func waitRunNextCommands(t *testing.T, out map[string]any) []string {
	t.Helper()
	meta := mapStringAny(out["meta"])
	var cmds []string
	for _, raw := range sliceAny(meta["nextCommands"]) {
		if s := firstNonBlankString(raw); s != "" {
			cmds = append(cmds, s)
		}
	}
	return cmds
}

func TestAddWaitRunNextCommands_InstallationScopedHintIsRunnable(t *testing.T) {
	t.Run("explicit installation id is threaded into the inspect hint", func(t *testing.T) {
		out := map[string]any{"data": map[string]any{"run": map[string]any{"status": "completed"}}}
		addWaitRunNextCommands(out, "wf-linked", "prof-consumer")
		cmds := waitRunNextCommands(t, out)
		if !containsString(cmds, "breyta runs inspect wf-linked --installation-id prof-consumer") {
			t.Fatalf("expected installation-scoped inspect hint, got %#v", cmds)
		}
	})

	t.Run("installation id falls back to the run snapshot", func(t *testing.T) {
		out := map[string]any{"data": map[string]any{"run": map[string]any{
			"status":         "completed",
			"installationId": "prof-consumer",
		}}}
		addWaitRunNextCommands(out, "wf-linked", "")
		cmds := waitRunNextCommands(t, out)
		if !containsString(cmds, "breyta runs inspect wf-linked --installation-id prof-consumer") {
			t.Fatalf("expected inspect hint to use snapshot installation id, got %#v", cmds)
		}
	})

	t.Run("workspace-owned runs keep the plain inspect hint", func(t *testing.T) {
		out := map[string]any{"data": map[string]any{"run": map[string]any{"status": "completed"}}}
		addWaitRunNextCommands(out, "wf-local", "")
		cmds := waitRunNextCommands(t, out)
		if !containsString(cmds, "breyta runs inspect wf-local") {
			t.Fatalf("expected plain inspect hint, got %#v", cmds)
		}
		for _, c := range cmds {
			if strings.Contains(c, "--installation-id") {
				t.Fatalf("did not expect an installation-id flag for a workspace run, got %#v", cmds)
			}
		}
	})
}
