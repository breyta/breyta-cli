package authinfo

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

func tokenClaims(token string) map[string]any {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}

	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil
	}

	return payload
}

// EmailFromToken extracts an email address from a JWT-like token payload.
// It does not validate signatures; it is used only for local UI display.
func EmailFromToken(token string) string {
	payload := tokenClaims(token)
	if payload == nil {
		return ""
	}

	email, _ := payload["email"].(string)
	return strings.TrimSpace(email)
}

// UIDFromToken extracts the stable authenticated user ID from a JWT-like token payload.
func UIDFromToken(token string) string {
	payload := tokenClaims(token)
	if payload == nil {
		return ""
	}

	if userID, _ := payload["user_id"].(string); strings.TrimSpace(userID) != "" {
		return strings.TrimSpace(userID)
	}
	if sub, _ := payload["sub"].(string); strings.TrimSpace(sub) != "" {
		return strings.TrimSpace(sub)
	}
	return ""
}

// NameFromToken extracts a display name from a JWT-like token payload.
func NameFromToken(token string) string {
	payload := tokenClaims(token)
	if payload == nil {
		return ""
	}

	name, _ := payload["name"].(string)
	return strings.TrimSpace(name)
}
