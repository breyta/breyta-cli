package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultMCPTimeout     = 60 * time.Second
	defaultMCPServerName  = "breyta-workspace"
	defaultMCPTokenEnvVar = "BREYTA_MCP_TOKEN"
)

type mcpPolicyOptions struct {
	Toolsets     string
	Tools        string
	ExcludeTools string
	ReadOnly     bool
}

type mcpProxyOptions struct {
	WorkspaceID string
	APIURL      string
	Token       string
	Policy      mcpPolicyOptions
	Timeout     time.Duration
	In          io.Reader
	Out         io.Writer
	Err         io.Writer
	HTTP        *http.Client
}

type mcpSetupOptions struct {
	Provider    string
	Transport   string
	ServerName  string
	WorkspaceID string
	APIURL      string
	TokenEnvVar string
	Policy      mcpPolicyOptions
}

type mcpHTTPSession struct {
	SessionID       string
	ProtocolVersion string
}

func newMCPCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Configure and run Breyta workspace MCP clients",
		Long: strings.TrimSpace(`
Configure Breyta workspace MCP for coding agents and ACP-compatible sessions.

Use direct HTTP MCP when the host supports Streamable HTTP. Use the stdio proxy
when the host only supports stdio MCP servers. Every generated server entry is
bound to one explicit workspace id; use multiple named entries for multiple
workspaces.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newMCPStdioCmd(app))
	cmd.AddCommand(newMCPConfigCmd(app))
	cmd.AddCommand(newMCPDoctorCmd(app))
	return cmd
}

func newMCPStdioCmd(app *App) *cobra.Command {
	var workspaceID string
	var toolsets string
	var tools string
	var excludeTools string
	var readOnly bool
	var timeout time.Duration
	var tokenEnvVar string

	cmd := &cobra.Command{
		Use:   "stdio --workspace-id <workspace-id>",
		Short: "Run a workspace-bound stdio MCP proxy",
		Long: strings.TrimSpace(`
Run a stdio MCP server that bridges JSON-RPC messages to the Breyta workspace
MCP HTTP endpoint.

This is intended for MCP hosts and ACP-compatible agents that can launch stdio
MCP servers but cannot connect to hosted Streamable HTTP MCP directly. The
workspace id must be explicit so the proxy cannot silently follow a mutable
default workspace.
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedWorkspace, err := explicitMCPWorkspaceID(cmd, app, workspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			app.WorkspaceID = resolvedWorkspace
			if err := applyMCPTokenEnvVar(cmd, app, tokenEnvVar); err != nil {
				return writeErr(cmd, err)
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			opts := mcpProxyOptions{
				WorkspaceID: resolvedWorkspace,
				APIURL:      app.APIURL,
				Token:       app.Token,
				Policy: mcpPolicyOptions{
					Toolsets:     toolsets,
					Tools:        tools,
					ExcludeTools: excludeTools,
					ReadOnly:     readOnly,
				},
				Timeout: timeout,
				In:      cmd.InOrStdin(),
				Out:     cmd.OutOrStdout(),
				Err:     cmd.ErrOrStderr(),
			}
			return runMCPStdioProxy(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace id to bind this MCP server to (required)")
	cmd.Flags().StringVar(&toolsets, "toolsets", "", "Comma-separated MCP toolsets to expose (for example read,setup,debug,feedback)")
	cmd.Flags().StringVar(&tools, "tools", "", "Comma-separated MCP tool names to include")
	cmd.Flags().StringVar(&excludeTools, "exclude-tools", "", "Comma-separated MCP tool names to hide")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Expose only read-only MCP tools")
	cmd.Flags().StringVar(&tokenEnvVar, "token-env-var", "", "Environment variable containing the service-account API key")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultMCPTimeout, "Upstream MCP HTTP request timeout")
	return cmd
}

func newMCPConfigCmd(app *App) *cobra.Command {
	var opts mcpSetupOptions

	cmd := &cobra.Command{
		Use:     "config --workspace-id <workspace-id>",
		Aliases: []string{"init", "setup"},
		Short:   "Print MCP client configuration for one workspace",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedWorkspace, err := explicitMCPWorkspaceID(cmd, app, opts.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			app.WorkspaceID = resolvedWorkspace
			ensureAPIURL(app)
			opts.WorkspaceID = resolvedWorkspace
			opts.APIURL = firstNonBlankString(opts.APIURL, app.APIURL)
			rendered, err := renderMCPClientConfig(opts)
			if err != nil {
				return writeErr(cmd, err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), rendered)
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Provider, "provider", "generic", "Client provider (generic|codex|claude|cursor|vscode|windsurf|cline|roo|continue|gemini|opencode|zed|goose|acp)")
	cmd.Flags().StringVar(&opts.Transport, "transport", "stdio", "MCP transport to render (stdio|http)")
	cmd.Flags().StringVar(&opts.ServerName, "name", defaultMCPServerName, "MCP server entry name")
	cmd.Flags().StringVar(&opts.WorkspaceID, "workspace-id", "", "Workspace id to bind this MCP server to (required)")
	cmd.Flags().StringVar(&opts.APIURL, "api-url", "", "Breyta API base URL for the rendered config")
	cmd.Flags().StringVar(&opts.TokenEnvVar, "token-env-var", defaultMCPTokenEnvVar, "Environment variable containing the service-account API key")
	cmd.Flags().StringVar(&opts.Policy.Toolsets, "toolsets", "", "Comma-separated MCP toolsets to expose")
	cmd.Flags().StringVar(&opts.Policy.Tools, "tools", "", "Comma-separated MCP tool names to include")
	cmd.Flags().StringVar(&opts.Policy.ExcludeTools, "exclude-tools", "", "Comma-separated MCP tool names to hide")
	cmd.Flags().BoolVar(&opts.Policy.ReadOnly, "read-only", false, "Expose only read-only MCP tools")
	return cmd
}

func newMCPDoctorCmd(app *App) *cobra.Command {
	var workspaceID string
	var toolsets string
	var tools string
	var excludeTools string
	var readOnly bool
	var timeout time.Duration
	var tokenEnvVar string

	cmd := &cobra.Command{
		Use:   "doctor --workspace-id <workspace-id>",
		Short: "Verify workspace MCP setup without invoking mutating tools",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedWorkspace, err := explicitMCPWorkspaceID(cmd, app, workspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			app.WorkspaceID = resolvedWorkspace
			if err := applyMCPTokenEnvVar(cmd, app, tokenEnvVar); err != nil {
				return writeErr(cmd, err)
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			result, err := runMCPDoctor(cmd.Context(), mcpProxyOptions{
				WorkspaceID: resolvedWorkspace,
				APIURL:      app.APIURL,
				Token:       app.Token,
				Policy: mcpPolicyOptions{
					Toolsets:     toolsets,
					Tools:        tools,
					ExcludeTools: excludeTools,
					ReadOnly:     readOnly,
				},
				Timeout: timeout,
			})
			if err != nil {
				return writeFailure(cmd, app, "mcp_doctor_failed", err, "Check workspace id, service-account scopes, API URL, and MCP policy narrowing flags.", result)
			}
			return writeData(cmd, app, nil, result)
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace id to verify (required)")
	cmd.Flags().StringVar(&toolsets, "toolsets", "", "Comma-separated MCP toolsets to expose while verifying")
	cmd.Flags().StringVar(&tools, "tools", "", "Comma-separated MCP tool names to include while verifying")
	cmd.Flags().StringVar(&excludeTools, "exclude-tools", "", "Comma-separated MCP tool names to hide while verifying")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Verify with read-only MCP exposure")
	cmd.Flags().StringVar(&tokenEnvVar, "token-env-var", "", "Environment variable containing the service-account API key")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultMCPTimeout, "Upstream MCP HTTP request timeout")
	return cmd
}

func applyMCPTokenEnvVar(cmd *cobra.Command, app *App, tokenEnvVar string) error {
	tokenEnvVar = strings.TrimSpace(tokenEnvVar)
	if tokenEnvVar == "" || flagExplicit(cmd, "token") || flagExplicit(cmd, "api-key") {
		return nil
	}
	value := strings.TrimSpace(os.Getenv(tokenEnvVar))
	if value == "" {
		return fmt.Errorf("missing %s environment variable", tokenEnvVar)
	}
	app.Token = value
	app.APIKey = value
	app.TokenExplicit = true
	app.APIKeyExplicit = true
	return nil
}

func explicitMCPWorkspaceID(cmd *cobra.Command, app *App, localWorkspaceID string) (string, error) {
	if trimmed := strings.TrimSpace(localWorkspaceID); trimmed != "" {
		return trimmed, nil
	}
	if flagExplicit(cmd, "workspace") || strings.TrimSpace(os.Getenv("BREYTA_WORKSPACE")) != "" {
		if trimmed := strings.TrimSpace(app.WorkspaceID); trimmed != "" {
			return trimmed, nil
		}
	}
	return "", errors.New("missing --workspace-id (MCP server entries must be bound to one explicit workspace id)")
}

func runMCPStdioProxy(ctx context.Context, opts mcpProxyOptions) error {
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}
	scanner := bufio.NewScanner(opts.In)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	writer := bufio.NewWriter(opts.Out)
	defer writer.Flush()
	httpSession := &mcpHTTPSession{}

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		id, idPresent := jsonRPCIDFromBytes(line)
		if !json.Valid(line) {
			if err := writeMCPJSONRPCError(writer, nil, -32700, "Parse error", nil); err != nil {
				return err
			}
			continue
		}
		resp, status, err := postWorkspaceMCP(ctx, opts, httpSession, line)
		if err != nil {
			if idPresent {
				if writeErr := writeMCPJSONRPCError(writer, id, -32000, "Breyta MCP upstream request failed", map[string]any{"error": sanitizeMCPError(err.Error(), opts.Token)}); writeErr != nil {
					return writeErr
				}
			} else {
				fmt.Fprintf(opts.Err, "breyta mcp stdio: upstream request failed: %s\n", sanitizeMCPError(err.Error(), opts.Token))
			}
			continue
		}
		resp = bytes.TrimSpace(resp)
		if status == http.StatusAccepted && len(resp) == 0 {
			continue
		}
		if status < 200 || status > 299 {
			if idPresent {
				msg := upstreamMCPErrorMessage(resp)
				if msg == "" {
					msg = fmt.Sprintf("Breyta MCP upstream returned HTTP %d", status)
				}
				msg = sanitizeMCPError(msg, opts.Token)
				if err := writeMCPJSONRPCError(writer, id, -32000, msg, map[string]any{"status": status}); err != nil {
					return err
				}
			} else {
				msg := sanitizedMCPUpstreamMessage(resp, opts.Token, fmt.Sprintf("Breyta MCP upstream returned HTTP %d", status))
				fmt.Fprintf(opts.Err, "breyta mcp stdio: upstream notification failed (HTTP %d): %s\n", status, msg)
			}
			continue
		}
		if len(resp) == 0 {
			continue
		}
		if _, err := writer.Write(resp); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read MCP stdio: %w", err)
	}
	return nil
}

func runMCPDoctor(ctx context.Context, opts mcpProxyOptions) (map[string]any, error) {
	httpSession := &mcpHTTPSession{}
	initialize := []byte(`{"jsonrpc":"2.0","id":"breyta-mcp-doctor-init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"breyta-cli","version":"doctor"}}}`)
	initResp, status, err := postWorkspaceMCP(ctx, opts, httpSession, initialize)
	if err != nil {
		return map[string]any{"stage": "initialize"}, err
	}
	if status < 200 || status > 299 {
		msg := sanitizedMCPUpstreamMessage(initResp, opts.Token, fmt.Sprintf("HTTP %d", status))
		return map[string]any{"stage": "initialize", "status": status}, fmt.Errorf("initialize failed: %s", msg)
	}
	initMap := map[string]any{}
	_ = json.Unmarshal(initResp, &initMap)
	if errMap := mapStringAny(initMap["error"]); errMap != nil {
		msg := sanitizeMCPError(firstNonBlankString(errMap["message"]), opts.Token)
		return map[string]any{"stage": "initialize", "response": redactMCPValueWithSecrets(initMap, opts.Token)}, fmt.Errorf("initialize failed: %s", msg)
	}

	initializedReq := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	initializedResp, status, err := postWorkspaceMCP(ctx, opts, httpSession, initializedReq)
	if err != nil {
		return map[string]any{"stage": "notifications/initialized"}, err
	}
	if status < 200 || status > 299 {
		msg := sanitizedMCPUpstreamMessage(initializedResp, opts.Token, fmt.Sprintf("HTTP %d", status))
		return map[string]any{"stage": "notifications/initialized", "status": status}, fmt.Errorf("notifications/initialized failed: %s", msg)
	}

	toolsReq := []byte(`{"jsonrpc":"2.0","id":"breyta-mcp-doctor-tools","method":"tools/list","params":{}}`)
	toolsResp, status, err := postWorkspaceMCP(ctx, opts, httpSession, toolsReq)
	if err != nil {
		return map[string]any{"stage": "tools/list"}, err
	}
	if status < 200 || status > 299 {
		msg := sanitizedMCPUpstreamMessage(toolsResp, opts.Token, fmt.Sprintf("HTTP %d", status))
		return map[string]any{"stage": "tools/list", "status": status}, fmt.Errorf("tools/list failed: %s", msg)
	}
	toolsMap := map[string]any{}
	_ = json.Unmarshal(toolsResp, &toolsMap)
	if errMap := mapStringAny(toolsMap["error"]); errMap != nil {
		msg := sanitizeMCPError(firstNonBlankString(errMap["message"]), opts.Token)
		return map[string]any{"stage": "tools/list", "response": redactMCPValueWithSecrets(toolsMap, opts.Token)}, fmt.Errorf("tools/list failed: %s", msg)
	}
	toolNames := []string{}
	for _, item := range sliceAny(mapStringAny(toolsMap["result"])["tools"]) {
		if name := firstNonBlankString(mapStringAny(item)["name"]); name != "" {
			toolNames = append(toolNames, name)
		}
	}
	return map[string]any{
		"workspaceId":     opts.WorkspaceID,
		"apiUrl":          strings.TrimRight(strings.TrimSpace(opts.APIURL), "/"),
		"endpoint":        workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID),
		"transport":       "streamable-http",
		"protocolVersion": firstNonBlankString(mapStringAny(mapStringAny(initMap["result"])["serverInfo"])["protocolVersion"], mapStringAny(initMap["result"])["protocolVersion"]),
		"toolCount":       len(toolNames),
		"tools":           toolNames,
		"policy":          mcpPolicySummary(opts.Policy),
	}, nil
}

func postWorkspaceMCP(ctx context.Context, opts mcpProxyOptions, session *mcpHTTPSession, body []byte) ([]byte, int, error) {
	if strings.TrimSpace(opts.WorkspaceID) == "" {
		return nil, 0, errors.New("missing workspace id")
	}
	if strings.TrimSpace(opts.APIURL) == "" {
		return nil, 0, errors.New("missing api url")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return nil, 0, errors.New("missing API key or token")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultMCPTimeout
	}
	client := opts.HTTP
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opts.Token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if session != nil && strings.TrimSpace(session.SessionID) != "" {
		req.Header.Set("Mcp-Session-Id", strings.TrimSpace(session.SessionID))
	}
	for k, v := range mcpStandardHTTPHeaders(body, session) {
		req.Header.Set(k, v)
	}
	for k, v := range mcpPolicyHeaders(opts.Policy) {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if session != nil {
		if sessionID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sessionID != "" {
			session.SessionID = sessionID
		}
	}
	respBody, err = decodeMCPHTTPResponseBody(resp.Header.Get("Content-Type"), respBody)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if session != nil && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		if protocolVersion := mcpProtocolVersionFromResponse(respBody); protocolVersion != "" {
			session.ProtocolVersion = protocolVersion
		}
	}
	return respBody, resp.StatusCode, nil
}

func decodeMCPHTTPResponseBody(contentType string, body []byte) ([]byte, error) {
	if !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return body, nil
	}
	return decodeMCPSSEData(body)
}

func decodeMCPSSEData(body []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var payloads [][]byte
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		if payload != "" {
			payloads = append(payloads, []byte(payload))
		}
		dataLines = nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.HasPrefix(value, " ") {
			value = strings.TrimPrefix(value, " ")
		}
		if field == "data" {
			dataLines = append(dataLines, value)
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse MCP SSE response: %w", err)
	}
	if len(payloads) == 0 {
		return nil, nil
	}
	return bytes.Join(payloads, []byte("\n")), nil
}

func mcpStandardHTTPHeaders(body []byte, session *mcpHTTPSession) map[string]string {
	var message map[string]any
	if err := json.Unmarshal(body, &message); err != nil {
		return nil
	}
	params := mapStringAny(message["params"])
	meta := mapStringAny(params["_meta"])
	headers := map[string]string{}
	method := firstNonBlankString(message["method"])
	if method != "" {
		headers["Mcp-Method"] = mcpHeaderValue(method)
	}
	protocolVersion := firstNonBlankString(
		meta["io.modelcontextprotocol/protocolVersion"],
		meta["protocolVersion"],
		params["protocolVersion"],
		params["protocol-version"],
	)
	if protocolVersion == "" && session != nil {
		protocolVersion = strings.TrimSpace(session.ProtocolVersion)
	}
	if protocolVersion != "" {
		if session != nil {
			session.ProtocolVersion = strings.TrimSpace(protocolVersion)
		}
		headers["MCP-Protocol-Version"] = mcpHeaderValue(protocolVersion)
	}
	var name string
	switch method {
	case "tools/call", "prompts/get":
		name = firstNonBlankString(params["name"])
	case "resources/read":
		name = firstNonBlankString(params["uri"])
	}
	if name != "" {
		headers["Mcp-Name"] = mcpHeaderValue(name)
	}
	return headers
}

func mcpProtocolVersionFromResponse(body []byte) string {
	var message map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(body), &message); err != nil {
		return ""
	}
	result := mapStringAny(message["result"])
	return firstNonBlankString(result["protocolVersion"], mapStringAny(result["serverInfo"])["protocolVersion"])
}

func mcpHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	safe := true
	for _, r := range value {
		if r == '\t' || r == ' ' || (r >= 0x21 && r <= 0x7e) {
			continue
		}
		safe = false
		break
	}
	if safe {
		return value
	}
	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

func workspaceMCPEndpoint(apiURL string, workspaceID string) string {
	base := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	return base + "/api/workspaces/" + url.PathEscape(strings.TrimSpace(workspaceID)) + "/mcp"
}

func mcpPolicyHeaders(policy mcpPolicyOptions) map[string]string {
	headers := map[string]string{}
	if strings.TrimSpace(policy.Toolsets) != "" {
		headers["X-MCP-Toolsets"] = strings.TrimSpace(policy.Toolsets)
	}
	if strings.TrimSpace(policy.Tools) != "" {
		headers["X-MCP-Tools"] = strings.TrimSpace(policy.Tools)
	}
	if strings.TrimSpace(policy.ExcludeTools) != "" {
		headers["X-MCP-Exclude-Tools"] = strings.TrimSpace(policy.ExcludeTools)
	}
	if policy.ReadOnly {
		headers["X-MCP-Readonly"] = "true"
	}
	return headers
}

func mcpPolicySummary(policy mcpPolicyOptions) map[string]any {
	return map[string]any{
		"toolsets":     strings.TrimSpace(policy.Toolsets),
		"tools":        strings.TrimSpace(policy.Tools),
		"excludeTools": strings.TrimSpace(policy.ExcludeTools),
		"readOnly":     policy.ReadOnly,
	}
}

