package live

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Bootstrap struct {
	Enabled         bool      `json:"enabled"`
	WorkspaceID     string    `json:"workspaceId"`
	WorkflowID      string    `json:"workflowId"`
	BaseURL         string    `json:"baseUrl"`
	SnapshotURL     string    `json:"snapshotUrl"`
	SignalsURL      string    `json:"signalsUrl"`
	StreamURL       string    `json:"streamUrl"`
	Auth            Auth      `json:"auth"`
	PollMs          int       `json:"pollMs"`
	RefreshBeforeMs int       `json:"refreshBeforeMs"`
	ReceivedAt      time.Time `json:"-"`
}

type Auth struct {
	Type      string `json:"type"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

func (b Bootstrap) PollInterval(defaultValue time.Duration) time.Duration {
	if b.PollMs <= 0 {
		return defaultValue
	}
	return time.Duration(b.PollMs) * time.Millisecond
}

func (b Bootstrap) RefreshBefore(defaultValue time.Duration) time.Duration {
	if b.RefreshBeforeMs <= 0 {
		return defaultValue
	}
	return time.Duration(b.RefreshBeforeMs) * time.Millisecond
}

func (b Bootstrap) TokenExpiresAt() (time.Time, bool) {
	raw := strings.TrimSpace(b.Auth.ExpiresAt)
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func DecodeBootstrap(v any) (Bootstrap, error) {
	var bootstrap Bootstrap
	b, err := json.Marshal(v)
	if err != nil {
		return bootstrap, err
	}
	if err := json.Unmarshal(b, &bootstrap); err != nil {
		return bootstrap, err
	}
	bootstrap.ReceivedAt = time.Now()
	return bootstrap, nil
}

type SnapshotClient struct {
	HTTP *http.Client
}

func (c SnapshotClient) Fetch(ctx context.Context, bootstrap Bootstrap) (Snapshot, error) {
	var snapshot Snapshot
	snapshotURL := strings.TrimSpace(bootstrap.SnapshotURL)
	if snapshotURL == "" {
		return snapshot, fmt.Errorf("missing realtime snapshot URL")
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, snapshotURL, nil)
	if err != nil {
		return snapshot, err
	}
	if strings.EqualFold(strings.TrimSpace(bootstrap.Auth.Type), "bearer") && strings.TrimSpace(bootstrap.Auth.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bootstrap.Auth.Token))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return snapshot, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return snapshot, fmt.Errorf("realtime snapshot request failed: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

type StreamClient struct {
	HTTP *http.Client
}

func (c StreamClient) Stream(ctx context.Context, bootstrap Bootstrap, snapshots chan<- Snapshot) error {
	streamURL := strings.TrimSpace(bootstrap.StreamURL)
	if streamURL == "" {
		return fmt.Errorf("missing realtime stream URL")
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 0}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if strings.EqualFold(strings.TrimSpace(bootstrap.Auth.Type), "bearer") && strings.TrimSpace(bootstrap.Auth.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bootstrap.Auth.Token))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("realtime stream request failed: status %d", resp.StatusCode)
	}
	return decodeSnapshotSSE(ctx, resp.Body, snapshots)
}

func decodeSnapshotSSE(ctx context.Context, r io.Reader, snapshots chan<- Snapshot) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var event string
	var data bytes.Buffer
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if err := emitSnapshotSSEEvent(ctx, event, data.Bytes(), snapshots); err != nil {
				return err
			}
			event = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if data.Len() > 0 {
		return emitSnapshotSSEEvent(ctx, event, data.Bytes(), snapshots)
	}
	return nil
}

func emitSnapshotSSEEvent(ctx context.Context, event string, data []byte, snapshots chan<- Snapshot) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if event != "" && event != "workspace_snapshot" && event != "snapshot" && event != "message" {
		return nil
	}
	snapshot, err := decodeStreamSnapshot(data)
	if err != nil {
		return err
	}
	select {
	case snapshots <- snapshot:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func decodeStreamSnapshot(data []byte) (Snapshot, error) {
	var wrapped struct {
		Type     string   `json:"type"`
		Snapshot Snapshot `json:"snapshot"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && !snapshotIsZero(wrapped.Snapshot) {
		return wrapped.Snapshot, nil
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func snapshotIsZero(snapshot Snapshot) bool {
	return snapshot.Workspace.WorkspaceID == "" &&
		len(snapshot.Runs) == 0 &&
		len(snapshot.Relations) == 0 &&
		len(snapshot.Nodes) == 0
}
