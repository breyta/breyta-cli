package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
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
	defaultCommandRetryAttempts = 3
	defaultCommandRetryBaseMS   = 250
	defaultCommandRetryMaxMS    = 2000
	maxCommandRetryShift        = 20
)

func commandIdempotencyKey(command string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(command), ".", "-")
	if normalized == "" {
		normalized = "unknown-command"
	}
	return fmt.Sprintf("cli:%s:%d", normalized, time.Now().UnixNano())
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func commandRetryAttempts() int {
	enabled := strings.ToLower(strings.TrimSpace(os.Getenv("BREYTA_CLI_COMMAND_RETRY_ENABLED")))
	if enabled == "false" || enabled == "0" || enabled == "no" {
		return 1
	}
	attempts := envInt("BREYTA_CLI_COMMAND_RETRY_ATTEMPTS", defaultCommandRetryAttempts)
	if attempts < 1 {
		return 1
	}
	return attempts
}

func parseRetryAfter(headerValue string) time.Duration {
	s := strings.TrimSpace(headerValue)
	if s == "" {
		return 0
	}
	if secs, err := strconv.Atoi(s); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(s); err == nil {
		d := time.Until(at)
		if d <= 0 {
			return 0
		}
		return d
	}
	return 0
}

func commandRetryDelay(attempt int, retryAfterHeader string) time.Duration {
	retryAfter := parseRetryAfter(retryAfterHeader)
	if retryAfter > 0 {
		return retryAfter
	}
	if attempt < 1 {
		attempt = 1
	}
	baseMS := envInt("BREYTA_CLI_COMMAND_RETRY_BASE_MS", defaultCommandRetryBaseMS)
	maxMS := envInt("BREYTA_CLI_COMMAND_RETRY_MAX_MS", defaultCommandRetryMaxMS)
	if baseMS < 0 {
		baseMS = 0
	}
	if maxMS < 0 {
		maxMS = 0
	}
	if maxMS > 0 && baseMS > maxMS {
		baseMS = maxMS
	}
	shift := attempt - 1
	if shift > maxCommandRetryShift {
		shift = maxCommandRetryShift
	}
	multiplier := int64(1) << shift
	delayMS := int64(baseMS) * multiplier
	if maxMS > 0 && delayMS > int64(maxMS) {
		delayMS = int64(maxMS)
	}
	if delayMS <= 0 {
		return 0
	}
	// Deterministic tiny jitter avoids synchronized retry spikes without introducing random test flakiness.
	jitterMS := int64((attempt % 97) * 37 % 97)
	if maxMS > 0 {
		maxWithJitter := int64(maxMS)
		if delayMS+jitterMS > maxWithJitter {
			return time.Duration(maxWithJitter) * time.Millisecond
		}
	}
	maxDurationMS := int64(^uint64(0)>>1) / int64(time.Millisecond)
	if delayMS > maxDurationMS-jitterMS {
		delayMS = maxDurationMS - jitterMS
	}
	return time.Duration(delayMS+jitterMS) * time.Millisecond
}

func shouldRetryCommandTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// Treat non-context transport errors as retryable (EOF/reset/etc).
	return true
}

func waitCommandRetryDelay(ctx context.Context, attempt int, retryAfterHeader string) error {
	delay := commandRetryDelay(attempt, retryAfterHeader)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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

	var r io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, 0, err
		}
		r = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, r)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(c.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
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

// DoRootRESTBytes is like DoRootREST, but sends a pre-encoded request body and optional headers.
// This is useful for signing webhooks where the signature must match the exact request bytes.
func (c Client) DoRootRESTBytes(ctx context.Context, method string, path string, query url.Values, body []byte, headers map[string]string) (any, int, error) {
	endpoint, err := c.baseEndpointFor(path)
	if err != nil {
		return nil, 0, err
	}
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

	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, r)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(c.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if strings.TrimSpace(c.WorkspaceID) != "" {
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

func (c Client) DoREST(ctx context.Context, method string, path string, query url.Values, body any) (any, int, error) {
	endpoint, err := c.endpointFor(path)
	if err != nil {
		return nil, 0, err
	}
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

	var r io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, 0, err
		}
		r = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, r)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(c.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("X-Breyta-Workspace", c.WorkspaceID)

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

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	idempotencyKey := commandIdempotencyKey(command)

	attempts := commandRetryAttempts()
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(c.Token) != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		req.Header.Set("X-Breyta-Workspace", c.WorkspaceID)
		req.Header.Set("Idempotency-Key", idempotencyKey)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			if attempt < attempts && shouldRetryCommandTransportError(err) {
				if waitErr := waitCommandRetryDelay(ctx, attempt, ""); waitErr != nil {
					return nil, 0, waitErr
				}
				continue
			}
			return nil, 0, err
		}

		b, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, resp.StatusCode, readErr
		}

		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) && attempt < attempts {
			if waitErr := waitCommandRetryDelay(ctx, attempt, resp.Header.Get("Retry-After")); waitErr != nil {
				return nil, 0, waitErr
			}
			continue
		}

		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("invalid json response (status=%d): %w\n%s", resp.StatusCode, err, string(b))
		}
		return out, resp.StatusCode, nil
	}

	return nil, 0, fmt.Errorf("command request exhausted retries without response")
}