func jsonRPCIDFromBytes(raw []byte) (any, bool) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, false
	}
	rawID, ok := envelope["id"]
	if !ok {
		return nil, false
	}
	var id any
	if err := json.Unmarshal(rawID, &id); err != nil {
		return nil, true
	}
	return id, true
}

func writeMCPJSONRPCError(w *bufio.Writer, id any, code int, message string, data any) error {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	if data != nil {
		payload["error"].(map[string]any)["data"] = redactMCPValue(data)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}

func upstreamMCPErrorMessage(body []byte) string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return strings.TrimSpace(string(body))
	}
	if msg := firstNonBlankString(mapStringAny(m["error"])["message"], m["message"], m["details"], m["hint"]); msg != "" {
		return msg
	}
	if msg := firstNonBlankString(m["error"]); msg != "" {
		return msg
	}
	return ""
}

func sanitizedMCPUpstreamMessage(body []byte, token string, fallback string) string {
	message := upstreamMCPErrorMessage(body)
	if strings.TrimSpace(message) == "" {
		message = fallback
	}
	return sanitizeMCPError(message, token)
}

func sanitizeMCPError(message string, extraSecrets ...string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "unknown error"
	}
	secrets := []string{os.Getenv("BREYTA_API_KEY"), os.Getenv("BREYTA_TOKEN"), os.Getenv(defaultMCPTokenEnvVar)}
	secrets = append(secrets, extraSecrets...)
	for _, key := range secrets {
		if strings.TrimSpace(key) != "" {
			message = strings.ReplaceAll(message, key, "[redacted]")
		}
	}
	return message
}

