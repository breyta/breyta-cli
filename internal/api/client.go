package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL     string
	WorkspaceID string
	Token       string
	HTTP        *http.Client
}

func (c Client) baseEndpointFor(path string) (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", fmt.Errorf("missing api base url")
	}
	u, err := url.Parse(strings.TrimRight(c.BaseURL, "/"))
	if err != nil {
		return "", fmt.Errorf("invalid api url: %w", err)
	}
	p := strings.TrimSpace(path)
	p = strings.TrimPrefix(p, "/")
	u.Path = strings.TrimRight(u.Path, "/") + "/" + p
	return u.String(), nil
}

func (c Client) endpoint() (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", fmt.Errorf("missing api base url")
	}
	if strings.TrimSpace(c.WorkspaceID) == "" {
		return "", fmt.Errorf("missing workspace id")
	}
	return c.baseEndpointFor("/api/commands")
}

func (c Client) globalEndpoint() (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", fmt.Errorf("missing api base url")
	}
	return c.baseEndpointFor("/api/global/commands")
}

func (c Client) endpointFor(path string) (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", fmt.Errorf("missing api base url")
	}
	if strings.TrimSpace(c.WorkspaceID) == "" {
		return "", fmt.Errorf("missing workspace id")
	}
	return c.baseEndpointFor(path)
}

func (c Client) DoRootREST(ctx context.Context, method string, path string, query url.Values, body any) (any, int, error) {
	endpoint, err := c.baseEndpointFor(path)
	if err != nil {
		return nil, 0, err
	}
	var (
		r             io.Reader
		contentType   string
		contentLength int64 = -1
	)
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, 0, err
		}
		r = &buf
		contentType = "application/json"
		contentLength = int64(buf.Len())
	}
	return c.doRESTWithReader(ctx, endpoint, method, query, r, contentType, contentLength, false, nil)
}

// DoRootRESTBytes is like DoRootREST, but sends a pre-encoded request body and optional headers.
// This is useful for signing webhooks where the signature must match the exact request bytes.
func (c Client) DoRootRESTBytes(ctx context.Context, method string, path string, query url.Values, body []byte, headers map[string]string) (any, int, error) {
	endpoint, err := c.baseEndpointFor(path)
	if err != nil {
		return nil, 0, err
	}
	var (
		r             io.Reader
		contentLength int64 = -1
	)
	if body != nil {
		r = bytes.NewReader(body)
		contentLength = int64(len(body))
	}
	return c.doRESTWithReader(ctx, endpoint, method, query, r, "application/json", contentLength, true, headers)
}

func (c Client) DoREST(ctx context.Context, method string, path string, query url.Values, body any) (any, int, error) {
	endpoint, err := c.endpointFor(path)
	if err != nil {
		return nil, 0, err
	}
	var (
		r             io.Reader
		contentType   string
		contentLength int64 = -1
	)
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, 0, err
		}
		r = &buf
		contentType = "application/json"
		contentLength = int64(buf.Len())
	}
	return c.doRESTWithReader(ctx, endpoint, method, query, r, contentType, contentLength, true, nil)
}

func (c Client) DoRootRESTReader(ctx context.Context, method string, path string, query url.Values, body io.Reader, contentType string, contentLength int64, headers map[string]string) (any, int, error) {
	endpoint, err := c.baseEndpointFor(path)
	if err != nil {
		return nil, 0, err
	}
	return c.doRESTWithReader(ctx, endpoint, method, query, body, contentType, contentLength, true, headers)
}

func (c Client) doRESTWithReader(ctx context.Context, endpoint string, method string, query url.Values, body io.Reader, contentType string, contentLength int64, includeWorkspace bool, headers map[string]string) (any, int, error) {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second}
	}

	if len(query) > 0 {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, 0, err
		}
		u.RawQuery = query.Encode()
		endpoint = u.String()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, 0, err
	}
	if body != nil && strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	if strings.TrimSpace(c.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if includeWorkspace && strings.TrimSpace(c.WorkspaceID) != "" {
		req.Header.Set("X-Breyta-Workspace", c.WorkspaceID)
	}
	for k, v := range headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	// Allow non-JSON (HTML 404 pages etc) to surface as a raw string so callers can wrap.
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return string(b), resp.StatusCode, nil
	}
	return out, resp.StatusCode, nil
}

func (c Client) DoCommand(ctx context.Context, command string, args map[string]any) (map[string]any, int, error) {
	if strings.TrimSpace(command) == "" {
		return nil, 0, fmt.Errorf("missing command")
	}
	endpoint, err := c.endpoint()
	if err != nil {
		return nil, 0, err
	}
	return c.doCommandWithEndpoint(ctx, endpoint, command, args, true, true)
}

