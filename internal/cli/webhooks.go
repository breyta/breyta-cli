package cli

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/breyta/breyta-cli/internal/format"
	"github.com/spf13/cobra"
)

type webhookFilePart struct {
	Field       string
	Path        string
	Filename    string
	ContentType string
	SizeBytes   int64
}

type webhookPayload struct {
	Body        []byte
	ContentType string
	InputMap    map[string]any
}

func newWebhooksCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Send webhook-style events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newWebhooksSendCmd(app))
	return cmd
}

func newWebhooksSendCmd(app *App) *cobra.Command {
	var eventPathRaw string
	var baseURLOverride string
	var draft bool
	var jsonPayload string
	var jsonFile string
	var formFields []string
	var multipartFiles []string
	var rawFile string
	var contentType string
	var headersRaw []string
	var apiKey string
	var bearerToken string
	var basicAuth string
	var headerAuth string
	var apiKeyLocation string
	var apiKeyParam string
	var hmacSecret string
	var signSpec string
	var signSecret string
	var signPublicKeyPath string
	var signPrivateKeyPath string
	var signatureHeader string
	var signatureFormat string
	var signaturePrefix string
	var timestampHeader string
	var timestampValue string
	var timestampMaxSkewMs int64
	var quiet bool
	var failOnHTTP int
	var printInputMap bool
	var validateOnly bool
	var persistResources bool
	var saveResponsePath string

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a webhook request",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(eventPathRaw) == "" {
				return writeFailure(cmd, app, "missing_path", errors.New("missing --path"), "Provide --path <event-path> (e.g. webhooks/orders).", nil)
			}

			if strings.TrimSpace(app.WorkspaceID) == "" {
				return writeFailure(cmd, app, "missing_workspace", errors.New("missing workspace id"), "Provide --workspace or set BREYTA_WORKSPACE.", nil)
			}

			if draft {
				if err := requireAPI(app); err != nil {
					return writeFailure(cmd, app, "api_auth_required", err, "Provide --token or run `breyta auth login`.", nil)
				}
			}

			baseURL := strings.TrimSpace(baseURLOverride)
			if baseURL == "" {
				if envURL := strings.TrimSpace(os.Getenv("BREYTA_API_URL")); envURL != "" {
					baseURL = envURL
				}
			}
			if baseURL == "" {
				ensureAPIURL(app)
				baseURL = app.APIURL
			}
			baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
			if baseURL == "" {
				return writeFailure(cmd, app, "missing_base_url", errors.New("missing base url"), "Provide --base-url or set BREYTA_API_URL.", nil)
			}

			payload, err := buildWebhookPayload(jsonPayload, jsonFile, formFields, multipartFiles, rawFile, contentType)
			if err != nil {
				return writeFailure(cmd, app, "payload_invalid", err, "Check payload flags; exactly one payload type is required.", nil)
			}

			headers := map[string]string{}
			if payload.ContentType != "" {
				headers["Content-Type"] = payload.ContentType
			}
			customHeaders, err := parseHeaderFlags(headersRaw)
			if err != nil {
				return writeFailure(cmd, app, "invalid_header", err, "Use --header 'Name: Value'.", nil)
			}
			for k, v := range customHeaders {
				headers[k] = v
			}

			query := url.Values{}
			if err := applyAuthFlags(headers, query, apiKey, bearerToken, basicAuth, headerAuth, apiKeyLocation, apiKeyParam); err != nil {
				return writeFailure(cmd, app, "auth_invalid", err, "Provide one auth option (--api-key, --bearer, or --basic).", nil)
			}

			signatureInfo, err := applySignatureHeaders(headers, payload.Body, hmacSecret, signSpec, signSecret, signPrivateKeyPath, signatureHeader, signatureFormat, signaturePrefix, timestampHeader, timestampValue)
			if err != nil {
				return writeFailure(cmd, app, "signature_invalid", err, "Verify signature flags and key paths.", nil)
			}

			if err := validateSignaturePreview(validateOnly, signatureInfo, signPublicKeyPath, timestampHeader, timestampValue, timestampMaxSkewMs); err != nil {
				return writeFailure(cmd, app, "signature_validation_failed", err, "Verify signature flags and keys.", nil)
			}

			if persistResources && !validateOnly {
				return writeFailure(cmd, app, "persist_requires_validate_only", errors.New("--persist-resources requires --validate-only"), "Re-run with --validate-only.", nil)
			}

			eventPath := escapePathSegments(eventPathRaw)
			if eventPath == "" {
				return writeFailure(cmd, app, "empty_path", errors.New("empty event path"), "Provide a non-empty --path (e.g. webhooks/orders).", map[string]any{"path": eventPathRaw})
			}

			endpoint := ""
			if draft {
				endpoint = fmt.Sprintf("/%s/api/events/draft/%s", strings.TrimSpace(app.WorkspaceID), eventPath)
			} else {
				endpoint = fmt.Sprintf("/%s/events/%s", strings.TrimSpace(app.WorkspaceID), eventPath)
			}

			fullURL := fmt.Sprintf("%s%s", baseURL, endpoint)

			if printInputMap && !validateOnly {
				preview := map[string]any{"input": payload.InputMap}
				_ = writeOut(cmd, app, preview)
			}

			if validateOnly {
				if err := requireAPI(app); err != nil {
					return writeFailure(cmd, app, "api_auth_required", err, "Provide --token or run `breyta auth login`.", nil)
				}
				validateQuery := query
				if persistResources {
					validateQuery.Set("persist-resources", "true")
				}
				if draft {
					validateQuery.Set("draft", "true")
				}
				validateEndpoint := fmt.Sprintf("/%s/api/events/validate/%s", strings.TrimSpace(app.WorkspaceID), eventPath)
				validateURL := fmt.Sprintf("%s%s", baseURL, validateEndpoint)
				client := apiClient(app)
				client.BaseURL = baseURL
				out, status, err := client.DoRootRESTBytes(context.Background(), http.MethodPost, validateEndpoint, validateQuery, payload.Body, headers)
				if err != nil {
					return writeFailure(cmd, app, "webhook_validate_failed", err, "Check API connectivity and webhook path.", map[string]any{"url": validateURL})
				}
				return writeREST(cmd, app, status, out)
			}

			client := apiClient(app)
			client.BaseURL = baseURL
			if !draft {
				client.Token = ""
			}
			out, status, err := client.DoRootRESTBytes(context.Background(), http.MethodPost, endpoint, query, payload.Body, headers)
			if err != nil {
				return writeFailure(cmd, app, "webhook_send_failed", err, "Check connectivity and webhook path.", map[string]any{"url": fullURL})
			}

			if strings.TrimSpace(saveResponsePath) != "" {
				if err := writeResponseFile(saveResponsePath, out, app.PrettyJSON); err != nil {
					return writeFailure(cmd, app, "response_write_failed", err, "Check the response path permissions.", map[string]any{"path": saveResponsePath})
				}
			}

			threshold := 400
			if failOnHTTP > 0 {
				threshold = failOnHTTP
			}

			if quiet {
				_ = writeOut(cmd, app, out)
				if status >= threshold {
					return errors.New("api error")
				}
				return nil
			}

			if status >= threshold {
				return writeREST(cmd, app, status, out)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&eventPathRaw, "path", "", "Webhook path (no workspace prefix)")
	cmd.Flags().StringVar(&baseURLOverride, "base-url", "", "API base URL (default: BREYTA_API_URL or config)")
	cmd.Flags().BoolVar(&draft, "draft", false, "Send to draft endpoint (/api/events/draft)")

	cmd.Flags().StringVar(&jsonPayload, "json", "", "JSON payload string")
	cmd.Flags().StringVar(&jsonFile, "json-file", "", "JSON payload file path")
	cmd.Flags().StringArrayVar(&formFields, "form", nil, "Form field key=value (repeatable)")
	cmd.Flags().StringArrayVar(&multipartFiles, "multipart-file", nil, "Multipart file field=path (repeatable)")
	cmd.Flags().StringVar(&rawFile, "raw-file", "", "Raw file payload path")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Content-Type override")
	cmd.Flags().StringArrayVar(&headersRaw, "header", nil, "Custom header (Name: Value) (repeatable)")

	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key value")
	cmd.Flags().StringVar(&bearerToken, "bearer", "", "Bearer token")
	cmd.Flags().StringVar(&basicAuth, "basic", "", "Basic auth user:pass")
	cmd.Flags().StringVar(&headerAuth, "header-auth", "", "Header name for api-key (default X-API-Key)")
	cmd.Flags().StringVar(&apiKeyLocation, "api-key-location", "header", "API key location (header|query)")
	cmd.Flags().StringVar(&apiKeyParam, "api-key-param", "", "API key query param name (default token)")

	cmd.Flags().StringVar(&hmacSecret, "hmac-secret", "", "HMAC secret (shortcut for --sign algo:hmac-sha256)")
	cmd.Flags().StringVar(&signSpec, "sign", "", "Signature spec (algo:hmac-sha256|ecdsa-p256)")
	cmd.Flags().StringVar(&signSecret, "sign-secret", "", "Signature secret (HMAC)")
	cmd.Flags().StringVar(&signPublicKeyPath, "sign-public-key", "", "Signature public key path (validation only)")
	cmd.Flags().StringVar(&signPrivateKeyPath, "sign-private-key", "", "Signature private key path (ECDSA)")
	cmd.Flags().StringVar(&signatureHeader, "signature-header", "X-Signature", "Signature header name")
	cmd.Flags().StringVar(&signatureFormat, "signature-format", "base64", "Signature format (base64|hex)")
	cmd.Flags().StringVar(&signaturePrefix, "signature-prefix", "", "Signature prefix (optional)")
	cmd.Flags().StringVar(&timestampHeader, "timestamp-header", "", "Timestamp header name (sign timestamp + payload)")
	cmd.Flags().StringVar(&timestampValue, "timestamp", "", "Timestamp value (unix seconds or ms)")
	cmd.Flags().Int64Var(&timestampMaxSkewMs, "timestamp-max-skew-ms", 0, "Max allowed timestamp skew for validation")

	cmd.Flags().BoolVar(&quiet, "quiet", false, "Print only response JSON")
	cmd.Flags().IntVar(&failOnHTTP, "fail-on-http", 0, "Exit non-zero if status >= code")
	cmd.Flags().BoolVar(&printInputMap, "print-input-map", false, "Print inferred flow/input map before sending")
	cmd.Flags().BoolVar(&validateOnly, "validate-only", false, "Validate payload without triggering a run (API mode)")
	cmd.Flags().BoolVar(&persistResources, "persist-resources", false, "Persist multipart resources during validation (requires --validate-only)")
	cmd.Flags().StringVar(&saveResponsePath, "save-response", "", "Save response JSON to path")

	return cmd
}

