package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/breyta/breyta-cli/internal/authstore"
)

func newAPIConnectionTestServer(t *testing.T, got *map[string]any) *httptest.Server {
	t.Helper()
	return newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/runtime-connection" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(got)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"secretId": "breyta-api-auth",
			"connection": map[string]any{
				"connection-id": "conn-123",
				"name":          "Breyta API",
			},
		})
	}))
}

func seedAPIConnectionAuthStore(t *testing.T, apiURL string, rec authstore.Record) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	storePath := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	st := &authstore.Store{}
	st.SetRecord(apiURL, rec)
	if err := authstore.SaveAtomic(storePath, st); err != nil {
		t.Fatalf("save auth store: %v", err)
	}
}

func fullAPIConnectionRecord() authstore.Record {
	return authstore.Record{
		Token:        "id-token-123",
		RefreshToken: "refresh-token-123",
		ExpiresAt:    time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC),
	}
}

func TestAuthAPIConnection_DefaultSendsServiceAccountMode(t *testing.T) {
	var got map[string]any
	srv := newAPIConnectionTestServer(t, &got)
	defer srv.Close()
	seedAPIConnectionAuthStore(t, srv.URL, fullAPIConnectionRecord())

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"auth", "api-connection",
	)
	if err != nil {
		t.Fatalf("auth api-connection failed: %v\n%s", err, stdout)
	}
	if got["authMode"] != "service-account" {
		t.Fatalf("expected authMode=service-account in request, got %#v", got)
	}
	if _, ok := got["force"]; ok {
		t.Fatalf("expected no force field in default mode, got %#v", got)
	}
	if _, ok := got["new"]; ok {
		t.Fatalf("expected no new field without --new, got %#v", got)
	}
	if got["refreshToken"] != "refresh-token-123" {
		t.Fatalf("expected refreshToken still sent, got %#v", got)
	}
}

func TestAuthAPIConnection_DefaultAllowsMissingRefreshToken(t *testing.T) {
	var got map[string]any
	srv := newAPIConnectionTestServer(t, &got)
	defer srv.Close()
	seedAPIConnectionAuthStore(t, srv.URL, authstore.Record{Token: "id-token-123"})

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"auth", "api-connection",
	)
	if err != nil {
		t.Fatalf("auth api-connection failed: %v\n%s", err, stdout)
	}
	if got["authMode"] != "service-account" {
		t.Fatalf("expected authMode=service-account in request, got %#v", got)
	}
}

func TestAuthAPIConnection_OAuthSendsLegacyModeWithForce(t *testing.T) {
	var got map[string]any
	srv := newAPIConnectionTestServer(t, &got)
	defer srv.Close()
	seedAPIConnectionAuthStore(t, srv.URL, fullAPIConnectionRecord())

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"auth", "api-connection",
		"--oauth",
	)
	if err != nil {
		t.Fatalf("auth api-connection failed: %v\n%s", err, stdout)
	}
	if _, ok := got["authMode"]; ok {
		t.Fatalf("expected no authMode field in --oauth mode, got %#v", got)
	}
	if got["force"] != true {
		t.Fatalf("expected force=true in --oauth mode, got %#v", got)
	}
	if got["refreshToken"] != "refresh-token-123" {
		t.Fatalf("expected refreshToken in request, got %#v", got)
	}
}

func TestAuthAPIConnection_OAuthRequiresRefreshToken(t *testing.T) {
	var got map[string]any
	srv := newAPIConnectionTestServer(t, &got)
	defer srv.Close()
	seedAPIConnectionAuthStore(t, srv.URL, authstore.Record{Token: "id-token-123"})

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"auth", "api-connection",
		"--oauth",
	)
	if err == nil {
		t.Fatalf("expected error for --oauth without refresh token\n%s", stdout)
	}
	if !strings.Contains(err.Error(), "refresh token") {
		t.Fatalf("expected refresh token error, got %v\n%s", err, stderr)
	}
	if got != nil {
		t.Fatalf("expected no request to be sent, got %#v", got)
	}
}

func TestAuthAPIConnection_CapabilitiesSentAsArray(t *testing.T) {
	var got map[string]any
	srv := newAPIConnectionTestServer(t, &got)
	defer srv.Close()
	seedAPIConnectionAuthStore(t, srv.URL, fullAPIConnectionRecord())

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"auth", "api-connection",
		"--capabilities", "resources.read, flows.run,",
	)
	if err != nil {
		t.Fatalf("auth api-connection failed: %v\n%s", err, stdout)
	}
	want := []any{"resources.read", "flows.run"}
	if !reflect.DeepEqual(got["capabilities"], want) {
		t.Fatalf("expected capabilities %v, got %#v", want, got["capabilities"])
	}
	if got["authMode"] != "service-account" {
		t.Fatalf("expected authMode=service-account in request, got %#v", got)
	}
}

func TestAuthAPIConnection_NewSendsNewTrue(t *testing.T) {
	var got map[string]any
	srv := newAPIConnectionTestServer(t, &got)
	defer srv.Close()
	seedAPIConnectionAuthStore(t, srv.URL, fullAPIConnectionRecord())

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"auth", "api-connection",
		"--new",
	)
	if err != nil {
		t.Fatalf("auth api-connection failed: %v\n%s", err, stdout)
	}
	if got["new"] != true {
		t.Fatalf("expected new=true in request, got %#v", got)
	}
}

func TestAuthAPIConnection_CapabilitiesWithOAuthErrors(t *testing.T) {
	var got map[string]any
	srv := newAPIConnectionTestServer(t, &got)
	defer srv.Close()
	seedAPIConnectionAuthStore(t, srv.URL, fullAPIConnectionRecord())

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-acme",
		"auth", "api-connection",
		"--oauth",
		"--capabilities", "resources.read",
	)
	if err == nil {
		t.Fatalf("expected error for --capabilities with --oauth\n%s", stdout)
	}
	if !strings.Contains(err.Error(), "--capabilities requires service-account mode") {
		t.Fatalf("expected capabilities mode error, got %v\n%s", err, stderr)
	}
	if got != nil {
		t.Fatalf("expected no request to be sent, got %#v", got)
	}
}
