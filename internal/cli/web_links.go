package cli

import (
	"net/url"
	"strings"
)

func enrichEnvelopeWebLinks(app *App, envelope map[string]any) {
	base := workspaceWebBaseURL(app)
	if base == "" || envelope == nil {
		return
	}
	root := strings.TrimRight(strings.TrimSpace(app.APIURL), "/")

	data, _ := envelope["data"].(map[string]any)
	if data == nil {
		return
	}
	if root != "" {
		absolutizeKnownWebLinks(root, data)
	}

	enrichDataWebLinks(base, data)
	if webURL, _ := data["webUrl"].(string); strings.TrimSpace(webURL) != "" {
		meta := ensureMeta(envelope)
		if _, exists := meta["webUrl"]; !exists {
			meta["webUrl"] = strings.TrimSpace(webURL)
		}
	}
}

func absolutizeKnownWebLinks(baseRoot string, data map[string]any) {
	if strings.TrimSpace(baseRoot) == "" || data == nil {
		return
	}
	absolutizeKnownObjectWebLinks(baseRoot, data)

	if run, _ := data["run"].(map[string]any); run != nil {
		absolutizeKnownObjectWebLinks(baseRoot, run)
	}
	if flow, _ := data["flow"].(map[string]any); flow != nil {
		absolutizeKnownObjectWebLinks(baseRoot, flow)
	}
	if inst, _ := data["instance"].(map[string]any); inst != nil {
		absolutizeKnownObjectWebLinks(baseRoot, inst)
	}
	if inst, _ := data["installation"].(map[string]any); inst != nil {
		absolutizeKnownObjectWebLinks(baseRoot, inst)
	}
	if conn, _ := data["connection"].(map[string]any); conn != nil {
		absolutizeKnownObjectWebLinks(baseRoot, conn)
	}
	if items, _ := data["items"].([]any); len(items) > 0 {
		for _, itemAny := range items {
			item, _ := itemAny.(map[string]any)
			if item == nil {
				continue
			}
			absolutizeKnownObjectWebLinks(baseRoot, item)
		}
	}
}

func absolutizeKnownObjectWebLinks(baseRoot string, m map[string]any) {
	if !isKnownWebLinkObject(m) {
		return
	}
	if s, ok := m["webUrl"].(string); ok {
		if abs := absolutizeWebURL(baseRoot, s); abs != "" {
			m["webUrl"] = abs
		}
	}
	if s, ok := m["outputWebUrl"].(string); ok {
		if abs := absolutizeWebURL(baseRoot, s); abs != "" {
			m["outputWebUrl"] = abs
		}
	}
}

func isKnownWebLinkObject(m map[string]any) bool {
	if m == nil {
		return false
	}
	if strings.HasPrefix(asString(m, "uri"), "res://") {
		return true
	}
	if extractRunID(m) != "" && extractFlowSlug(m) != "" {
		return true
	}
	if extractConnectionID(m) != "" {
		return true
	}
	if extractProfileID(m) != "" {
		return true
	}
	if looksLikeFlowObject(m) && extractFlowSlug(m) != "" {
		return true
	}
	if _, ok := m["run"]; ok {
		return true
	}
	if _, ok := m["flow"]; ok {
		return true
	}
	if _, ok := m["connection"]; ok {
		return true
	}
	if _, ok := m["instance"]; ok {
		return true
	}
	if _, ok := m["installation"]; ok {
		return true
	}
	if _, ok := m["items"]; ok {
		return true
	}
	return false
}