func buildWebhookPayload(jsonPayload string, jsonFile string, formFields []string, multipartFiles []string, rawFile string, contentTypeOverride string) (webhookPayload, error) {
	jsonPayload = strings.TrimSpace(jsonPayload)
	jsonFile = strings.TrimSpace(jsonFile)
	rawFile = strings.TrimSpace(rawFile)

	multipartFiles = trimStringSlice(multipartFiles)
	formFields = trimStringSlice(formFields)

	payloadTypes := 0
	if jsonPayload != "" {
		payloadTypes++
	}
	if jsonFile != "" {
		payloadTypes++
	}
	if rawFile != "" {
		payloadTypes++
	}
	if len(multipartFiles) > 0 {
		payloadTypes++
	} else if len(formFields) > 0 {
		payloadTypes++
	}

	if payloadTypes != 1 {
		return webhookPayload{}, fmt.Errorf("exactly one payload type is required")
	}

	if jsonPayload != "" {
		body := []byte(jsonPayload)
		input, err := decodeJSONInput(body)
		if err != nil {
			return webhookPayload{}, err
		}
		return webhookPayload{Body: body, ContentType: pickContentType(contentTypeOverride, "application/json"), InputMap: input}, nil
	}

	if jsonFile != "" {
		body, err := os.ReadFile(jsonFile)
		if err != nil {
			return webhookPayload{}, fmt.Errorf("read json-file: %w", err)
		}
		input, err := decodeJSONInput(body)
		if err != nil {
			return webhookPayload{}, err
		}
		return webhookPayload{Body: body, ContentType: pickContentType(contentTypeOverride, "application/json"), InputMap: input}, nil
	}

	if rawFile != "" {
		body, err := os.ReadFile(rawFile)
		if err != nil {
			return webhookPayload{}, fmt.Errorf("read raw-file: %w", err)
		}
		contentType := pickContentType(contentTypeOverride, "application/octet-stream")
		return webhookPayload{Body: body, ContentType: contentType, InputMap: map[string]any{}}, nil
	}

	if len(multipartFiles) > 0 {
		parts, err := parseMultipartFiles(multipartFiles)
		if err != nil {
			return webhookPayload{}, err
		}
		formMap, err := parseFormFields(formFields)
		if err != nil {
			return webhookPayload{}, err
		}
		body, contentType, inputMap, err := buildMultipartBody(formMap, parts)
		if err != nil {
			return webhookPayload{}, err
		}
		if contentTypeOverride != "" {
			return webhookPayload{}, fmt.Errorf("content-type override is not supported with multipart payloads")
		}
		return webhookPayload{Body: body, ContentType: contentType, InputMap: inputMap}, nil
	}

	formMap, err := parseFormFields(formFields)
	if err != nil {
		return webhookPayload{}, err
	}
	encoded := url.Values{}
	for k, v := range formMap {
		switch vv := v.(type) {
		case []string:
			for _, item := range vv {
				encoded.Add(k, item)
			}
		default:
			encoded.Set(k, fmt.Sprint(v))
		}
	}
	body := []byte(encoded.Encode())
	return webhookPayload{Body: body, ContentType: pickContentType(contentTypeOverride, "application/x-www-form-urlencoded"), InputMap: formMap}, nil
}