func (c Client) DoGlobalCommand(ctx context.Context, command string, args map[string]any) (map[string]any, int, error) {
	if strings.TrimSpace(command) == "" {
		return nil, 0, fmt.Errorf("missing command")
	}
	endpoint, err := c.globalEndpoint()
	if err != nil {
		return nil, 0, err
	}
	return c.doCommandWithEndpoint(ctx, endpoint, command, args, false, true)
}

func (c Client) doCommandWithEndpoint(ctx context.Context, endpoint string, command string, args map[string]any, includeWorkspace bool, allowLocalBootstrap bool) (map[string]any, int, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, 0, fmt.Errorf("missing command endpoint")
	}
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second}
	}

	filtered := map[string]any{}
	for k, v := range args {
		if k == "command" {
			continue
		}
		filtered[k] = v
	}
	payload := map[string]any{"command": command, "args": filtered}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if includeWorkspace && strings.TrimSpace(c.WorkspaceID) != "" {
		req.Header.Set("X-Breyta-Workspace", c.WorkspaceID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("invalid json response (status=%d): %w\n%s", resp.StatusCode, err, string(b))
	}
	if allowLocalBootstrap && includeWorkspace && c.shouldAutoBootstrapLocalWorkspace(out, resp.StatusCode) {
		bootstrapOut, bootstrapStatus, bootstrapErr := c.bootstrapLocalWorkspace(ctx)
		if bootstrapErr == nil && bootstrapStatus < 400 {
			retryOut, retryStatus, retryErr := c.doCommandWithEndpoint(ctx, endpoint, command, args, includeWorkspace, false)
			if retryErr != nil {
				return nil, 0, retryErr
			}
			annotateAutoBootstrappedLocalWorkspace(retryOut, c.WorkspaceID, bootstrapOut)
			return retryOut, retryStatus, nil
		}
		annotateLocalBootstrapError(out, bootstrapStatus, bootstrapErr)
	}
	return out, resp.StatusCode, nil
}

func (c Client) shouldAutoBootstrapLocalWorkspace(out map[string]any, status int) bool {
	if status != http.StatusForbidden {
		return false
	}
	if strings.TrimSpace(c.WorkspaceID) == "" || !isLoopbackBaseURL(c.BaseURL) {
		return false
	}
	msg := strings.ToLower(apiErrorMessage(out))
	return strings.Contains(msg, "not a workspace member")
}

func isLoopbackBaseURL(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func apiErrorMessage(out map[string]any) string {
	if out == nil {
		return ""
	}
	if errAny, ok := out["error"]; ok {
		switch v := errAny.(type) {
		case string:
			return strings.TrimSpace(v)
		case map[string]any:
			if msg, _ := v["message"].(string); strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
			if details, _ := v["details"].(string); strings.TrimSpace(details) != "" {
				return strings.TrimSpace(details)
			}
		}
	}
	return ""
}

func (c Client) bootstrapLocalWorkspace(ctx context.Context) (map[string]any, int, error) {
	endpoint, err := c.baseEndpointFor("/api/debug/workspace/bootstrap")
	if err != nil {
		return nil, 0, err
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]any{"workspaceId": strings.TrimSpace(c.WorkspaceID)}); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(c.Token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("x-debug-user-id", token)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("invalid json response (status=%d): %w\n%s", resp.StatusCode, err, string(b))
	}
	if resp.StatusCode >= 400 {
		msg := apiErrorMessage(out)
		if msg == "" {
			msg = strings.TrimSpace(string(b))
		}
		if msg == "" {
			msg = "unknown error"
		}
		return out, resp.StatusCode, fmt.Errorf("local workspace bootstrap failed (status=%d): %s", resp.StatusCode, msg)
	}
	return out, resp.StatusCode, nil
}

func ensureMeta(out map[string]any) map[string]any {
	if out == nil {
		return nil
	}
	if metaAny, ok := out["meta"]; ok {
		if meta, ok := metaAny.(map[string]any); ok && meta != nil {
			return meta
		}
	}
	meta := map[string]any{}
	out["meta"] = meta
	return meta
}

func annotateAutoBootstrappedLocalWorkspace(out map[string]any, workspaceID string, bootstrapOut map[string]any) {
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	info := map[string]any{
		"workspaceId": strings.TrimSpace(workspaceID),
		"reason":      "membership-403",
	}
	for _, key := range []string{"created", "member", "role"} {
		if v, ok := bootstrapOut[key]; ok {
			info[key] = v
		}
	}
	meta["localWorkspaceBootstrap"] = info
}

func annotateLocalBootstrapError(out map[string]any, status int, err error) {
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	if err != nil {
		meta["localWorkspaceBootstrapError"] = err.Error()
		return
	}
	if status > 0 {
		meta["localWorkspaceBootstrapError"] = fmt.Sprintf("local workspace bootstrap failed (status=%d)", status)
	}
}
