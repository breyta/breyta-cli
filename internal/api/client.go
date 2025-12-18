package api

import (
        "bytes"
        "context"
        "encoding/json"
        "fmt"
        "io"
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

func (c Client) endpoint() (string, error) {
        if strings.TrimSpace(c.BaseURL) == "" {
                return "", fmt.Errorf("missing api base url")
        }
        if strings.TrimSpace(c.WorkspaceID) == "" {
                return "", fmt.Errorf("missing workspace id")
        }
        u, err := url.Parse(strings.TrimRight(c.BaseURL, "/"))
        if err != nil {
                return "", fmt.Errorf("invalid api url: %w", err)
        }
        u.Path = strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(c.WorkspaceID) + "/api/commands"
        return u.String(), nil
}

func (c Client) endpointFor(path string) (string, error) {
        if strings.TrimSpace(c.BaseURL) == "" {
                return "", fmt.Errorf("missing api base url")
        }
        if strings.TrimSpace(c.WorkspaceID) == "" {
                return "", fmt.Errorf("missing workspace id")
        }
        u, err := url.Parse(strings.TrimRight(c.BaseURL, "/"))
        if err != nil {
                return "", fmt.Errorf("invalid api url: %w", err)
        }
        p := strings.TrimSpace(path)
        p = strings.TrimPrefix(p, "/")
        u.Path = strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(c.WorkspaceID) + "/" + p
        return u.String(), nil
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

        payload := map[string]any{"command": command}
        for k, v := range args {
                // Allow args to override nothing except we protect "command".
                if k == "command" {
                        continue
                }
                payload[k] = v
        }

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
