package skilldocs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchBundle(t *testing.T) {
	skill := []byte("name: breyta\n")
	ref := []byte("# Ref\n")
	skillHash := sha256.Sum256(skill)
	refHash := sha256.Sum256(ref)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFetchBundleChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
