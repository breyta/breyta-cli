package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJobsWorkerUploadFileResourceRetriesStableTransientStages(t *testing.T) {
	originalBackoffs := jobsWorkerUploadRetryBackoffs
	jobsWorkerUploadRetryBackoffs = []time.Duration{0}
	t.Cleanup(func() { jobsWorkerUploadRetryBackoffs = originalBackoffs })

	const resourceURI = "res://v1/ws/ws-acme/file/report"
	counts := map[string]int{}
	var directBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.URL.Path]++
		if counts[r.URL.Path] == 1 {
			http.Error(w, "temporary gateway failure", http.StatusBadGateway)
			return
		}
		switch r.URL.Path {
		case "/api/files/uploads/init":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"uri": resourceURI}})
		case "/api/files/uploads/direct":
			body, _ := io.ReadAll(r.Body)
			directBodies = append(directBodies, string(body))
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/api/files/uploads/complete":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"contentType": "text/markdown"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte("report body"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := jobsWorkerUploadFileResource(context.Background(), &App{
		APIURL: srv.URL, WorkspaceID: "ws-acme", Token: "token",
	}, path, "stable-report.md", "text/markdown", "", true)
	if err != nil {
		t.Fatalf("upload failed after transient retries: %v", err)
	}
	if result["resourceUri"] != resourceURI {
		t.Fatalf("expected resource URI %q, got %#v", resourceURI, result["resourceUri"])
	}
	for _, endpoint := range []string{"/api/files/uploads/init", "/api/files/uploads/direct", "/api/files/uploads/complete"} {
		if counts[endpoint] != 2 {
			t.Fatalf("expected one retry for %s, got %d calls", endpoint, counts[endpoint])
		}
	}
	if len(directBodies) != 1 || directBodies[0] != "report body" {
		t.Fatalf("expected reset direct upload body, got %#v", directBodies)
	}
}

func TestJobsWorkerUploadFileResourceDoesNotRetryUnstableInit(t *testing.T) {
	originalBackoffs := jobsWorkerUploadRetryBackoffs
	jobsWorkerUploadRetryBackoffs = []time.Duration{0}
	t.Cleanup(func() { jobsWorkerUploadRetryBackoffs = originalBackoffs })

	initCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initCalls++
		http.Error(w, "temporary gateway failure", http.StatusBadGateway)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte("report body"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := jobsWorkerUploadFileResource(context.Background(), &App{
		APIURL: srv.URL, WorkspaceID: "ws-acme", Token: "token",
	}, path, "report.md", "text/markdown", "", false)
	if err == nil {
		t.Fatal("expected transient init failure")
	}
	if initCalls != 1 {
		t.Fatalf("unsafe unnamed init must not retry, got %d calls", initCalls)
	}
}

func TestJobsWorkerWaitForUploadRetryStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if jobsWorkerWaitForUploadRetry(ctx, time.Hour) {
		t.Fatal("cancelled retry wait must not continue")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancelled retry wait took too long: %s", elapsed)
	}
}
