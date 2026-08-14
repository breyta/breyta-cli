package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseExpiresInSeconds(t *testing.T) {
	n, err := parseExpiresInSeconds(" 3600 ")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if n != 3600 {
		t.Fatalf("expected 3600, got %d", n)
	}

	if _, err := parseExpiresInSeconds(""); err == nil {
		t.Fatalf("expected error for empty value")
	}
	if _, err := parseExpiresInSeconds("nope"); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestShellExportTokenLine(t *testing.T) {
	if got := shellExportTokenLine("abc"); got != "export BREYTA_TOKEN='abc'" {
		t.Fatalf("unexpected export line: %q", got)
	}
	if got := shellExportTokenLine("a'b"); got != "" {
		t.Fatalf("expected empty string for unsafe token, got %q", got)
	}
}

func TestWhoamiVerifyRejectsUnauthorizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid token"}}`))
	}))
	defer server.Close()

	_, status, _, err := whoamiVerify(context.Background(), &App{APIURL: server.URL, Token: "invalid"})
	if err == nil {
		t.Fatal("expected unauthorized identity response to fail")
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", status)
	}
}