func redactMCPValue(v any) any {
	return redactMCPValueWithSecrets(v)
}

func redactMCPValueWithSecrets(v any, extraSecrets ...string) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "key") || strings.Contains(lk, "authorization") {
				out[k] = "[redacted]"
			} else {
				out[k] = redactMCPValueWithSecrets(val, extraSecrets...)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, redactMCPValueWithSecrets(item, extraSecrets...))
		}
		return out
	case string:
		return sanitizeMCPError(x, extraSecrets...)
	default:
		return v
	}
}

func renderMCPClientConfig(opts mcpSetupOptions) (string, error) {
	opts.Provider = strings.ToLower(strings.TrimSpace(opts.Provider))
	if opts.Provider == "" {
		opts.Provider = "generic"
	}
	opts.Transport = strings.ToLower(strings.TrimSpace(opts.Transport))
	if opts.Transport == "" || opts.Transport == "auto" {
		opts.Transport = "stdio"
	}
	if opts.Transport != "stdio" && opts.Transport != "http" {
		return "", fmt.Errorf("invalid --transport %q (expected stdio or http)", opts.Transport)
	}
	opts.ServerName = strings.TrimSpace(opts.ServerName)
	if opts.ServerName == "" {
		opts.ServerName = defaultMCPServerName
	}
	opts.TokenEnvVar = strings.TrimSpace(opts.TokenEnvVar)
	if opts.TokenEnvVar == "" {
		opts.TokenEnvVar = defaultMCPTokenEnvVar
	}
	opts.APIURL = strings.TrimRight(strings.TrimSpace(opts.APIURL), "/")
	if opts.APIURL == "" {
		opts.APIURL = "https://flows.breyta.ai"
	}
	if strings.TrimSpace(opts.WorkspaceID) == "" {
		return "", errors.New("missing workspace id")
	}

	switch opts.Provider {
	case "codex":
		return renderCodexMCPConfig(opts), nil
	case "opencode":
		return renderOpenCodeMCPConfig(opts)
	case "continue":
		return renderContinueMCPConfig(opts), nil
	case "goose":
		return renderGooseMCPConfig(opts), nil
	case "zed":
		return renderZedMCPConfig(opts)
	case "vscode":
		return renderVSCodeMCPConfig(opts)
	case "generic", "claude", "cursor", "windsurf", "cline", "roo", "gemini", "acp":
		return renderGenericMCPJSON(opts)
	default:
		return "", fmt.Errorf("unsupported --provider %q", opts.Provider)
	}
}

