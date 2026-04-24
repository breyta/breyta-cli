package cli_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestIncidentsListBuildsCanonicalRequest(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"incident-id": "incident-1", "status": "open"},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-breyta",
		"--token", "tok-1",
		"incidents", "list",
		"--status", "open",
		"--mine",
		"--limit", "15",
	)
	if err != nil {
		t.Fatalf("command failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if gotPath != "/api/incidents" {
		t.Fatalf("expected /api/incidents, got %q", gotPath)
	}
	if gotQuery.Get("status") != "open" {
		t.Fatalf("expected status=open, got %q", gotQuery.Get("status"))
	}
	if gotQuery.Get("scope") != "mine" {
		t.Fatalf("expected scope=mine, got %q", gotQuery.Get("scope"))
	}
	if gotQuery.Get("limit") != "15" {
		t.Fatalf("expected limit=15, got %q", gotQuery.Get("limit"))
	}
	out := decodeEnvelope(t, stdout)
	if !out.OK {
		t.Fatalf("expected ok=true, got output: %s", stdout)
	}
}

func TestIncidentsShowBuildsFailureLimitRequest(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"incident": map[string]any{"incident-id": "incident-1"},
			"failures": []map[string]any{{"failure-id": "failure-1"}},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-breyta",
		"--token", "tok-1",
		"incidents", "show", "incident-1",
		"--failure-limit", "5",
	)
	if err != nil {
		t.Fatalf("command failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if gotPath != "/api/incidents/incident-1" {
		t.Fatalf("expected /api/incidents/incident-1, got %q", gotPath)
	}
	if gotQuery.Get("limit") != "5" {
		t.Fatalf("expected limit=5, got %q", gotQuery.Get("limit"))
	}
	out := decodeEnvelope(t, stdout)
	if !out.OK {
		t.Fatalf("expected ok=true, got output: %s", stdout)
	}
}