func absolutizeWebURL(baseRoot, value string) string {
	baseRoot = strings.TrimRight(strings.TrimSpace(baseRoot), "/")
	value = strings.TrimSpace(value)
	if baseRoot == "" || value == "" {
		return value
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.HasPrefix(value, "/") {
		return baseRoot + value
	}
	return baseRoot + "/" + strings.TrimLeft(value, "/")
}

func workspaceWebBaseURL(app *App) string {
	if app == nil || !isAPIMode(app) {
		return ""
	}
	workspaceID := strings.TrimSpace(app.WorkspaceID)
	if workspaceID == "" {
		return ""
	}
	ensureAPIURL(app)
	base := strings.TrimRight(strings.TrimSpace(app.APIURL), "/")
	if base == "" {
		return ""
	}
	return base + "/ui?workspace=" + url.QueryEscape(workspaceID)
}

func enrichDataWebLinks(base string, data map[string]any) {
	if base == "" || data == nil {
		return
	}

	parentFlowSlug := extractFlowSlug(data)

	if run, _ := data["run"].(map[string]any); run != nil {
		enrichRunWebLinks(base, run)
	}
	if flow, _ := data["flow"].(map[string]any); flow != nil {
		enrichFlowWebLinks(base, flow)
	}
	if inst, _ := data["instance"].(map[string]any); inst != nil {
		enrichInstallationWebLinks(base, inst, parentFlowSlug)
	}
	if inst, _ := data["installation"].(map[string]any); inst != nil {
		enrichInstallationWebLinks(base, inst, parentFlowSlug)
	}
	if conn, _ := data["connection"].(map[string]any); conn != nil {
		enrichConnectionWebLinks(base, conn)
	}

	if items, _ := data["items"].([]any); len(items) > 0 {
		for _, itemAny := range items {
			item, _ := itemAny.(map[string]any)
			if item == nil {
				continue
			}
			enrichRunWebLinks(base, item)
			enrichInstallationWebLinks(base, item, parentFlowSlug)
			enrichFlowWebLinks(base, item)
			enrichConnectionWebLinks(base, item)
			normalizeResourceWebURL(base, item, parentFlowSlug)
		}
	}

	if resourceURL := inferResourceRunURL(base, data, parentFlowSlug); resourceURL != "" {
		setIfMissing(data, "webUrl", resourceURL)
	}

	if primary := inferPrimaryDataWebURL(base, data, parentFlowSlug); primary != "" {
		setIfMissing(data, "webUrl", primary)
	}

	// Resource links must point at retained engine pages, even when an older
	// server response contains a hosted-product URL. Runs last so it wins over
	// the pass-through webUrl absolutized earlier.
	normalizeResourceWebURL(base, data, parentFlowSlug)
}

func normalizeResourceWebURL(base string, m map[string]any, parentFlowSlug string) {
	if m == nil {
		return
	}
	resourceURI := coalesceNonBlank(asString(m, "uri"), asString(m, "resourceUri"), asString(m, "resource-uri"))
	if !strings.HasPrefix(resourceURI, "res://") {
		return
	}
	workflowID, _, _ := parseRunResourceURI(resourceURI)
	if workflowID != "" {
		m["webUrl"] = runWebURL(base, coalesceNonBlank(parentFlowSlug, extractFlowSlug(m), asString(m, "flowSlug")), workflowID)
		return
	}
	page := "resources"
	parts := resourcePathParts(resourceURI)
	if len(parts) >= 2 && parts[0] == "result" && parts[1] == "table" {
		page = "tables"
	} else if len(parts) >= 1 && parts[0] == "file" {
		page = "files"
	}
	m["webUrl"] = engineUIURL(base, page, "", "")
}

func inferPrimaryDataWebURL(base string, data map[string]any, parentFlowSlug string) string {
	if run, _ := data["run"].(map[string]any); run != nil {
		if u := runWebURL(base, extractFlowSlug(run), extractRunID(run)); u != "" {
			return u
		}
	}
	if flow, _ := data["flow"].(map[string]any); flow != nil {
		if u := flowWebURL(base, extractFlowSlug(flow)); u != "" {
			return u
		}
	}
	if inst, _ := data["instance"].(map[string]any); inst != nil {
		if u := installationWebURL(base, coalesceNonBlank(extractFlowSlug(inst), parentFlowSlug), extractProfileID(inst)); u != "" {
			return u
		}
	}
	if inst, _ := data["installation"].(map[string]any); inst != nil {
		if u := installationWebURL(base, coalesceNonBlank(extractFlowSlug(inst), parentFlowSlug), extractProfileID(inst)); u != "" {
			return u
		}
	}
	if connID := extractConnectionID(data); connID != "" {
		return connectionEditWebURL(base, connID)
	}

	items, _ := data["items"].([]any)
	if len(items) == 0 {
		return ""
	}

	first, _ := items[0].(map[string]any)
	if first == nil {
		return ""
	}
	if extractRunID(first) != "" && extractFlowSlug(first) != "" {
		if parentFlowSlug != "" {
			return flowRunsWebURL(base, parentFlowSlug)
		}
		return runsWebURL(base)
	}
	if extractProfileID(first) != "" {
		if parentFlowSlug != "" {
			return flowInstallationsWebURL(base, parentFlowSlug)
		}
		return installationsWebURL(base)
	}
	if extractConnectionID(first) != "" {
		return connectionsWebURL(base)
	}
	if extractFlowSlug(first) != "" {
		if parentFlowSlug != "" {
			return flowWebURL(base, parentFlowSlug)
		}
		return flowsWebURL(base)
	}
	if u, _ := first["webUrl"].(string); strings.TrimSpace(u) != "" {
		return strings.TrimSpace(u)
	}
	if u := flowWebURL(base, parentFlowSlug); u != "" {
		return u
	}
	return ""
}

func enrichRunWebLinks(base string, m map[string]any) {
	flowSlug := extractFlowSlug(m)
	runID := extractRunID(m)
	if flowSlug == "" || runID == "" {
		return
	}
	m["webUrl"] = runWebURL(base, flowSlug, runID)
	m["outputWebUrl"] = runOutputWebURL(base, flowSlug, runID)
}

func enrichFlowWebLinks(base string, m map[string]any) {
	flowSlug := extractFlowSlug(m)
	if flowSlug == "" {
		return
	}
	m["webUrl"] = flowWebURL(base, flowSlug)
}

func enrichInstallationWebLinks(base string, m map[string]any, parentFlowSlug string) {
	profileID := extractProfileID(m)
	flowSlug := coalesceNonBlank(extractFlowSlug(m), parentFlowSlug)
	if profileID == "" {
		return
	}
	if u := installationWebURL(base, flowSlug, profileID); u != "" {
		m["webUrl"] = u
	}
}

func enrichConnectionWebLinks(base string, m map[string]any) {
	connID := extractConnectionID(m)
	if connID == "" {
		return
	}
	m["webUrl"] = connectionEditWebURL(base, connID)
}

func inferResourceRunURL(base string, data map[string]any, parentFlowSlug string) string {
	uri := asString(data, "uri")
	if uri == "" {
		return ""
	}
	workflowID, stepID, kind := parseRunResourceURI(uri)
	if workflowID == "" {
		return ""
	}
	flowSlug := coalesceNonBlank(parentFlowSlug, extractFlowSlug(data), asString(data, "flowSlug"))
	if flowSlug == "" {
		return ""
	}
	if stepID != "" {
		return runStepWebURL(base, flowSlug, workflowID, stepID)
	}
	if kind == "flow-output" {
		return runOutputWebURL(base, flowSlug, workflowID)
	}
	return runWebURL(base, flowSlug, workflowID)
}

func parseRunResourceURI(resourceURI string) (workflowID string, stepID string, kind string) {
	parts := resourcePathParts(resourceURI)
	if len(parts) < 4 || parts[0] != "result" || parts[1] != "run" {
		return "", "", ""
	}
	workflowID = strings.TrimSpace(parts[2])
	if workflowID == "" {
		return "", "", ""
	}
	if parts[3] == "step" && len(parts) >= 6 {
		return workflowID, strings.TrimSpace(parts[4]), strings.TrimSpace(parts[5])
	}
	if parts[3] == "flow-output" || parts[3] == "flow-error" {
		return workflowID, "", strings.TrimSpace(parts[3])
	}
	return workflowID, "", ""
}

func resourcePathParts(resourceURI string) []string {
	parsed, err := url.Parse(strings.TrimSpace(resourceURI))
	if err != nil || parsed.Scheme != "res" {
		return nil
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "ws" || strings.TrimSpace(parts[1]) == "" {
		return nil
	}
	return parts[2:]
}

func extractRunID(m map[string]any) string {
	if m == nil {
		return ""
	}
	return coalesceNonBlank(
		asString(m, "workflowId"),
		asString(m, "workflow-id"),
		asString(m, "runId"),
	)
}

func extractFlowSlug(m map[string]any) string {
	if m == nil {
		return ""
	}
	if slug := coalesceNonBlank(asString(m, "flowSlug"), asString(m, "flow-slug")); slug != "" {
		return slug
	}
	if flowAny, ok := m["flow"]; ok {
		if flow, _ := flowAny.(map[string]any); flow != nil {
			if slug := coalesceNonBlank(asString(flow, "flowSlug"), asString(flow, "slug")); slug != "" {
				return slug
			}
		}
	}
	if looksLikeFlowObject(m) {
		return asString(m, "slug")
	}
	return ""
}

func looksLikeFlowObject(m map[string]any) bool {
	if m == nil {
		return false
	}
	if _, ok := m["activeVersion"]; ok {
		return true
	}
	if _, ok := m["spine"]; ok {
		return true
	}
	if _, ok := m["versions"]; ok {
		return true
	}
	if _, ok := m["flowLiteral"]; ok {
		return true
	}
	return false
}

func extractProfileID(m map[string]any) string {
	if m == nil {
		return ""
	}
	if profileID := coalesceNonBlank(asString(m, "profileId"), asString(m, "profile-id")); profileID != "" {
		return profileID
	}
	if instance, _ := m["instance"].(map[string]any); instance != nil {
		if profileID := coalesceNonBlank(asString(instance, "profileId"), asString(instance, "profile-id")); profileID != "" {
			return profileID
		}
	}
	if installation, _ := m["installation"].(map[string]any); installation != nil {
		if profileID := coalesceNonBlank(asString(installation, "profileId"), asString(installation, "profile-id")); profileID != "" {
			return profileID
		}
	}
	return ""
}

func extractConnectionID(m map[string]any) string {
	if m == nil {
		return ""
	}
	if connID := coalesceNonBlank(asString(m, "connectionId"), asString(m, "connection-id")); connID != "" {
		return connID
	}
	if conn, _ := m["connection"].(map[string]any); conn != nil {
		if connID := coalesceNonBlank(asString(conn, "id"), asString(conn, "connectionId")); isConnectionID(connID) {
			return connID
		}
	}
	id := asString(m, "id")
	if isConnectionID(id) {
		return id
	}
	return ""
}

func isConnectionID(id string) bool {
	id = strings.TrimSpace(id)
	return strings.HasPrefix(id, "conn-")
}

func asString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func setIfMissing(m map[string]any, key string, value string) {
	if m == nil || strings.TrimSpace(value) == "" {
		return
	}
	if existing, _ := m[key].(string); strings.TrimSpace(existing) != "" {
		return
	}
	m[key] = value
}

func coalesceNonBlank(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func engineUIURL(base, page, selectionKey, selectionValue string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	workspace := strings.TrimSpace(parsed.Query().Get("workspace"))
	page = strings.TrimSpace(page)
	if workspace == "" || page == "" {
		return ""
	}
	query := "workspace=" + url.QueryEscape(workspace) + "&page=" + url.QueryEscape(page)
	if key, value := strings.TrimSpace(selectionKey), strings.TrimSpace(selectionValue); key != "" && value != "" {
		query += "&" + url.QueryEscape(key) + "=" + url.QueryEscape(value)
	}
	parsed.RawQuery = query
	parsed.Fragment = ""
	return parsed.String()
}

func flowsWebURL(base string) string {
	return engineUIURL(base, "flows", "", "")
}

func flowWebURL(base, flowSlug string) string {
	return engineUIURL(base, "flows", "flow", flowSlug)
}

func flowRunsWebURL(base, flowSlug string) string {
	return runsListWebURL(base, runsListFilters{Flow: flowSlug})
}

func flowInstallationsWebURL(base, flowSlug string) string {
	return flowWebURL(base, flowSlug)
}

func runsWebURL(base string) string {
	return engineUIURL(base, "runs", "", "")
}

func runWebURL(base, flowSlug, runID string) string {
	return engineUIURL(base, "runs", "run", runID)
}

func runOutputWebURL(base, flowSlug, runID string) string {
	return runWebURL(base, flowSlug, runID)
}

func runStepWebURL(base, flowSlug, runID, stepID string) string {
	return runWebURL(base, flowSlug, runID)
}

func installationsWebURL(base string) string {
	return flowsWebURL(base)
}

func installationWebURL(base, flowSlug, profileID string) string {
	if strings.TrimSpace(profileID) == "" {
		return ""
	}
	if strings.TrimSpace(flowSlug) != "" {
		return flowWebURL(base, flowSlug)
	}
	return flowsWebURL(base)
}

func connectionsWebURL(base string) string {
	return engineUIURL(base, "connections", "", "")
}

func connectionEditWebURL(base, connectionID string) string {
	return engineUIURL(base, "connections", "connection", connectionID)
}