func stdioMCPArgs(opts mcpSetupOptions) []string {
	args := []string{"mcp", "stdio", "--workspace-id", opts.WorkspaceID}
	if strings.TrimSpace(opts.TokenEnvVar) != "" {
		args = append(args, "--token-env-var", strings.TrimSpace(opts.TokenEnvVar))
	}
	if opts.Policy.ReadOnly {
		args = append(args, "--read-only")
	}
	if strings.TrimSpace(opts.Policy.Toolsets) != "" {
		args = append(args, "--toolsets", strings.TrimSpace(opts.Policy.Toolsets))
	}
	if strings.TrimSpace(opts.Policy.Tools) != "" {
		args = append(args, "--tools", strings.TrimSpace(opts.Policy.Tools))
	}
	if strings.TrimSpace(opts.Policy.ExcludeTools) != "" {
		args = append(args, "--exclude-tools", strings.TrimSpace(opts.Policy.ExcludeTools))
	}
	return args
}

func renderCodexMCPConfig(opts mcpSetupOptions) string {
	name := sanitizeConfigName(opts.ServerName)
	if opts.Transport == "http" {
		var b strings.Builder
		fmt.Fprintf(&b, `
[mcp_servers.%s]
url = %q
bearer_token_env_var = %q`, name, workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID), opts.TokenEnvVar)
		if headers := mcpPolicyHeaders(opts.Policy); len(headers) > 0 {
			fmt.Fprintf(&b, "\n\n[mcp_servers.%s.http_headers]", name)
			for _, key := range orderedMCPPolicyHeaderKeys() {
				if value, ok := headers[key]; ok {
					fmt.Fprintf(&b, "\n%s = %q", strconv.Quote(key), value)
				}
			}
		}
		return strings.TrimSpace(b.String())
	}
	args := stdioMCPArgs(opts)
	quotedArgs := make([]string, 0, len(args))
	for _, arg := range args {
		quotedArgs = append(quotedArgs, strconv.Quote(arg))
	}
	return strings.TrimSpace(fmt.Sprintf(`
[mcp_servers.%s]
command = "breyta"
args = [%s]
env = { BREYTA_API_URL = %q }
env_vars = [%q]`, name, strings.Join(quotedArgs, ", "), opts.APIURL, opts.TokenEnvVar))
}