func pickContentType(override string, fallback string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return fallback
}

func trimStringSlice(items []string) []string {
	trimmed := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}
	return trimmed
}

func decodeJSONInput(body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return map[string]any{}, nil
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid json payload: %w", err)
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{"value": v}, nil
}

func parseFormFields(formFields []string) (map[string]any, error) {
	out := map[string]any{}
	for _, raw := range formFields {
		key, val, err := splitKeyValue(raw)
		if err != nil {
			return nil, err
		}
		if existing, ok := out[key]; ok {
			switch v := existing.(type) {
			case []string:
				out[key] = append(v, val)
			default:
				out[key] = []string{fmt.Sprint(v), val}
			}
			continue
		}
		out[key] = val
	}
	return out, nil
}

func parseMultipartFiles(items []string) ([]webhookFilePart, error) {
	parts := make([]webhookFilePart, 0, len(items))
	for _, raw := range items {
		key, path, err := splitKeyValue(raw)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		filename := filepath.Base(path)
		contentType := mime.TypeByExtension(filepath.Ext(filename))
		parts = append(parts, webhookFilePart{
			Field:       key,
			Path:        path,
			Filename:    filename,
			ContentType: contentType,
			SizeBytes:   info.Size(),
		})
	}
	return parts, nil
}

func splitKeyValue(raw string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid key=value: %s", raw)
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	if key == "" || val == "" {
		return "", "", fmt.Errorf("invalid key=value: %s", raw)
	}
	return key, val, nil
}

