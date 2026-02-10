package skilldocs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type ManifestFile struct {
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"contentType"`
}

type Manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	SkillSlug     string         `json:"skillSlug"`
	Version       string         `json:"version"`
	MinCLIVersion string         `json:"minCliVersion"`
	KeyID         string         `json:"keyId"`
	Signature     string         `json:"signature"`
	Files         []ManifestFile `json:"files"`
}

type envelope struct {
	OK   bool     `json:"ok"`
	Data Manifest `json:"data"`
}

func endpoint(baseURL string, relPath string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("missing api base url")
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", fmt.Errorf("invalid api base url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimPrefix(relPath, "/")
	return u.String(), nil
}

func doGet(ctx context.Context, httpClient *http.Client, baseURL, token, relPath, accept string) (*http.Response, error) {
	hc := httpClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	e, err := endpoint(baseURL, relPath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if strings.TrimSpace(accept) != "" {
		req.Header.Set("Accept", accept)
	}
	return hc.Do(req)
}

func FetchManifest(ctx context.Context, httpClient *http.Client, baseURL, token, skillSlug string) (Manifest, error) {
	resp, err := doGet(ctx, httpClient, baseURL, token, "/api/docs/skills/"+url.PathEscape(skillSlug)+"/manifest", "application/json")
	if err != nil {
		return Manifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return Manifest{}, fmt.Errorf("manifest request failed (status=%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return Manifest{}, err
	}
	manifest := env.Data
	if manifest.SkillSlug == "" {
		return Manifest{}, fmt.Errorf("manifest missing skillSlug")
	}
	if len(manifest.Files) == 0 {
		return Manifest{}, fmt.Errorf("manifest has no files")
	}
	return manifest, nil
}

func fetchFile(ctx context.Context, httpClient *http.Client, baseURL, token, skillSlug, relPath string) ([]byte, error) {
	parts := strings.Split(strings.TrimSpace(relPath), "/")
	encodedParts := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		encodedParts = append(encodedParts, url.PathEscape(p))
	}
	if len(encodedParts) == 0 {
		return nil, fmt.Errorf("invalid manifest path %q", relPath)
	}
	rel := path.Join("/api/docs/skills", skillSlug, "files", strings.Join(encodedParts, "/"))
	resp, err := doGet(ctx, httpClient, baseURL, token, rel, "text/markdown, text/plain, */*")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, fmt.Errorf("file request failed for %s (status=%d): %s", relPath, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

func FetchBundle(ctx context.Context, httpClient *http.Client, baseURL, token, skillSlug string) (Manifest, map[string][]byte, error) {
	manifest, err := FetchManifest(ctx, httpClient, baseURL, token, skillSlug)
	if err != nil {
		return Manifest{}, nil, err
	}
	files := make(map[string][]byte, len(manifest.Files))
	for _, f := range manifest.Files {
		content, err := fetchFile(ctx, httpClient, baseURL, token, skillSlug, f.Path)
		if err != nil {
			return Manifest{}, nil, err
		}
		if f.Bytes > 0 && int64(len(content)) != f.Bytes {
			return Manifest{}, nil, fmt.Errorf("file size mismatch for %s (expected=%d got=%d)", f.Path, f.Bytes, len(content))
		}
		if strings.TrimSpace(f.SHA256) != "" {
			h := sha256.Sum256(content)
			if !strings.EqualFold(hex.EncodeToString(h[:]), strings.TrimSpace(f.SHA256)) {
				return Manifest{}, nil, fmt.Errorf("file checksum mismatch for %s", f.Path)
			}
		}
		files[f.Path] = content
	}
	if _, ok := files["SKILL.md"]; !ok {
		return Manifest{}, nil, fmt.Errorf("manifest missing required SKILL.md")
	}
	return manifest, files, nil
}