func renderGenericMCPJSON(opts mcpSetupOptions) (string, error) {
	var server map[string]any
	if opts.Transport == "http" {
		server = map[string]any{
			"type": "http",
			"url":  workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID),
			"headers": mergeStringMap(map[string]string{
				"Authorization": "Bearer ${env:" + opts.TokenEnvVar + "}",
			}, mcpPolicyHeaders(opts.Policy)),
		}
	} else {
		server = map[string]any{
			"type":    "stdio",
			"command": "breyta",
			"args":    stdioMCPArgs(opts),
			"env": map[string]string{
				"BREYTA_API_URL": opts.APIURL,
				opts.TokenEnvVar: "${env:" + opts.TokenEnvVar + "}",
			},
		}
	}
	return marshalPretty(map[string]any{"mcpServers": map[string]any{opts.ServerName: server}})
}

func renderOpenCodeMCPConfig(opts mcpSetupOptions) (string, error) {
	var server map[string]any
	if opts.Transport == "http" {
		server = map[string]any{
			"type":    "remote",
			"url":     workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID),
			"enabled": true,
			"oauth":   false,
			"headers": mergeStringMap(map[string]string{
				"Authorization": "Bearer {env:" + opts.TokenEnvVar + "}",
			}, mcpPolicyHeaders(opts.Policy)),
		}
	} else {
		server = map[string]any{
			"type":    "local",
			"command": append([]string{"breyta"}, stdioMCPArgs(opts)...),
			"enabled": true,
			"environment": map[string]string{
				"BREYTA_API_URL": opts.APIURL,
				opts.TokenEnvVar: "{env:" + opts.TokenEnvVar + "}",
			},
		}
	}
	return marshalPretty(map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp":     map[string]any{opts.ServerName: server},
	})
}

