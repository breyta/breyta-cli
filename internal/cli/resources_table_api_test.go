package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestResourcesRead_TablePreviewPassesLimitAndOffset(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/content" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("uri"); got != "res://v1/ws/ws-acme/result/table/tbl_1" {
			t.Fatalf("expected uri query param, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Fatalf("expected limit=50, got %q", got)
		}
		if got := r.URL.Query().Get("offset"); got != "200" {
			t.Fatalf("expected offset=200, got %q", got)
		}
		if got := r.URL.Query().Get("tablePartition"); got != "month-2026-03" {
			t.Fatalf("expected tablePartition=month-2026-03, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceUri": "res://v1/ws/ws-acme/result/table/tbl_1",
			"tableName":   "orders",
			"query": map[string]any{
				"limit":      50,
				"offset":     200,
				"hasMore":    true,
				"nextOffset": 250,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "read", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--limit", "50",
		"--offset", "200",
		"--partition-key", "month-2026-03",
	)
	if err != nil {
		t.Fatalf("resources read failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	query, _ := data["query"].(map[string]any)
	if got, _ := query["limit"].(float64); got != 50 {
		t.Fatalf("unexpected query.limit: %v", query["limit"])
	}
}

func TestResourcesTableVerify_ReadsMetadataAndPreview(t *testing.T) {
	var sawMetadata bool
	var sawPreview bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/resources/by-uri":
			sawMetadata = true
			if got := r.URL.Query().Get("uri"); got != "res://v1/ws/ws-acme/result/table/tbl_1" {
				t.Fatalf("expected metadata uri query param, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uri":         "res://v1/ws/ws-acme/result/table/tbl_1",
				"contentType": "application/vnd.breyta.table+json",
			})
		case "/api/resources/content":
			sawPreview = true
			if got := r.URL.Query().Get("uri"); got != "res://v1/ws/ws-acme/result/table/tbl_1" {
				t.Fatalf("expected preview uri query param, got %q", got)
			}
			if got := r.URL.Query().Get("limit"); got != "7" {
				t.Fatalf("expected limit=7, got %q", got)
			}
			if got := r.URL.Query().Get("view"); got != "summary" {
				t.Fatalf("expected compact verify to request view=summary, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource-uri": "res://v1/ws/ws-acme/result/table/tbl_1",
				"table-name":   "orders",
				"query": map[string]any{
					"rows": []any{
						map[string]any{"order-id": "ord-1"},
						map[string]any{"order-id": "ord-2"},
					},
					"count": float64(2),
					"page": map[string]any{
						"total-count": float64(12),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "verify", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--limit", "7",
	)
	if err != nil {
		t.Fatalf("resources table verify failed: %v\n%s", err, stdout)
	}
	if !sawMetadata || !sawPreview {
		t.Fatalf("expected metadata and preview requests, saw metadata=%v preview=%v", sawMetadata, sawPreview)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	verification, _ := data["verification"].(map[string]any)
	if verification["contentType"] != "application/vnd.breyta.table+json" {
		t.Fatalf("unexpected contentType: %#v", verification["contentType"])
	}
	if verification["previewRows"] != float64(2) {
		t.Fatalf("unexpected previewRows: %#v", verification["previewRows"])
	}
	if verification["rowsWritten"] != float64(12) {
		t.Fatalf("unexpected rowsWritten: %#v", verification["rowsWritten"])
	}
	metadata, _ := data["metadata"].(map[string]any)
	if metadata["tableName"] != "orders" {
		t.Fatalf("unexpected compact metadata tableName: %#v", metadata["tableName"])
	}
	if metadata["rowCount"] != float64(12) {
		t.Fatalf("unexpected compact metadata rowCount: %#v", metadata["rowCount"])
	}
	preview, _ := data["preview"].(map[string]any)
	if preview["rowsPreviewed"] != float64(2) {
		t.Fatalf("unexpected compact preview rowsPreviewed: %#v", preview["rowsPreviewed"])
	}
	if _, ok := preview["query"]; ok {
		t.Fatalf("expected compact preview to omit raw query payload, got %#v", preview["query"])
	}
	meta, _ := out["meta"].(map[string]any)
	if _, ok := meta["nextCommands"].([]any); !ok {
		t.Fatalf("expected nextCommands, got %#v", meta["nextCommands"])
	}
}

func TestResourcesTableVerify_FullKeepsRawPayloads(t *testing.T) {
	var sawPreview bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/resources/by-uri":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"uri":         "res://v1/ws/ws-acme/result/table/tbl_1",
					"contentType": "application/vnd.breyta.table+json",
					"schema":      map[string]any{"columns": []any{"order-id"}},
				},
			})
		case "/api/resources/content":
			sawPreview = true
			if got := r.URL.Query().Get("view"); got != "" {
				t.Fatalf("expected --full verify to omit view=summary, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"resource-uri": "res://v1/ws/ws-acme/result/table/tbl_1",
					"query": map[string]any{
						"rows": []any{
							map[string]any{"order-id": "ord-1"},
						},
						"count": float64(1),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "verify", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--full",
	)
	if err != nil {
		t.Fatalf("resources table verify --full failed: %v\n%s", err, stdout)
	}
	if !sawPreview {
		t.Fatalf("expected preview request")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	metadata, _ := data["metadata"].(map[string]any)
	if _, ok := metadata["schema"].(map[string]any); !ok {
		t.Fatalf("expected raw metadata schema in --full output, got %#v", metadata["schema"])
	}
	preview, _ := data["preview"].(map[string]any)
	query, _ := preview["query"].(map[string]any)
	if rows, ok := query["rows"].([]any); !ok || len(rows) != 1 {
		t.Fatalf("expected raw preview query rows in --full output, got %#v", query["rows"])
	}
}

func TestResourcesTableVerify_TreatsOKFalseRESTPayloadsAsErrors(t *testing.T) {
	tests := []struct {
		name        string
		failPath    string
		expectQuery bool
	}{
		{name: "metadata", failPath: "/api/resources/by-uri", expectQuery: false},
		{name: "preview", failPath: "/api/resources/content", expectQuery: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sawPreview bool
			srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/resources/by-uri":
					if tt.failPath == r.URL.Path {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"ok":    false,
							"error": map[string]any{"message": "metadata unavailable"},
						})
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"ok": true,
						"data": map[string]any{
							"uri":         "res://v1/ws/ws-acme/result/table/tbl_1",
							"contentType": "application/vnd.breyta.table+json",
						},
					})
				case "/api/resources/content":
					sawPreview = true
					if tt.failPath == r.URL.Path {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"ok":    false,
							"error": map[string]any{"message": "preview unavailable"},
						})
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"ok": true,
						"data": map[string]any{
							"query": map[string]any{"rows": []any{}},
						},
					})
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			stdout, _, err := runCLIArgs(t,
				"--dev",
				"--workspace", "ws-acme",
				"--api", srv.URL,
				"--token", "user-dev",
				"resources", "table", "verify", "res://v1/ws/ws-acme/result/table/tbl_1",
			)
			if err == nil {
				t.Fatalf("expected resources table verify to fail for ok=false payload\n%s", stdout)
			}
			if sawPreview != tt.expectQuery {
				t.Fatalf("unexpected preview request state: got %v want %v", sawPreview, tt.expectQuery)
			}
			if !strings.Contains(stdout, `"ok":false`) {
				t.Fatalf("expected ok=false output, got:\n%s", stdout)
			}
		})
	}
}

func TestResourcesRead_DefaultsToCompactTablePreviewLimit(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/content" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("uri"); got != "res://v1/ws/ws-acme/result/table/tbl_1" {
			t.Fatalf("expected uri query param, got %q", got)
		}
		if got := r.URL.Query().Get("view"); got != "summary" {
			t.Fatalf("expected default view=summary, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "25" {
			t.Fatalf("expected default compact limit=25, got %q", got)
		}
		if got := r.URL.Query().Get("offset"); got != "" {
			t.Fatalf("expected offset to be omitted by default, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceUri": "res://v1/ws/ws-acme/result/table/tbl_1",
			"tableName":   "orders",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "read", "res://v1/ws/ws-acme/result/table/tbl_1",
	)
	if err != nil {
		t.Fatalf("resources read failed: %v\n%s", err, stdout)
	}
}

func TestResourcesRead_FullOmitsDefaultPreviewLimit(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/content" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("limit"); got != "" {
			t.Fatalf("expected --full to omit default limit, got %q", got)
		}
		if got := r.URL.Query().Get("view"); got != "" {
			t.Fatalf("expected --full to omit summary view, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceUri": "res://v1/ws/ws-acme/result/table/tbl_1",
			"tableName":   "orders",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "read", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--full",
	)
	if err != nil {
		t.Fatalf("resources read --full failed: %v\n%s", err, stdout)
	}
}

func TestResourcesRead_PrettyKeepsCompactSummaryPreview(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/content" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("view"); got != "summary" {
			t.Fatalf("expected --pretty to keep view=summary, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "25" {
			t.Fatalf("expected --pretty to keep default compact limit=25, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceUri": "res://v1/ws/ws-acme/result/table/tbl_1",
			"tableName":   "orders",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"--pretty",
		"resources", "read", "res://v1/ws/ws-acme/result/table/tbl_1",
	)
	if err != nil {
		t.Fatalf("resources read --pretty failed: %v\n%s", err, stdout)
	}
}

func TestResourcesTableQuery_UsesTableQueryEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/query" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, _ := body["uri"].(string); got != "res://v1/ws/ws-acme/result/table/tbl_1" {
			t.Fatalf("expected uri in body, got %#v", body["uri"])
		}
		page, _ := body["page"].(map[string]any)
		if got, _ := page["mode"].(string); got != "offset" {
			t.Fatalf("expected page.mode=offset, got %#v", page["mode"])
		}
		if got, _ := page["limit"].(float64); got != 25 {
			t.Fatalf("expected page.limit=25, got %#v", page["limit"])
		}
		if got, _ := page["offset"].(float64); got != 50 {
			t.Fatalf("expected page.offset=50, got %#v", page["offset"])
		}
		partitions, _ := body["partition-keys"].([]any)
		if len(partitions) != 2 || partitions[0] != "month-2026-03" || partitions[1] != "month-2026-04" {
			t.Fatalf("expected partition-keys payload, got %#v", body["partition-keys"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tableName": "orders",
			"rows":      []any{map[string]any{"order-id": "ord-1"}},
			"count":     1,
			"page": map[string]any{
				"mode":   "offset",
				"limit":  25,
				"offset": 50,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "query", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--page-mode", "offset",
		"--offset", "50",
		"--select", "order-id,status",
		"--partition-keys", "month-2026-03,month-2026-04",
	)
	if err != nil {
		t.Fatalf("resources table query failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["tableName"].(string); got != "orders" {
		t.Fatalf("unexpected tableName: %v", data["tableName"])
	}
}

func TestResourcesTableQuery_DefaultsToOffsetPageMode(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/query" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		page, _ := body["page"].(map[string]any)
		if got, _ := page["mode"].(string); got != "offset" {
			t.Fatalf("expected default page.mode=offset, got %#v", page["mode"])
		}
		if got, _ := page["limit"].(float64); got != 5 {
			t.Fatalf("expected page.limit=5, got %#v", page["limit"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tableName": "orders",
			"rows":      []any{},
			"count":     0,
			"page": map[string]any{
				"mode":  "offset",
				"limit": 5,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "query", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--limit", "5",
	)
	if err != nil {
		t.Fatalf("resources table query without page-mode failed: %v\n%s", err, stdout)
	}
}

func TestResourcesTableQuery_SendsCursorPayloadWhenRequested(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/query" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, ok := body["offset"]; ok {
			t.Fatalf("did not expect offset in cursor request, got %#v", got)
		}
		page, _ := body["page"].(map[string]any)
		if got, _ := page["mode"].(string); got != "cursor" {
			t.Fatalf("expected page.mode=cursor, got %#v", page["mode"])
		}
		if got, _ := page["cursor"].(string); got != "cursor-25" {
			t.Fatalf("expected page.cursor=cursor-25, got %#v", page["cursor"])
		}
		if got, _ := page["include-total-count?"].(bool); !got {
			t.Fatalf("expected page.include-total-count?=true, got %#v", page["include-total-count?"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tableName": "orders",
			"rows":      []any{map[string]any{"order-id": "ord-26"}},
			"count":     1,
			"page": map[string]any{
				"mode":       "cursor",
				"limit":      25,
				"pageSize":   1,
				"cursor":     "cursor-25",
				"nextCursor": "cursor-26",
				"hasMore":    true,
				"totalCount": 30,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "query", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--page-mode", "cursor",
		"--limit", "25",
		"--cursor", "cursor-25",
		"--include-total-count",
		"--sort-json", `[["order-id","asc"]]`,
	)
	if err != nil {
		t.Fatalf("resources table cursor query failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	page, _ := data["page"].(map[string]any)
	if got, _ := page["nextCursor"].(string); got != "cursor-26" {
		t.Fatalf("unexpected nextCursor: %#v", page["nextCursor"])
	}
}

func TestResourcesTableExport_WritesCSVToStdout(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/export" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = io.WriteString(w, "order-id,status\nord-1,open\n")
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "export", "res://v1/ws/ws-acme/result/table/tbl_1",
	)
	if err != nil {
		t.Fatalf("resources table export failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "order-id,status\nord-1,open\n") {
		t.Fatalf("expected raw csv output, got:\n%s", stdout)
	}
}

func TestResourcesTableGetRow_UsesGetRowEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/get-row" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, _ := body["uri"].(string); got != "res://v1/ws/ws-acme/result/table/tbl_1" {
			t.Fatalf("expected uri in body, got %#v", body["uri"])
		}
		key, _ := body["key"].(map[string]any)
		if got, _ := key["event-id"].(string); got != "evt-1" {
			t.Fatalf("expected keyed get-row payload, got %#v", body["key"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"row": map[string]any{"event-id": "evt-1", "active": true},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "get-row", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--key", "event-id=evt-1",
	)
	if err != nil {
		t.Fatalf("resources table get-row failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "\"event-id\":\"evt-1\"") {
		t.Fatalf("expected row payload in output, got:\n%s", stdout)
	}
}

func TestResourcesTableImport_UsesImportEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/import-csv" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, _ := body["uri"].(string); got != "res://v1/ws/ws-acme/result/table/tbl_1" {
			t.Fatalf("expected uri in body, got %#v", body["uri"])
		}
		if got, _ := body["write-mode"].(string); got != "append" {
			t.Fatalf("expected write-mode=append, got %#v", body["write-mode"])
		}
		if got, _ := body["partition-key"].(string); got != "month-2026-03" {
			t.Fatalf("expected partition-key=month-2026-03, got %#v", body["partition-key"])
		}
		if got, _ := body["csv"].(string); !strings.Contains(got, "order-id,status") {
			t.Fatalf("expected csv content in body, got %#v", body["csv"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rowsWritten":  2,
			"rowsInserted": 2,
			"rowsUpdated":  0,
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	csvPath := dir + "/orders.csv"
	if err := os.WriteFile(csvPath, []byte("order-id,status\nord-1,open\nord-2,paid\n"), 0o644); err != nil {
		t.Fatalf("write csv fixture: %v", err)
	}

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "import", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--file", csvPath,
		"--write-mode", "append",
		"--partition-key", "month-2026-03",
	)
	if err != nil {
		t.Fatalf("resources table import failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["rowsWritten"].(float64); got != 2 {
		t.Fatalf("unexpected rowsWritten: %v", data["rowsWritten"])
	}
}

func TestResourcesTableImport_UsesNamedTableTargetWhenCreating(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/import-csv" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if _, ok := body["uri"]; ok {
			t.Fatalf("expected named-table import to omit uri, got %#v", body["uri"])
		}
		if got, _ := body["table"].(string); got != "orders-import" {
			t.Fatalf("expected table name in body, got %#v", body["table"])
		}
		if got, _ := body["write-mode"].(string); got != "upsert" {
			t.Fatalf("expected write-mode=upsert, got %#v", body["write-mode"])
		}
		keyFields, _ := body["key-fields"].([]any)
		if len(keyFields) != 1 || keyFields[0] != "order-id" {
			t.Fatalf("expected key-fields=[order-id], got %#v", body["key-fields"])
		}
		indexFields, _ := body["index-fields"].([]any)
		if len(indexFields) != 1 || indexFields[0] != "status" {
			t.Fatalf("expected index-fields=[status], got %#v", body["index-fields"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceUri":  "res://v1/ws/ws-acme/result/table/tbl_orders_import",
			"tableName":    "orders-import",
			"rowsWritten":  2,
			"rowsInserted": 2,
			"rowsUpdated":  0,
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	csvPath := dir + "/orders.csv"
	if err := os.WriteFile(csvPath, []byte("order-id,status\nord-1,open\nord-2,paid\n"), 0o644); err != nil {
		t.Fatalf("write csv fixture: %v", err)
	}

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "import", "orders-import",
		"--file", csvPath,
		"--write-mode", "upsert",
		"--key-fields", "order-id",
		"--index-fields", "status",
	)
	if err != nil {
		t.Fatalf("resources table import failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["tableName"].(string); got != "orders-import" {
		t.Fatalf("unexpected tableName: %v", data["tableName"])
	}
	if got, _ := data["resourceUri"].(string); got != "res://v1/ws/ws-acme/result/table/tbl_orders_import" {
		t.Fatalf("unexpected resourceUri: %v", data["resourceUri"])
	}
}

func TestResourcesTableAggregate_UsesExpandedPayload(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/aggregate" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		groupBy, _ := body["group-by"].([]any)
		if len(groupBy) != 1 {
			t.Fatalf("expected one group-by item, got %#v", body["group-by"])
		}
		groupByItem, _ := groupBy[0].(map[string]any)
		if got, _ := groupByItem["field"].(string); got != "amount" {
			t.Fatalf("expected group-by field=amount, got %#v", body["group-by"])
		}
		bucket, _ := groupByItem["bucket"].(map[string]any)
		if got, _ := bucket["op"].(string); got != "numeric-bin" {
			t.Fatalf("expected numeric-bin bucket, got %#v", groupByItem["bucket"])
		}
		if got, _ := bucket["size"].(float64); got != 10 {
			t.Fatalf("expected numeric-bin size=10, got %#v", groupByItem["bucket"])
		}
		metrics, _ := body["metrics"].([]any)
		if len(metrics) != 2 {
			t.Fatalf("expected two metrics, got %#v", body["metrics"])
		}
		firstMetric, _ := metrics[0].(map[string]any)
		if got, _ := firstMetric["op"].(string); got != "percentile" {
			t.Fatalf("expected first metric op=percentile, got %#v", firstMetric["op"])
		}
		if got, _ := firstMetric["p"].(float64); got != 0.95 {
			t.Fatalf("expected percentile p=0.95, got %#v", firstMetric["p"])
		}
		secondMetric, _ := metrics[1].(map[string]any)
		if got, _ := secondMetric["op"].(string); got != "median" {
			t.Fatalf("expected second metric op=median, got %#v", secondMetric["op"])
		}
		having, _ := body["having"].([]any)
		if len(having) != 1 {
			t.Fatalf("expected one having predicate, got %#v", body["having"])
		}
		orderBy, _ := body["order-by"].([]any)
		if len(orderBy) != 1 {
			t.Fatalf("expected one order-by term, got %#v", body["order-by"])
		}
		if got, _ := body["partition-key"].(string); got != "month-2026-03" {
			t.Fatalf("expected partition-key=month-2026-03, got %#v", body["partition-key"])
		}
		if got, _ := body["limit"].(float64); got != 25 {
			t.Fatalf("expected compact default aggregate limit=25, got %#v", body["limit"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []any{map[string]any{"amount-bin": 10.0, "p95-amount": 14.75, "median-amount": 12.5}},
			"count":   1,
			"limit":   10,
			"hasMore": false,
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "aggregate", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--group-by-json", `[{"field":"amount","bucket":{"op":"numeric-bin","size":10},"as":"amount-bin"}]`,
		"--metrics-json", `[{"op":"percentile","field":"amount","p":0.95,"as":"p95-amount"},{"op":"median","field":"amount","as":"median-amount"}]`,
		"--having-json", `[["amount-bin","=",10.0]]`,
		"--order-by-json", `[["amount-bin","asc"]]`,
		"--partition-key", "month-2026-03",
	)
	if err != nil {
		t.Fatalf("resources table aggregate failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "\"amount-bin\":10") {
		t.Fatalf("expected grouped aggregate result in output, got:\n%s", stdout)
	}
}

func TestResourcesTableUpdateCell_UsesUpdateEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/update-cell" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, _ := body["column"].(string); got != "status" {
			t.Fatalf("expected column=status, got %#v", body["column"])
		}
		if got, _ := body["value"].(bool); !got {
			t.Fatalf("expected bool value=true, got %#v", body["value"])
		}
		if got, _ := body["partition-key"].(string); got != "month-2026-03" {
			t.Fatalf("expected partition-key=month-2026-03, got %#v", body["partition-key"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"row": map[string]any{"status": true},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "update-cell", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--key", "order-id=ord-1",
		"--column", "status",
		"--value-json", "true",
		"--partition-key", "month-2026-03",
	)
	if err != nil {
		t.Fatalf("resources table update-cell failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "\"status\":true") {
		t.Fatalf("expected updated row in output, got:\n%s", stdout)
	}
}

func TestResourcesTableUpdateCellFormat_UsesFormatEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/update-cell-format" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		format, _ := body["format"].(map[string]any)
		if got, _ := format["display"].(string); got != "currency" {
			t.Fatalf("expected display=currency, got %#v", format["display"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"format": map[string]any{"display": "currency", "currency": "USD"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "update-cell-format", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--key", "order-id=ord-1",
		"--column", "amount",
		"--format-json", "{\"display\":\"currency\",\"currency\":\"USD\"}",
	)
	if err != nil {
		t.Fatalf("resources table update-cell-format failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "\"currency\":\"USD\"") {
		t.Fatalf("expected updated format in output, got:\n%s", stdout)
	}
}

func TestResourcesTableSetColumn_UsesSetColumnEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/set-column" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, _ := body["column"].(string); got != "customer-name" {
			t.Fatalf("expected column=customer-name, got %#v", body["column"])
		}
		definition, _ := body["definition"].(map[string]any)
		if got, _ := definition["semantic-type"].(string); got != "text" {
			t.Fatalf("expected semantic-type=text, got %#v", definition["semantic-type"])
		}
		enumDef, _ := definition["enum"].(map[string]any)
		options, _ := enumDef["options"].([]any)
		if len(options) != 1 {
			t.Fatalf("expected enum options payload, got %#v", definition["enum"])
		}
		computed, _ := definition["computed"].(map[string]any)
		if got, _ := computed["type"].(string); got != "lookup" {
			t.Fatalf("expected computed lookup payload, got %#v", definition["computed"])
		}
		partitions, _ := body["partition-keys"].([]any)
		if len(partitions) != 2 || partitions[0] != "month-2026-03" || partitions[1] != "month-2026-04" {
			t.Fatalf("expected partition-keys payload, got %#v", body["partition-keys"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"column": map[string]any{
				"column-name":   "customer-name",
				"semantic-type": "text",
				"computed": map[string]any{
					"type":             "lookup",
					"reference-column": "customer-id",
					"field":            "name",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "set-column", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--column", "customer-name",
		"--semantic-type", "text",
		"--enum-json", `{"options":[{"id":"open","name":"Open"}]}`,
		"--computed-json", `{"type":"lookup","reference-column":"customer-id","field":"name"}`,
		"--partition-keys", "month-2026-03,month-2026-04",
	)
	if err != nil {
		t.Fatalf("resources table set-column failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "\"customer-name\"") {
		t.Fatalf("expected updated column in output, got:\n%s", stdout)
	}
}

func TestResourcesTableRecompute_UsesRecomputeEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/recompute" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, _ := body["limit"].(float64); got != 250 {
			t.Fatalf("expected limit=250, got %#v", body["limit"])
		}
		if got, _ := body["offset"].(float64); got != 50 {
			t.Fatalf("expected offset=50, got %#v", body["offset"])
		}
		if got, _ := body["partition-key"].(string); got != "month-2026-03" {
			t.Fatalf("expected partition-key=month-2026-03, got %#v", body["partition-key"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rowsScanned": 250,
			"rowsUpdated": 200,
			"limit":       250,
			"offset":      50,
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "recompute", "res://v1/ws/ws-acme/result/table/tbl_1",
		"--limit", "250",
		"--offset", "50",
		"--partition-key", "month-2026-03",
	)
	if err != nil {
		t.Fatalf("resources table recompute failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "\"rowsUpdated\":200") {
		t.Fatalf("expected recompute summary in output, got:\n%s", stdout)
	}
}

func TestResourcesTableMaterializeJoin_UsesMaterializeJoinEndpoint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/table/materialize-join" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, _ := body["join-type"].(string); got != "inner" {
			t.Fatalf("expected join-type=inner, got %#v", body["join-type"])
		}
		left, _ := body["left"].(map[string]any)
		leftTable, _ := left["table"].(map[string]any)
		if got, _ := leftTable["ref"].(string); got != "res://v1/ws/ws-acme/result/table/tbl_left" {
			t.Fatalf("expected left table ref, got %#v", left["table"])
		}
		right, _ := body["right"].(map[string]any)
		rightRows, _ := right["rows"].([]any)
		if len(rightRows) != 1 {
			t.Fatalf("expected one inline right row, got %#v", right["rows"])
		}
		on, _ := body["on"].([]any)
		if len(on) != 1 {
			t.Fatalf("expected one join key mapping, got %#v", body["on"])
		}
		into, _ := body["into"].(map[string]any)
		if got, _ := into["table"].(string); got != "joined-orders" {
			t.Fatalf("expected destination table name, got %#v", into["table"])
		}
		if got, _ := body["op-id"].(string); got != "join-op-1" {
			t.Fatalf("expected op-id=join-op-1, got %#v", body["op-id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceUri": "res://v1/ws/ws-acme/result/table/tbl_joined",
			"rowsWritten": 3,
			"join": map[string]any{
				"matchedRows": 3,
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "table", "materialize-join",
		"--left-json", `{"table":{"ref":"res://v1/ws/ws-acme/result/table/tbl_left"},"select":["order-id","customer-id"]}`,
		"--right-json", `{"rows":[{"customer-id":"cust-1","name":"Acme"}]}`,
		"--on-json", `[{"left-field":"customer-id","right-field":"customer-id"}]`,
		"--project-json", `{"keep-left":"all","right-fields":[{"field":"name","as":"customer-name"}]}`,
		"--into-json", `{"table":"joined-orders","write-mode":"upsert","key-fields":["order-id"]}`,
		"--join-type", "inner",
		"--op-id", "join-op-1",
	)
	if err != nil {
		t.Fatalf("resources table materialize-join failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "\"rowsWritten\":3") {
		t.Fatalf("expected materialize-join summary in output, got:\n%s", stdout)
	}
}
