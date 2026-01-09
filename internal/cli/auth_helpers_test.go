package cli

import "testing"

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
