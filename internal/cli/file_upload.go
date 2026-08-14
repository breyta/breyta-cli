package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var fileUploadRetryBackoffs = []time.Duration{
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
}

func detectFileContentType(path string, explicit string, file *os.File) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	if ext := strings.TrimSpace(filepath.Ext(path)); ext != "" {
		switch strings.ToLower(ext) {
		case ".md", ".markdown":
			return "text/markdown; charset=utf-8", nil
		}
		if guessed := strings.TrimSpace(mime.TypeByExtension(ext)); guessed != "" {
			return guessed, nil
		}
	}
	if file == nil {
		return "", errors.New("file handle is required for content-type detection")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek file for content-type detection: %w", err)
	}
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read file for content-type detection: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("reset file after content-type detection: %w", err)
	}
	return http.DetectContentType(header[:n]), nil
}

func uploadFileResource(ctx context.Context, app *App, path string, filename string, contentType string, folder string, replaceExisting bool) (map[string]any, error) {
	file, err := openExplicitFile(path)
	if err != nil {
		return nil, fmt.Errorf("open upload file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat upload file: %w", err)
	}
	contentType, err = detectFileContentType(path, contentType, file)
	if err != nil {
		return nil, err
	}

	initBody := map[string]any{
		"filename":     filename,
		"content-type": contentType,
	}
	if strings.TrimSpace(folder) != "" {
		initBody["folder"] = folder
	}
	if replaceExisting {
		initBody["replace-existing"] = true
	}
	var initRetryBackoffs []time.Duration
	if replaceExisting {
		initRetryBackoffs = fileUploadRetryBackoffs
	}
	initResp, status, err := fileUploadREST(ctx, app, "/api/files/uploads/init", initBody, initRetryBackoffs)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fileUploadRESTError(status, initResp)
	}

	initData := fileUploadRESTPayload(initResp)
	resourceURI := firstNonBlankString(initData["uri"])
	uploadURL := firstNonBlankString(initData["upload-url"], initData["uploadUrl"])
	uploadSessionID := firstNonBlankString(initData["upload-session-id"], initData["uploadSessionId"])
	if resourceURI == "" {
		return nil, errors.New("upload init response missing resource uri")
	}

	if supportsSignedUploadURL(uploadURL) {
		if err := uploadWithSignedURL(ctx, uploadURL, contentType, file, fileInfo.Size()); err != nil {
			if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
				return nil, fmt.Errorf("signed upload failed (%v); reset upload file for direct upload fallback: %w", err, seekErr)
			}
			if directErr := uploadWithAPIDirect(ctx, app, resourceURI, contentType, file, fileInfo.Size()); directErr != nil {
				return nil, fmt.Errorf("signed upload failed (%v); direct upload fallback failed: %w", err, directErr)
			}
		}
	} else {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("reset upload file for direct upload: %w", err)
		}
		if err := uploadWithAPIDirect(ctx, app, resourceURI, contentType, file, fileInfo.Size()); err != nil {
			return nil, err
		}
	}

	completeBody := map[string]any{
		"uri": resourceURI,
	}
	// Forward the init-issued upload session id so the server can locate the
	// staged upload when replace-existing routes bytes through a staging path.
	// Without it, complete cannot find the session and reports 404 "Uploaded
	// object not found" for --name/--folder/--replace uploads.
	if strings.TrimSpace(uploadSessionID) != "" {
		completeBody["upload-session-id"] = uploadSessionID
	}
	completeResp, status, err := fileUploadREST(ctx, app, "/api/files/uploads/complete", completeBody, fileUploadRetryBackoffs)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fileUploadRESTError(status, completeResp)
	}
	completeData := fileUploadRESTPayload(completeResp)
	result := map[string]any{
		"resourceUri": resourceURI,
		"contentType": firstNonBlankString(completeData["content-type"], completeData["contentType"], contentType),
		"filename":    filename,
	}
	if sizeBytes, ok := completeData["size-bytes"]; ok {
		result["sizeBytes"] = sizeBytes
	} else if sizeBytes, ok := completeData["sizeBytes"]; ok {
		result["sizeBytes"] = sizeBytes
	} else if fileInfo.Size() >= 0 {
		result["sizeBytes"] = fileInfo.Size()
	}
	return result, nil
}

func fileUploadRESTPayload(resp any) map[string]any {
	out := mapStringAny(resp)
	if len(out) == 0 {
		return nil
	}
	if data := mapStringAny(out["data"]); len(data) > 0 {
		return data
	}
	return out
}

func fileUploadRESTError(status int, resp any) error {
	if out := mapStringAny(resp); len(out) > 0 {
		return fmt.Errorf("api error (status=%d): %s", status, formatAPIError(out))
	}
	msg := strings.TrimSpace(fmt.Sprintf("%v", resp))
	if msg == "" || msg == "<nil>" {
		msg = "unknown error"
	}
	return fmt.Errorf("api error (status=%d): %s", status, msg)
}

func fileUploadREST(ctx context.Context, app *App, path string, body map[string]any, backoffs []time.Duration) (any, int, error) {
	for attempt := 0; ; attempt++ {
		out, status, err := apiClient(app).DoREST(ctx, http.MethodPost, path, nil, body)
		if err != nil || !retryableFileUploadStatus(status) || attempt >= len(backoffs) {
			return out, status, err
		}
		if !waitForFileUploadRetry(ctx, backoffs[attempt]) {
			return out, status, ctx.Err()
		}
	}
}

func retryableFileUploadStatus(status int) bool {
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

func waitForFileUploadRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func supportsSignedUploadURL(uploadURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(uploadURL))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func uploadWithSignedURL(ctx context.Context, uploadURL string, contentType string, body io.Reader, contentLength int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, io.NopCloser(body))
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload failed (status=%d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

func uploadWithAPIDirect(ctx context.Context, app *App, resourceURI string, contentType string, body io.ReadSeeker, contentLength int64) error {
	query := url.Values{}
	query.Set("uri", resourceURI)
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if _, err := body.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("reset upload file for retry: %w", err)
			}
		}
		// Hide the file's io.Closer from http.NewRequest. The upload command owns
		// the file lifetime; a failed attempt must leave it open for Seek + retry.
		reader := struct{ io.Reader }{Reader: body}
		out, status, err := apiClient(app).DoRootRESTReader(ctx, http.MethodPut, "/api/files/uploads/direct", query, reader, contentType, contentLength, nil)
		if err != nil {
			return err
		}
		if !retryableFileUploadStatus(status) || attempt >= len(fileUploadRetryBackoffs) {
			if status >= 400 {
				return fileUploadRESTError(status, out)
			}
			return nil
		}
		if !waitForFileUploadRetry(ctx, fileUploadRetryBackoffs[attempt]) {
			return ctx.Err()
		}
	}
}
