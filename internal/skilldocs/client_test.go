package skilldocs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newLocalTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		if isLocalListenerDenied(err) {
			t.Skipf("local HTTP test server skipped: sandbox denied loopback listener creation: %v", err)
		}
		t.Fatalf("failed to start local test server: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func isLocalListenerDenied(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission denied")
}

func TestFetchBundle(t *testing.T) {
	skill := []byte("name: breyta\n")
	ref := []byte("# Ref\n")
	skillHash := sha256.Sum256(skill)
	refHash := sha256.Sum256(ref)

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/skills/breyta/manifest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"schemaVersion": 1,
					"skillSlug":     "breyta",
					"version":       "dev",
					"files": []map[string]any{
						{"path": "SKILL.md", "sha256": hex.EncodeToString(skillHash[:]), "bytes": len(skill), "contentType": "text/markdown"},
						{"path": "references/ref.md", "sha256": hex.EncodeToString(refHash[:]), "bytes": len(ref), "contentType": "text/markdown"},
					},
				},
			})
		case "/api/docs/skills/breyta/files/SKILL.md":
			_, _ = w.Write(skill)
		case "/api/docs/skills/breyta/files/references/ref.md":
			_, _ = w.Write(ref)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	manifest, files, err := FetchBundle(context.Background(), srv.Client(), srv.URL, "", "breyta")
	if err != nil {
		t.Fatalf("fetch bundle: %v", err)
	}
	if manifest.SkillSlug != "breyta" {
		t.Fatalf("expected skill slug breyta, got %q", manifest.SkillSlug)
	}
	if got := string(files["SKILL.md"]); got != string(skill) {
		t.Fatalf("unexpected SKILL.md content: %q", got)
	}
	if got := string(files["references/ref.md"]); got != string(ref) {
		t.Fatalf("unexpected references/ref.md content: %q", got)
	}
}

func TestFetchBundleFetchesFilesWithManifestCacheKey(t *testing.T) {
	currentSkill := []byte("name: breyta\n# Current\n")
	staleSkill := []byte("name: breyta\n# Stale\n")
	currentHash := sha256.Sum256(currentSkill)
	currentHashHex := hex.EncodeToString(currentHash[:])
	var fileQuery string

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/skills/breyta/manifest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"schemaVersion": 1,
					"skillSlug":     "breyta",
					"version":       "runtime-current",
					"files": []map[string]any{
						{"path": "SKILL.md", "sha256": currentHashHex, "bytes": len(currentSkill), "contentType": "text/markdown"},
					},
				},
			})
		case "/api/docs/skills/breyta/files/SKILL.md":
			fileQuery = r.URL.Query().Get("v")
			if r.Header.Get("Cache-Control") != "no-cache" {
				t.Errorf("expected no-cache request header, got %q", r.Header.Get("Cache-Control"))
			}
			if fileQuery == "" {
				_, _ = w.Write(staleSkill)
				return
			}
			_, _ = w.Write(currentSkill)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, files, err := FetchBundle(context.Background(), srv.Client(), srv.URL, "", "breyta")
	if err != nil {
		t.Fatalf("fetch bundle: %v", err)
	}
	if got := string(files["SKILL.md"]); got != string(currentSkill) {
		t.Fatalf("unexpected SKILL.md content: %q", got)
	}
	if !strings.Contains(fileQuery, "runtime-current") || !strings.Contains(fileQuery, currentHashHex) {
		t.Fatalf("file request did not include manifest cache key, got %q", fileQuery)
	}
}

func TestFetchBundleChecksumMismatch(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/skills/breyta/manifest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"schemaVersion": 1,
					"skillSlug":     "breyta",
					"version":       "dev",
					"files": []map[string]any{
						{"path": "SKILL.md", "sha256": "deadbeef", "bytes": 4, "contentType": "text/markdown"},
					},
				},
			})
		case "/api/docs/skills/breyta/files/SKILL.md":
			_, _ = w.Write([]byte("name: breyta\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, _, err := FetchBundle(context.Background(), srv.Client(), srv.URL, "", "breyta")
	if err == nil {
		t.Fatalf("expected checksum mismatch error")
	}
}