func buildMultipartBody(formMap map[string]any, files []webhookFilePart) ([]byte, string, map[string]any, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	input := map[string]any{}
	for key, val := range formMap {
		switch vv := val.(type) {
		case []string:
			for _, item := range vv {
				if err := writer.WriteField(key, item); err != nil {
					return nil, "", nil, fmt.Errorf("write field: %w", err)
				}
			}
			input[key] = vv
		default:
			if err := writer.WriteField(key, fmt.Sprint(vv)); err != nil {
				return nil, "", nil, fmt.Errorf("write field: %w", err)
			}
			input[key] = vv
		}
	}

	for _, part := range files {
		body, err := os.ReadFile(part.Path)
		if err != nil {
			return nil, "", nil, fmt.Errorf("read %s: %w", part.Path, err)
		}
		contentType := part.ContentType
		if contentType == "" {
			contentType = http.DetectContentType(body)
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, part.Field, part.Filename))
		h.Set("Content-Type", contentType)
		w, err := writer.CreatePart(h)
		if err != nil {
			return nil, "", nil, fmt.Errorf("create part: %w", err)
		}
		if _, err := w.Write(body); err != nil {
			return nil, "", nil, fmt.Errorf("write part: %w", err)
		}
		input[part.Field] = map[string]any{
			"content-type": contentType,
			"filename":     part.Filename,
			"path":         "resource://placeholder",
			"size-bytes":   len(body),
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", nil, fmt.Errorf("close multipart: %w", err)
	}

	return buf.Bytes(), writer.FormDataContentType(), input, nil
}

