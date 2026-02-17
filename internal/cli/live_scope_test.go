package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveLiveProfileTarget_AllowsUnpinnedLiveProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/flow-profiles" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"items": []any{
				map[string]any{
					"profileId": "prof-live-track",
					"enabled":   true,
					"version":   0,
					"updatedAt": "2026-02-17T10:00:00Z",
					"config": map[string]any{
						"installScope": "live",
					},
				},
			},
			"hasMore": false,
		})
	}))
	defer srv.Close()

	app := &App{
		APIURL:      srv.URL,
		WorkspaceID: "ws-acme",
		Token:       "dev-user-123",
	}

	target, err := resolveLiveProfileTarget(context.Background(), app, "flow-a", true)
	if err != nil {
		t.Fatalf("resolveLiveProfileTarget failed: %v", err)
	}
	if target == nil {
		t.Fatalf("expected live target")
	}
	if target.ProfileID != "prof-live-track" {
		t.Fatalf("unexpected profile id: %q", target.ProfileID)
	}
	if target.Version != 0 {
		t.Fatalf("expected unpinned version 0, got %d", target.Version)
	}
}

func TestResolveLiveProfileTarget_PaginatesWithTopLevelCamelCase(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/flow-profiles" {
			http.NotFound(w, r)
			return
		}
		calls++
		if calls == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"items": []any{
					map[string]any{
						"profileId": "prof-end-user",
						"enabled":   true,
						"version":   9,
						"updatedAt": "2026-02-17T10:00:00Z",
						"userId":    "u-1",
					},
				},
				"hasMore":    true,
				"nextCursor": "page-2",
			})
			return
		}
		if r.URL.Query().Get("cursor") != "page-2" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected cursor"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"items": []any{
				map[string]any{
					"profileId": "prof-live-track",
					"enabled":   true,
					"version":   0,
					"updatedAt": "2026-02-17T10:01:00Z",
					"config": map[string]any{
						"installScope": "live",
					},
				},
			},
			"hasMore": false,
		})
	}))
	defer srv.Close()

	app := &App{
		APIURL:      srv.URL,
		WorkspaceID: "ws-acme",
		Token:       "dev-user-123",
	}

	target, err := resolveLiveProfileTarget(context.Background(), app, "flow-a", true)
	if err != nil {
		t.Fatalf("resolveLiveProfileTarget failed: %v", err)
	}
	if target == nil {
		t.Fatalf("expected live target")
	}
	if target.ProfileID != "prof-live-track" {
		t.Fatalf("unexpected profile id: %q", target.ProfileID)
	}
	if target.Version != 0 {
		t.Fatalf("expected unpinned version 0, got %d", target.Version)
	}
	if calls != 2 {
		t.Fatalf("expected two pagination calls, got %d", calls)
	}
}
