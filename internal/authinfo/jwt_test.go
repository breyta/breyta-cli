package authinfo

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func jwtToken(t *testing.T, payload map[string]any) string {
	t.Helper()

	header := map[string]any{"alg": "none", "typ": "JWT"}
	hb, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	pb, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	h := base64.RawURLEncoding.EncodeToString(hb)
	p := base64.RawURLEncoding.EncodeToString(pb)
	return h + "." + p + "."
}

func TestEmailFromToken(t *testing.T) {
	tok := jwtToken(t, map[string]any{"email": "a@b.com"})
	if got := EmailFromToken(tok); got != "a@b.com" {
		t.Fatalf("unexpected email: %q", got)
	}
}

func TestEmailFromToken_NonJWT(t *testing.T) {
	if got := EmailFromToken("dev-user-123"); got != "" {
		t.Fatalf("expected empty email, got %q", got)
	}
}
