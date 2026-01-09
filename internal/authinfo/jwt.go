package authinfo

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// EmailFromToken extracts an email address from a JWT-like token payload.
// It does not validate signatures; it is used only for local UI display.
func EmailFromToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}

	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return ""
	}

	email, _ := payload["email"].(string)
	return strings.TrimSpace(email)
}