func TestIncidentOperatorCommandsBuildCanonicalRequests(t *testing.T) {
	var requests []struct {
		Method string
		Path   string
		Query  url.Values
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, struct {
			Method string
			Path   string
			Query  url.Values
		}{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()})
		switch r.URL.Path {
		case "/api/incidents/incident-1/lanes":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"lane-id": "ws-breyta/orders-sync/order-1"}},
			})
		case "/api/incidents/incident-1/acknowledge":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"incident": map[string]any{"incident-id": "incident-1", "status": "open", "operator-disposition": "acknowledged"},
			})
		case "/api/incidents/incident-1/snooze":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"incident": map[string]any{"incident-id": "incident-1", "snoozed-until": "2026-04-08T12:00:00Z"},
			})
		case "/api/incidents/incident-1/suppress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"incident": map[string]any{"incident-id": "incident-1", "status": "open", "operator-disposition": "suppressed-until-recovered"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	for _, args := range [][]string{
		{"incidents", "lanes", "incident-1", "--limit", "8"},
		{"incidents", "acknowledge", "incident-1"},
		{"incidents", "snooze", "incident-1", "--for", "2H"},
		{"incidents", "suppress", "incident-1"},
	} {
		cliArgs := append([]string{"--dev", "--api", srv.URL, "--workspace", "ws-breyta", "--token", "tok-1"}, args...)
		stdout, stderr, err := runCLIArgs(t, cliArgs...)
		if err != nil {
			t.Fatalf("command failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		out := decodeEnvelope(t, stdout)
		if !out.OK {
			t.Fatalf("expected ok=true, got output: %s", stdout)
		}
	}

	if len(requests) != 4 {
		t.Fatalf("expected 4 requests, got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet || requests[0].Path != "/api/incidents/incident-1/lanes" {
		t.Fatalf("unexpected first request: %#v", requests[0])
	}
	if requests[0].Query.Get("limit") != "8" {
		t.Fatalf("unexpected lanes query: %#v", requests[0].Query)
	}
	if requests[1].Method != http.MethodPost || requests[1].Path != "/api/incidents/incident-1/acknowledge" {
		t.Fatalf("unexpected second request: %#v", requests[1])
	}
	if requests[2].Method != http.MethodPost || requests[2].Path != "/api/incidents/incident-1/snooze" {
		t.Fatalf("unexpected third request: %#v", requests[2])
	}
	if requests[2].Query.Get("for") != "2h" {
		t.Fatalf("unexpected snooze query: %#v", requests[2].Query)
	}
	if requests[3].Method != http.MethodPost || requests[3].Path != "/api/incidents/incident-1/suppress" {
		t.Fatalf("unexpected fourth request: %#v", requests[3])
	}
}

func TestIncidentSnoozeRejectsInvalidDurationBeforeRequest(t *testing.T) {
	var requestCount int

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-breyta",
		"--token", "tok-1",
		"incidents", "snooze", "incident-1",
		"--for", "tomorrow",
	)
	if err == nil {
		t.Fatalf("expected command failure for invalid duration\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if requestCount != 0 {
		t.Fatalf("expected no API requests for invalid duration, got %d", requestCount)
	}
}

func TestDigestsCommandsBuildCanonicalRequests(t *testing.T) {
	var requests []struct {
		Method string
		Path   string
		Query  url.Values
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, struct {
			Method string
			Path   string
			Query  url.Values
		}{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()})
		switch r.URL.Path {
		case "/api/digests/preferences":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"preferences":      map[string]any{"digest-cadence": "monthly"},
				"settings-web-url": "https://flows.breyta.ai/ws-breyta/settings/my-updates",
			})
		case "/api/digests/preferences/cadence":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"preferences": map[string]any{"digest-cadence": r.URL.Query().Get("cadence")},
			})
		case "/api/digests":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"digest-id": "digest-1"}},
			})
		case "/api/digests/digest-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"digest": map[string]any{"digest-id": "digest-1"},
			})
		case "/api/digests/digest-1/deliveries":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"id": "delivery-1", "channel": "in-app"}},
			})
		case "/api/digests/digest-1/mark-read":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updated-count": 1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	for _, args := range [][]string{
		{"digests", "cadence"},
		{"digests", "cadence", "set", "monthly"},
		{"digests", "list", "--kind", "scheduled", "--status", "materialized", "--cadence", "monthly", "--limit", "10"},
		{"digests", "show", "digest-1"},
		{"digests", "deliveries", "digest-1", "--channel", "in-app", "--limit", "7"},
		{"digests", "mark-read", "digest-1"},
	} {
		cliArgs := append([]string{"--dev", "--api", srv.URL, "--workspace", "ws-breyta", "--token", "tok-1"}, args...)
		stdout, stderr, err := runCLIArgs(t, cliArgs...)
		if err != nil {
			t.Fatalf("command failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		out := decodeEnvelope(t, stdout)
		if !out.OK {
			t.Fatalf("expected ok=true, got output: %s", stdout)
		}
	}

	if len(requests) != 6 {
		t.Fatalf("expected 6 requests, got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet || requests[0].Path != "/api/digests/preferences" {
		t.Fatalf("expected first request to /api/digests/preferences, got %q", requests[0].Path)
	}
	if requests[1].Method != http.MethodPost || requests[1].Path != "/api/digests/preferences/cadence" {
		t.Fatalf("expected second request to /api/digests/preferences/cadence, got %#v", requests[1])
	}
	if requests[1].Query.Get("cadence") != "monthly" {
		t.Fatalf("unexpected cadence query: %#v", requests[1].Query)
	}
	if requests[2].Method != http.MethodGet || requests[2].Path != "/api/digests" {
		t.Fatalf("expected third request to /api/digests, got %q", requests[2].Path)
	}
	if requests[2].Query.Get("kind") != "scheduled" || requests[2].Query.Get("status") != "materialized" || requests[2].Query.Get("cadence") != "monthly" || requests[2].Query.Get("limit") != "10" {
		t.Fatalf("unexpected digest list query: %#v", requests[2].Query)
	}
	if requests[3].Method != http.MethodGet || requests[3].Path != "/api/digests/digest-1" {
		t.Fatalf("expected fourth request to /api/digests/digest-1, got %#v", requests[3])
	}
	if requests[4].Method != http.MethodGet || requests[4].Path != "/api/digests/digest-1/deliveries" {
		t.Fatalf("expected fifth request to /api/digests/digest-1/deliveries, got %#v", requests[4])
	}
	if requests[4].Query.Get("channel") != "in-app" || requests[4].Query.Get("limit") != "7" {
		t.Fatalf("unexpected digest deliveries query: %#v", requests[4].Query)
	}
	if requests[5].Method != http.MethodPost || requests[5].Path != "/api/digests/digest-1/mark-read" {
		t.Fatalf("expected sixth request to POST /api/digests/digest-1/mark-read, got %#v", requests[5])
	}
}

func TestDigestsCadenceRejectsInvalidValueBeforeRequest(t *testing.T) {
	var requestCount int

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-breyta",
		"--token", "tok-1",
		"digests", "cadence", "set", "hourly",
	)
	if err == nil {
		t.Fatalf("expected command failure for invalid cadence\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if requestCount != 0 {
		t.Fatalf("expected no API requests for invalid cadence, got %d", requestCount)
	}
}

func TestDigestsListRejectsInvalidCadenceFilterBeforeRequest(t *testing.T) {
	var requestCount int

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--api", srv.URL,
		"--workspace", "ws-breyta",
		"--token", "tok-1",
		"digests", "list",
		"--cadence", "hourly",
	)
	if err == nil {
		t.Fatalf("expected command failure for invalid cadence filter\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if requestCount != 0 {
		t.Fatalf("expected no API requests for invalid cadence filter, got %d", requestCount)
	}
}