func renderContinueMCPConfig(opts mcpSetupOptions) string {
	if opts.Transport == "http" {
		return strings.TrimSpace(fmt.Sprintf(`
name: Breyta MCP
version: 0.0.1
schema: v1
mcpServers:
  - name: %s
    type: streamable-http
    url: %s
    requestOptions:
      headers:
        Authorization: %s%s`, yamlScalar(opts.ServerName), yamlScalar(workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID)), yamlScalar("Bearer ${env:"+opts.TokenEnvVar+"}"), yamlPolicyHeaders(opts.Policy, "        ")))
	}
	return strings.TrimSpace(fmt.Sprintf(`
name: Breyta MCP
version: 0.0.1
schema: v1
mcpServers:
  - name: %s
    command: breyta
    args: %s
    env:
      BREYTA_API_URL: %s
      %s: %s`, yamlScalar(opts.ServerName), yamlStringArray(stdioMCPArgs(opts)), yamlScalar(opts.APIURL), yamlScalar(opts.TokenEnvVar), yamlScalar("${env:"+opts.TokenEnvVar+"}")))
}

func renderGooseMCPConfig(opts mcpSetupOptions) string {
	if opts.Transport == "http" {
		return strings.TrimSpace(fmt.Sprintf(`Use goose configure:
- Add Extension
- Remote Extension (Streamable HTTP)
- URL: %s
- Header: Authorization: Bearer <%s>
- Timeout: 300`, workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID), opts.TokenEnvVar))
	}
	return strings.TrimSpace(fmt.Sprintf(`Use goose configure:
- Add Extension
- Command-line Extension
- Command: breyta
- Arguments: %s
- Environment: BREYTA_API_URL=%s, %s=<service-account-api-key>`, strings.Join(stdioMCPArgs(opts), " "), opts.APIURL, opts.TokenEnvVar))
}