func parseHeaderFlags(headers []string) (map[string]string, error) {
	out := map[string]string{}
	for _, raw := range headers {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header: %s", raw)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid header: %s", raw)
		}
		out[key] = val
	}
	return out, nil
}

func applyAuthFlags(headers map[string]string, query url.Values, apiKey string, bearerToken string, basicAuth string, headerAuth string, apiKeyLocation string, apiKeyParam string) error {
	apiKey = strings.TrimSpace(apiKey)
	bearerToken = strings.TrimSpace(bearerToken)
	basicAuth = strings.TrimSpace(basicAuth)

	count := 0
	if apiKey != "" {
		count++
	}
	if bearerToken != "" {
		count++
	}
	if basicAuth != "" {
		count++
	}
	if count > 1 {
		return errors.New("multiple auth flags provided")
	}
	if apiKey == "" && bearerToken == "" && basicAuth == "" {
		return nil
	}

	if apiKey != "" {
		location := strings.ToLower(strings.TrimSpace(apiKeyLocation))
		if location == "" {
			location = "header"
		}
		switch location {
		case "header":
			name := strings.TrimSpace(headerAuth)
			if name == "" {
				name = "X-API-Key"
			}
			headers[name] = apiKey
		case "query":
			param := strings.TrimSpace(apiKeyParam)
			if param == "" {
				param = "token"
			}
			query.Set(param, apiKey)
		default:
			return fmt.Errorf("invalid api-key-location: %s", apiKeyLocation)
		}
		return nil
	}

	if bearerToken != "" {
		headers["Authorization"] = "Bearer " + bearerToken
		return nil
	}

	if basicAuth != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(basicAuth))
		return nil
	}

	return nil
}

type signaturePreview struct {
	Algo           string
	SignatureBytes []byte
	MessageBytes   []byte
	Timestamp      string
}

