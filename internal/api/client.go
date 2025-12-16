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
