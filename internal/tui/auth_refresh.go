package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
	"github.com/breyta/breyta-cli/internal/authstore"
	"github.com/breyta/breyta-cli/internal/configstore"
)

func authStorePath() (string, error) {
	if p := strings.TrimSpace(devAuthStorePath()); p != "" {
		return p, nil
	}
	if p := strings.TrimSpace(os.Getenv("BREYTA_AUTH_STORE")); p != "" {
		return p, nil
	}
	return authstore.DefaultPath()
}

func devAuthStorePath() string {
	p, err := configstore.DefaultPath()
	if err != nil || strings.TrimSpace(p) == "" {
		return ""
	}
	st, err := configstore.Load(p)
	if err != nil || st == nil || !st.DevMode || len(st.DevProfiles) == 0 {
		return ""
	}
	active := strings.TrimSpace(st.DevActive)
	if active == "" {
		if _, ok := st.DevProfiles["local"]; ok {
			active = "local"
		}
	}
	if active == "" {
		for name := range st.DevProfiles {
			active = name
			break
		}
	}
	if active == "" {
		return ""
	}
	prof, ok := st.DevProfiles[active]
	if !ok {
		return ""
	}
	return strings.TrimSpace(prof.AuthStorePath)
}

func parseJWTExpiry(token string) (time.Time, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return time.Time{}, false
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	expAny, ok := claims["exp"]
	if !ok {
		return time.Time{}, false
	}
	var expSeconds int64
	switch v := expAny.(type) {
	case float64:
		expSeconds = int64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			expSeconds = n
		}
	case int64:
		expSeconds = v
	case int:
		expSeconds = int64(v)
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			expSeconds = n
		}
	}
	if expSeconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(expSeconds, 0).UTC(), true
}

func parseExpiresInSeconds(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, errors.New("missing expiresIn")
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func storeAuthRecord(apiBaseURL, token, refreshToken, expiresIn string) error {
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	token = strings.TrimSpace(token)
	refreshToken = strings.TrimSpace(refreshToken)
	expiresIn = strings.TrimSpace(expiresIn)
	if apiBaseURL == "" {
		return errors.New("missing api base url")
	}
	if token == "" {
		return errors.New("missing token")
	}

	p, err := authStorePath()
	if err != nil {
		return err
	}
	st, _ := authstore.Load(p)
	if st == nil {
		st = &authstore.Store{}
	}

	rec := authstore.Record{
		Token:        token,
		RefreshToken: refreshToken,
	}
	if refreshToken != "" {
		if n, err := parseExpiresInSeconds(expiresIn); err == nil && n > 0 {
			rec.ExpiresAt = time.Now().UTC().Add(time.Duration(n) * time.Second)
		} else if exp, ok := parseJWTExpiry(token); ok {
			rec.ExpiresAt = exp
		}
	}
	st.SetRecord(apiBaseURL, rec)
	return authstore.SaveAtomic(p, st)
}

func refreshTokenViaAPI(apiBaseURL string, refreshToken string) (authstore.Record, error) {
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	refreshToken = strings.TrimSpace(refreshToken)
	if apiBaseURL == "" {
		return authstore.Record{}, errors.New("missing api base url")
	}
	if refreshToken == "" {
		return authstore.Record{}, errors.New("missing refresh token")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client := api.Client{BaseURL: apiBaseURL}
	out, status, err := client.DoRootREST(ctx, http.MethodPost, "/api/auth/refresh", nil, map[string]any{
		"refreshToken":  refreshToken,
		"refresh_token": refreshToken,
	})
	if err != nil {
		return authstore.Record{}, err
	}
	if status < 200 || status > 299 {
		return authstore.Record{}, fmt.Errorf("refresh failed (status=%d)", status)
	}

	m, ok := out.(map[string]any)
	if !ok {
		return authstore.Record{}, fmt.Errorf("refresh returned unexpected response (status=%d)", status)
	}
	if success, _ := m["success"].(bool); !success {
		msg, _ := m["error"].(string)
		if strings.TrimSpace(msg) == "" {
			msg = "refresh failed"
		}
		return authstore.Record{}, fmt.Errorf("%s (status=%d)", strings.TrimSpace(msg), status)
	}
	token, _ := m["token"].(string)
	if strings.TrimSpace(token) == "" {
		return authstore.Record{}, fmt.Errorf("refresh returned no token (status=%d)", status)
	}
	nextRefresh, _ := m["refreshToken"].(string)
	if strings.TrimSpace(nextRefresh) == "" {
		nextRefresh, _ = m["refresh_token"].(string)
	}
	if strings.TrimSpace(nextRefresh) == "" {
		nextRefresh = refreshToken
	}

	rec := authstore.Record{
		Token:        strings.TrimSpace(token),
		RefreshToken: strings.TrimSpace(nextRefresh),
	}

	expiresInAny := m["expiresIn"]
	if expiresInAny == nil {
		expiresInAny = m["expires_in"]
	}
	var expiresInSeconds int64
	switch v := expiresInAny.(type) {
	case string:
		if n, err := parseExpiresInSeconds(v); err == nil {
			expiresInSeconds = n
		}
	case float64:
		expiresInSeconds = int64(v)
	}
	if expiresInSeconds > 0 {
		rec.ExpiresAt = time.Now().UTC().Add(time.Duration(expiresInSeconds) * time.Second)
	}
	return rec, nil
}

func resolveTokenForAPI(apiBaseURL string, explicitToken string) (string, bool, error) {
	explicitToken = strings.TrimSpace(explicitToken)
	if explicitToken != "" {
		return explicitToken, false, nil
	}
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if apiBaseURL == "" {
		return "", false, nil
	}

	p, err := authStorePath()
	if err != nil {
		return "", false, err
	}
	st, err := authstore.Load(p)
	if err != nil || st == nil {
		return "", false, nil
	}
	rec, ok := st.GetRecord(apiBaseURL)
	if !ok {
		return "", false, nil
	}

	updated := false
	if rec.ExpiresAt.IsZero() {
		if exp, ok := parseJWTExpiry(rec.Token); ok {
			rec.ExpiresAt = exp
			updated = true
		}
	}
	if strings.TrimSpace(rec.RefreshToken) != "" && rec.ExpiresAt.IsZero() {
		next, err := refreshTokenViaAPI(apiBaseURL, rec.RefreshToken)
		if err != nil {
			return rec.Token, false, err
		}
		rec = next
		updated = true
	}
	if strings.TrimSpace(rec.RefreshToken) != "" && !rec.ExpiresAt.IsZero() && time.Until(rec.ExpiresAt) < 2*time.Minute {
		next, err := refreshTokenViaAPI(apiBaseURL, rec.RefreshToken)
		if err != nil {
			return rec.Token, false, err
		}
		rec = next
		updated = true
	}
	if updated {
		st.SetRecord(apiBaseURL, rec)
		_ = authstore.SaveAtomic(p, st)
	}
	return rec.Token, updated, nil
}
