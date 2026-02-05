package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const githubLatestReleaseURL = "https://api.github.com/repos/breyta/breyta-cli/releases/latest"

type latestReleaseResponse struct {
	TagName string `json:"tag_name"`
}

func fetchLatestReleaseTag(ctx context.Context, client *http.Client, ifNoneMatch string) (tag string, etag string, notModified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		return "", "", false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "breyta-cli")
	if strings.TrimSpace(ifNoneMatch) != "" {
		req.Header.Set("If-None-Match", strings.TrimSpace(ifNoneMatch))
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return "", strings.TrimSpace(resp.Header.Get("ETag")), true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", false, fmt.Errorf("github latest release: http %d", resp.StatusCode)
	}

	var out latestReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", false, err
	}
	tag = strings.TrimSpace(out.TagName)
	if tag == "" {
		return "", "", false, fmt.Errorf("github latest release: missing tag_name")
	}
	return tag, strings.TrimSpace(resp.Header.Get("ETag")), false, nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}
