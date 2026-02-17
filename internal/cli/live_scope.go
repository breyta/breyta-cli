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
	Enabled   bool
}

func resolveLiveProfileTarget(ctx context.Context, app *App, flowSlug string, includeDisabled bool) (*liveProfileTarget, error) {
	slug := strings.TrimSpace(flowSlug)
	if slug == "" {
		return nil, fmt.Errorf("missing flow slug")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	client := apiClient(app)
	candidates := make([]liveProfileTarget, 0, 16)
	cursor := ""
	seenCursors := map[string]bool{}

	for {
		q := url.Values{}
		q.Set("flow-slug", slug)
		q.Set("profile-type", "prod")
		if !includeDisabled {
			q.Set("enabled", "true")
		}
		q.Set("limit", "200")
		if strings.TrimSpace(cursor) != "" {
			q.Set("cursor", cursor)
		}

		outAny, status, err := client.DoREST(ctx, http.MethodGet, "/api/flow-profiles", q, nil)
		if err != nil {
			return nil, err
		}
		out, _ := outAny.(map[string]any)
		if status >= 400 {
			return nil, fmt.Errorf("resolve live target: %s", formatAPIError(out))
		}
		if out == nil {
			return nil, fmt.Errorf("resolve live target: invalid profile list response")
		}

		items := profileItemsFromListResponse(out)
		for _, item := range items {
			if profileInstallTarget(item) != "live" {
				continue
			}
			profileID := strings.TrimSpace(anyString(item["profile-id"], item["profileId"], item["id"]))
			if profileID == "" {
				continue
			}
			version := anyInt(item["version"])
			enabled := anyBoolWithDefault(item["enabled"], true)
			if !includeDisabled && !enabled {
				continue
			}
			candidates = append(candidates, liveProfileTarget{
				ProfileID: profileID,
				Version:   version,
				UpdatedAt: parseUpdatedAt(item["updated-at"], item["updatedAt"]),
				Enabled:   enabled,
			})
		}

		hasMore, nextCursor := profileListPagination(out)
		if !hasMore || strings.TrimSpace(nextCursor) == "" {
			break
		}
		if seenCursors[nextCursor] {
			break
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("live target is not configured for %s (run `breyta flows promote %s` or `breyta flows configure %s --target live --set <slot>.conn=...`)", slug, slug, slug)
	}

	if includeDisabled {
		enabledCandidates := make([]liveProfileTarget, 0, len(candidates))
		for _, c := range candidates {
			if c.Enabled {
				enabledCandidates = append(enabledCandidates, c)
			}
		}
		if len(enabledCandidates) > 0 {
			candidates = enabledCandidates
		}
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

func profileInstallTarget(item map[string]any) string {
	config, _ := item["config"].(map[string]any)
	rawTarget := anyString(
		valueFromMap(config, "install-scope"),
		valueFromMap(config, "installScope"),
	)
	target := strings.ToLower(strings.TrimSpace(rawTarget))
	switch target {
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

func anyBoolWithDefault(v any, defaultValue bool) bool {
	switch t := v.(type) {
	case nil:
		return defaultValue
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "y", "on":
			return true
		case "false", "0", "no", "n", "off":
			return false
		default:
			return defaultValue
		}
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	default:
		return defaultValue
	}
}

func valueFromMap(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}

func profileListPagination(out map[string]any) (bool, string) {
	if out == nil {
		return false, ""
	}
	meta, _ := out["meta"].(map[string]any)
	data, _ := out["data"].(map[string]any)
	return anyBool(meta["hasMore"], out["has-more"], data["has-more"]),
		strings.TrimSpace(anyString(meta["nextCursor"], out["next-cursor"], data["next-cursor"]))
}

func anyBool(values ...any) bool {
	for _, value := range values {
		switch t := value.(type) {
		case bool:
			return t
		case string:
			switch strings.ToLower(strings.TrimSpace(t)) {
			case "true", "1", "yes", "y", "on":
				return true
			case "false", "0", "no", "n", "off":
				return false
			}
		case float64:
			return t != 0
		case int:
			return t != 0
		case int64:
			return t != 0
		}
	}
	return false
}
