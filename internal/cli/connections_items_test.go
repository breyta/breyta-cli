package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestConnectionsItems_APIModeListsCachedConnectionItems(t *testing.T) {
	var sawPath string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		if r.Method != http.MethodGet || r.URL.Path != "/api/connections/conn-gh/items" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("item-type") != "github/repository" || r.URL.Query().Get("limit") != "1" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing query"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{
					"full-name":   "acme/api",
					"name":        "api",
					"description": "Main API",
				},
			},
			"summary": map[string]any{
				"itemType": "github/repository",
				"count":    2,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "items", "conn-gh",
		"--item-type", "github/repository",
		"--limit", "1",
	)
	if err != nil {
		t.Fatalf("connections items failed: %v\n%s", err, stdout)
	}
	if sawPath != "/api/connections/conn-gh/items" {
		t.Fatalf("expected connection get path, got %q", sawPath)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	summary, _ := data["summary"].(map[string]any)
	if summary["items"] != float64(2) || summary["returned"] != float64(1) || summary["filteredItemType"] != "github/repository" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	itemTypes, _ := data["itemTypes"].([]any)
	if len(itemTypes) != 1 {
		t.Fatalf("expected one item type, got %#v", itemTypes)
	}
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one returned item, got %#v", items)
	}
	first, _ := items[0].(map[string]any)
	if first["value"] != "acme/api" || first["label"] != "api" || first["description"] != "Main API" {
		t.Fatalf("unexpected item summary: %#v", first)
	}
	if _, ok := first["raw"]; ok {
		t.Fatalf("raw payload should be omitted unless --raw is set: %#v", first)
	}
}

func TestConnectionsItems_RejectsNegativeLimit(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t,
		"connections", "items", "conn-gh",
		"--limit", "-1",
	)
	if err == nil {
		t.Fatalf("expected negative limit to fail:\n%s", stdout)
	}
	if !strings.Contains(stdout+stderr+err.Error(), "--limit must be 0 or greater") {
		t.Fatalf("expected negative limit error, got stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
}

func TestConnectionsItems_APIModeRawIncludesCachedPayload(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/connections/conn-gh/items" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{"full-name": "acme/api", "name": "api"},
			},
			"summary": map[string]any{"itemType": "github/repository", "count": 1},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "items", "conn-gh",
		"--item-type", "github/repository",
		"--raw",
	)
	if err != nil {
		t.Fatalf("connections items failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	items, _ := data["items"].([]any)
	first, _ := items[0].(map[string]any)
	raw, _ := first["raw"].(map[string]any)
	if raw["full-name"] != "acme/api" {
		t.Fatalf("expected raw item payload, got %#v", first)
	}
}

func TestConnectionsItems_APIModeLimitZeroPaginatesAllItems(t *testing.T) {
	var cursors []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/connections/conn-gh/items" {
			http.NotFound(w, r)
			return
		}
		cursors = append(cursors, r.URL.Query().Get("cursor"))
		if r.URL.Query().Get("item-type") != "github/repository" || r.URL.Query().Get("limit") != "500" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing query"}})
			return
		}
		if r.URL.Query().Get("cursor") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{"full-name": "acme/api"},
				},
				"next-cursor": "1",
				"has-more":    true,
				"summary":     map[string]any{"itemType": "github/repository", "count": 2},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{"full-name": "acme/web"},
			},
			"has-more": false,
			"summary":  map[string]any{"itemType": "github/repository", "count": 2},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "items", "conn-gh",
		"--item-type", "github/repository",
		"--limit", "0",
	)
	if err != nil {
		t.Fatalf("connections items failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected both paginated items, got %#v", items)
	}
	if strings.Join(cursors, ",") != ",1" {
		t.Fatalf("unexpected cursors: %#v", cursors)
	}
}

func TestConnectionsItems_APIModeLimitZeroDoesNotStopAtOneHundredPages(t *testing.T) {
	var pageCount int
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/connections/conn-gh/items" {
			http.NotFound(w, r)
			return
		}
		cursor := r.URL.Query().Get("cursor")
		page := 0
		if cursor != "" {
			if _, err := fmt.Sscanf(cursor, "%d", &page); err != nil {
				t.Fatalf("unexpected cursor %q", cursor)
			}
		}
		pageCount++
		hasMore := page < 100
		resp := map[string]any{
			"items": []any{
				map[string]any{"full-name": fmt.Sprintf("acme/repo-%03d", page)},
			},
			"has-more": hasMore,
			"summary":  map[string]any{"itemType": "github/repository", "count": 101},
		}
		if hasMore {
			resp["next-cursor"] = fmt.Sprintf("%d", page+1)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "items", "conn-gh",
		"--item-type", "github/repository",
		"--limit", "0",
	)
	if err != nil {
		t.Fatalf("connections items failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 101 || pageCount != 101 {
		t.Fatalf("expected all 101 pages/items, got pages=%d items=%d", pageCount, len(items))
	}
}

func TestConnectionsItems_APIModeReadsTopLevelKebabConnectionItems(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/connections/conn-projects/items" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{"itemType": "project", "count": 1},
			},
			"summary": map[string]any{"itemTypes": 1},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "items", "conn-projects",
	)
	if err != nil {
		t.Fatalf("connections items failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, `"itemType":"project"`) || !strings.Contains(stdout, `"itemTypes":1`) {
		t.Fatalf("expected connection item type output, got:\n%s", stdout)
	}
}
