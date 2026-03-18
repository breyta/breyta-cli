package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTriggersLogs_APIEndpoint(t *testing.T) {
	var gotPath string
	var gotLimit string
	var gotOutcome string
	var gotDelivery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotLimit = r.URL.Query().Get("limit")
		gotOutcome = r.URL.Query().Get("outcome")
		gotDelivery = r.URL.Query().Get("delivery")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"triggerId": "trg-1",
			"items":     []any{},
			"hasMore":   false,
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "tok-123",
		"triggers", "logs", "trg-1",
		"--limit", "5",
		"--outcome", "accepted",
		"--delivery", "del-1",
	)
	if err != nil {
		t.Fatalf("triggers logs failed: %v\n%s", err, stdout)
	}
	if gotPath != "/api/triggers/trg-1/logs" {
		t.Fatalf("expected /api/triggers/trg-1/logs, got %q", gotPath)
	}
	if gotLimit != "5" {
		t.Fatalf("expected limit=5, got %q", gotLimit)
	}
	if gotOutcome != "accepted" {
		t.Fatalf("expected outcome=accepted, got %q", gotOutcome)
	}
	if gotDelivery != "del-1" {
		t.Fatalf("expected delivery=del-1, got %q", gotDelivery)
	}
}
