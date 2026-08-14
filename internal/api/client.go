package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL     string
	WorkspaceID string
	Token       string
	HTTP        *http.Client
}

const (
	clientName      = "cli"
	clientUserAgent = "breyta-cli"
)

var readCommandRetryBackoffs = []time.Duration{150 * time.Millisecond, 450 * time.Millisecond}

func setClientHeaders(req *http.Request) {
	req.Header.Set("User-Agent", clientUserAgent)
	req.Header.Set("X-Breyta-Client", clientName)
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
	setClientHeaders(req)
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
	return c.doCommandWithEndpoint(ctx, endpoint, command, args, true)
}

func (c Client) doCommandWithEndpoint(ctx context.Context, endpoint string, command string, args map[string]any, includeWorkspace bool) (map[string]any, int, error) {
	return c.doCommandWithOperationID(ctx, endpoint, command, args, includeWorkspace, newOperationID())
}

func (c Client) doCommandWithOperationID(ctx context.Context, endpoint string, command string, args map[string]any, includeWorkspace bool, operationID string) (map[string]any, int, error) {
	return c.doCommandWithOperationState(ctx, endpoint, command, args, includeWorkspace, operationID, 0)
}

func (c Client) doCommandWithOperationState(ctx context.Context, endpoint string, command string, args map[string]any, includeWorkspace bool, operationID string, attemptOffset int) (map[string]any, int, error) {
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
	payloadBytes := buf.Bytes()

	backoffs := commandRetryBackoffs(command)
	for attempt := 0; ; attempt++ {
		out, status, err := c.doCommandRequest(ctx, endpoint, payloadBytes, includeWorkspace, operationID, attemptOffset+attempt+1)
		if shouldRetryCommandAttempt(ctx, status, err, attempt, backoffs) {
			if !waitBeforeRetry(ctx, backoffs[attempt]) {
				if ctx != nil && ctx.Err() != nil {
					return nil, status, ctx.Err()
				}
				if err != nil {
					return nil, status, err
				}
				return out, status, nil
			}
			continue
		}
		if err != nil {
			return nil, status, err
		}

		return out, status, nil
	}
}

func (c Client) doCommandRequest(ctx context.Context, endpoint string, payloadBytes []byte, includeWorkspace bool, operationID string, operationAttempt int) (map[string]any, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, 0, err
	}
	setClientHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(operationID) != "" {
		req.Header.Set("X-Breyta-Operation-ID", operationID)
	}
	if operationAttempt > 0 {
		req.Header.Set("X-Breyta-Operation-Attempt", strconv.Itoa(operationAttempt))
	}
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
	return out, resp.StatusCode, nil
}

func newOperationID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ""
	}
	random[6] = (random[6] & 0x0f) | 0x40
	random[8] = (random[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		random[0:4],
		random[4:6],
		random[6:8],
		random[8:10],
		random[10:16])
}

func commandRetryBackoffs(command string) []time.Duration {
	if !retryableCommand(command) {
		return nil
	}
	return readCommandRetryBackoffs
}

func retryableCommand(command string) bool {
	switch strings.TrimSpace(command) {
	case "flows.get",
		"flows.list",
		"flows.diff",
		"flows.versions.list",
		"runs.get",
		"runs.list",
		"runs.events",
		"workspace.export":
		return true
	default:
		return false
	}
}

// IsRetryableCommandFailure reports whether a failed read command can be
// treated as a transient result by a higher-level polling loop after the
// client's own retry budget is exhausted.
func IsRetryableCommandFailure(ctx context.Context, command string, status int, err error) bool {
	if !retryableCommand(command) {
		return false
	}
	if err != nil {
		return retryableCommandError(ctx, status, err) || retryableCommandStatusIfContextActive(ctx, status)
	}
	return retryableCommandStatus(status)
}

func shouldRetryCommandAttempt(ctx context.Context, status int, err error, attempt int, backoffs []time.Duration) bool {
	if attempt >= len(backoffs) {
		return false
	}
	if err != nil {
		return retryableCommandError(ctx, status, err) || retryableCommandStatusIfContextActive(ctx, status)
	}
	return retryableCommandStatus(status)
}

func retryableCommandStatusIfContextActive(ctx context.Context, status int) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return retryableCommandStatus(status)
}

func retryableCommandStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryableCommandError(ctx context.Context, status int, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid json response") {
		return retryableCommandStatus(status)
	}
	return strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "client.timeout") ||
		strings.Contains(msg, "timeout awaiting response headers") ||
		strings.Contains(msg, "invalid json response") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "unexpected eof")
}

func waitBeforeRetry(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	if ctx == nil {
		<-timer.C
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