func applySignatureHeaders(headers map[string]string, payload []byte, hmacSecret string, signSpec string, signSecret string, signPrivateKeyPath string, signatureHeader string, signatureFormat string, signaturePrefix string, timestampHeader string, timestampValue string) (*signaturePreview, error) {
	if strings.TrimSpace(hmacSecret) != "" && strings.TrimSpace(signSpec) != "" {
		return nil, errors.New("--hmac-secret cannot be combined with --sign")
	}

	algo := strings.TrimSpace(signSpec)
	if strings.TrimSpace(hmacSecret) != "" {
		algo = "hmac-sha256"
		signSecret = hmacSecret
	}

	if algo == "" {
		return nil, nil
	}
	algo = strings.TrimPrefix(algo, "algo:")
	algo = strings.TrimSpace(algo)
	if algo == "" {
		return nil, errors.New("invalid --sign value")
	}

	format := strings.ToLower(strings.TrimSpace(signatureFormat))
	if format == "" {
		format = "base64"
	}
	if format != "base64" && format != "hex" {
		return nil, fmt.Errorf("invalid signature-format: %s", signatureFormat)
	}

	if strings.TrimSpace(signatureHeader) == "" {
		signatureHeader = "X-Signature"
	}

	message := payload
	if strings.TrimSpace(timestampHeader) != "" {
		timestamp := strings.TrimSpace(timestampValue)
		if timestamp == "" {
			timestamp = strconv.FormatInt(time.Now().UTC().Unix(), 10)
		}
		headers[timestampHeader] = timestamp
		message = append([]byte(timestamp), payload...)
		timestampValue = timestamp
	} else if strings.TrimSpace(timestampValue) != "" {
		return nil, errors.New("--timestamp requires --timestamp-header")
	}

	var sigBytes []byte
	switch algo {
	case "hmac-sha256":
		secret := strings.TrimSpace(signSecret)
		if secret == "" {
			return nil, errors.New("missing --sign-secret for hmac-sha256")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(message)
		sigBytes = mac.Sum(nil)
	case "ecdsa-p256":
		keyPath := strings.TrimSpace(signPrivateKeyPath)
		if keyPath == "" {
			return nil, errors.New("missing --sign-private-key for ecdsa-p256")
		}
		key, err := readECDSAPrivateKey(keyPath)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(message)
		sigBytes, err = ecdsa.SignASN1(rand.Reader, key, hash[:])
		if err != nil {
			return nil, fmt.Errorf("sign ecdsa: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported signature algo: %s", algo)
	}

	encoded := encodeSignature(sigBytes, format)
	if signaturePrefix != "" {
		encoded = signaturePrefix + encoded
	}
	if encoded != "" {
		headers[signatureHeader] = encoded
	}

	return &signaturePreview{Algo: algo, SignatureBytes: sigBytes, MessageBytes: message, Timestamp: timestampValue}, nil
}

func encodeSignature(sig []byte, format string) string {
	switch format {
	case "hex":
		return hex.EncodeToString(sig)
	default:
		return base64.StdEncoding.EncodeToString(sig)
	}
}

func validateSignaturePreview(validateOnly bool, preview *signaturePreview, publicKeyPath string, timestampHeader string, timestampValue string, maxSkewMs int64) error {
	if strings.TrimSpace(publicKeyPath) == "" {
		return nil
	}
	if !validateOnly {
		return errors.New("--sign-public-key requires --validate-only")
	}
	if preview == nil {
		return errors.New("signature preview missing")
	}
	if preview.Algo != "ecdsa-p256" {
		return errors.New("--sign-public-key only applies to ecdsa-p256")
	}

	pub, err := readECDSAPublicKey(publicKeyPath)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(preview.MessageBytes)
	if !ecdsa.VerifyASN1(pub, hash[:], preview.SignatureBytes) {
		return errors.New("ecdsa signature verification failed")
	}

	if strings.TrimSpace(timestampHeader) != "" && maxSkewMs > 0 {
		ts := strings.TrimSpace(timestampValue)
		if ts == "" {
			ts = preview.Timestamp
		}
		if ts == "" {
			return errors.New("missing timestamp for validation")
		}
		tsMs, err := parseTimestampMs(ts)
		if err != nil {
			return err
		}
		skew := time.Now().UTC().Sub(time.UnixMilli(tsMs))
		if skew < 0 {
			skew = -skew
		}
		if skew.Milliseconds() > maxSkewMs {
			return fmt.Errorf("timestamp skew exceeds max (%dms)", maxSkewMs)
		}
	}

	return nil
}

func parseTimestampMs(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("empty timestamp")
	}
	raw = strings.TrimPrefix(raw, "+")
	val, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp: %w", err)
	}
	if len(raw) <= 10 {
		return val * 1000, nil
	}
	return val, nil
}

func readECDSAPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid private key PEM")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not ECDSA")
	}
	return key, nil
}

func readECDSAPublicKey(path string) (*ecdsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid public key PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not ECDSA")
	}
	return key, nil
}

func buildValidateOnlyPreview(fullURL string, headers map[string]string, query url.Values, body []byte) map[string]any {
	preview := map[string]any{
		"method":  http.MethodPost,
		"url":     fullURL,
		"headers": headers,
	}
	if len(query) > 0 {
		preview["query"] = query
	}
	if utf8.Valid(body) {
		preview["body"] = string(body)
	} else {
		preview["body_base64"] = base64.StdEncoding.EncodeToString(body)
	}
	preview["body_bytes"] = len(body)
	return map[string]any{"data": preview}
}

func writeResponseFile(path string, data any, pretty bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return format.WriteJSON(file, data, pretty)
}
