package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type liveProfileTarget struct {
	ProfileID string
	Version   int
	UpdatedAt time.Time
}

func resolveLiveProfileTarget(app *App, flowSlug string) (*liveProfileTarget, error) {
	slug := strings.TrimSpace(flowSlug)
	if slug == "" {
		return nil, fmt.Errorf("missing flow slug")
	}

	client := apiClient(app)
	q := url.Values{}
	q.Set("flow-slug", slug)
	q.Set("profile-type", "prod")
	q.Set("enabled", "true")
	q.Set("limit", "200")

	outAny, status, err := client.DoREST(context.Background(), http.MethodGet, "/api/flow-profiles", q, nil)
	if err != nil {
		return nil, err
	}
	out, _ := outAny.(map[string]any)
	if status >= 400 {
		return nil, fmt.Errorf("resolve live scope: %s", formatAPIError(out))
	}
	if out == nil {
		return nil, fmt.Errorf("resolve live scope: invalid profile list response")
	}

	items := profileItemsFromListResponse(out)
	candidates := make([]liveProfileTarget, 0, len(items))
	for _, item := range items {
		if profileInstallScope(item) != "live" {
			continue
		}
		profileID := strings.TrimSpace(anyString(item["profile-id"], item["profileId"], item["id"]))
		if profileID == "" {
			continue
		}
		version := anyInt(item["version"])
		if version <= 0 {
			continue
		}
		candidates = append(candidates, liveProfileTarget{
			ProfileID: profileID,
			Version:   version,
			UpdatedAt: parseUpdatedAt(item["updated-at"], item["updatedAt"]),
		})
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("live scope is not configured for %s (run `breyta flows install promote %s --scope live` or `breyta flows configure %s --scope live --set <slot>.conn=...`)", slug, slug, slug)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].UpdatedAt.Equal(candidates[j].UpdatedAt) {
			return candidates[i].ProfileID > candidates[j].ProfileID
		}
		return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt)
	})
	target := candidates[0]
	return &target, nil
}

func profileItemsFromListResponse(out map[string]any) []map[string]any {
	itemsAny, _ := out["items"].([]any)
	if itemsAny == nil {
		if data, _ := out["data"].(map[string]any); data != nil {
			itemsAny, _ = data["items"].([]any)
		}
	}
	items := make([]map[string]any, 0, len(itemsAny))
	for _, it := range itemsAny {
		if m, _ := it.(map[string]any); m != nil {
			items = append(items, m)
		}
	}
	return items
}

func profileInstallScope(item map[string]any) string {
	config, _ := item["config"].(map[string]any)
	rawScope := anyString(
		valueFromMap(config, "install-scope"),
		valueFromMap(config, "installScope"),
		valueFromMap(config, "scope"),
	)
	scope := strings.ToLower(strings.TrimSpace(rawScope))
	switch scope {
	case "live":
		return "live"
	case "end-user":
		return "end-user"
	}

	owner := strings.TrimSpace(anyString(item["user-id"], item["userId"]))
	if owner != "" {
		return "end-user"
	}
	return "live"
}

func parseUpdatedAt(values ...any) time.Time {
	raw := strings.TrimSpace(anyString(values...))
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func anyString(values ...any) string {
	for _, value := range values {
		switch t := value.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return t
			}
		case fmt.Stringer:
			s := strings.TrimSpace(t.String())
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func anyInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil {
			return n
		}
	}
	return 0
}

func valueFromMap(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}