func renderZedMCPConfig(opts mcpSetupOptions) (string, error) {
	if opts.Transport == "http" {
		return marshalPretty(map[string]any{"context_servers": map[string]any{opts.ServerName: map[string]any{
			"url": workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID),
			"headers": mergeStringMap(map[string]string{
				"Authorization": "Bearer ${env:" + opts.TokenEnvVar + "}",
			}, mcpPolicyHeaders(opts.Policy)),
		}}})
	}
	return marshalPretty(map[string]any{"context_servers": map[string]any{opts.ServerName: map[string]any{
		"command": "breyta",
		"args":    stdioMCPArgs(opts),
		"env": map[string]string{
			"BREYTA_API_URL": opts.APIURL,
			opts.TokenEnvVar: "${env:" + opts.TokenEnvVar + "}",
		},
	}}})
}

func renderVSCodeMCPConfig(opts mcpSetupOptions) (string, error) {
	inputID := strings.ToLower(strings.ReplaceAll(opts.TokenEnvVar, "_", "-"))
	if opts.Transport == "http" {
		return marshalPretty(map[string]any{
			"servers": map[string]any{opts.ServerName: map[string]any{
				"type": "http",
				"url":  workspaceMCPEndpoint(opts.APIURL, opts.WorkspaceID),
				"headers": mergeStringMap(map[string]string{
					"Authorization": "Bearer ${input:" + inputID + "}",
				}, mcpPolicyHeaders(opts.Policy)),
			}},
			"inputs": []map[string]any{{
				"type":        "promptString",
				"id":          inputID,
				"description": "Breyta MCP service-account API key",
				"password":    true,
			}},
		})
	}
	return marshalPretty(map[string]any{
		"servers": map[string]any{opts.ServerName: map[string]any{
			"type":    "stdio",
			"command": "breyta",
			"args":    stdioMCPArgs(opts),
			"env": map[string]string{
				"BREYTA_API_URL": opts.APIURL,
				opts.TokenEnvVar: "${input:" + inputID + "}",
			},
		}},
		"inputs": []map[string]any{{
			"type":        "promptString",
			"id":          inputID,
			"description": "Breyta MCP service-account API key",
			"password":    true,
		}},
	})
}

func marshalPretty(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func mergeStringMap(base map[string]string, extra map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func sanitizeConfigName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultMCPServerName
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	clean := strings.Trim(b.String(), "_")
	if clean == "" {
		return defaultMCPServerName
	}
	return clean
}

func yamlStringArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func yamlScalar(value string) string {
	return strconv.Quote(value)
}

func orderedMCPPolicyHeaderKeys() []string {
	return []string{"X-MCP-Readonly", "X-MCP-Toolsets", "X-MCP-Tools", "X-MCP-Exclude-Tools"}
}

func yamlPolicyHeaders(policy mcpPolicyOptions, indent string) string {
	headers := mcpPolicyHeaders(policy)
	if len(headers) == 0 {
		return ""
	}
	var b strings.Builder
	for _, key := range orderedMCPPolicyHeaderKeys() {
		if value, ok := headers[key]; ok {
			fmt.Fprintf(&b, "\n%s%s: %s", indent, yamlScalar(key), yamlScalar(value))
		}
	}
	return b.String()
}
