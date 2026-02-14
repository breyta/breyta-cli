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

	data, _ := envelope["data"].(map[string]any)
	if data == nil {
		return
	}

	enrichDataWebLinks(base, data)
	if webURL, _ := data["webUrl"].(string); strings.TrimSpace(webURL) != "" {
		meta := ensureMeta(envelope)
		if _, exists := meta["webUrl"]; !exists {
			meta["webUrl"] = strings.TrimSpace(webURL)
		}
	}
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
	return base + "/" + url.PathEscape(workspaceID)
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
			enrichFlowWebLinks(base, item)
			enrichInstallationWebLinks(base, item, parentFlowSlug)
			enrichConnectionWebLinks(base, item)
		}
	}

	if resourceURL := inferResourceRunURL(base, data, parentFlowSlug); resourceURL != "" {
		setIfMissing(data, "webUrl", resourceURL)
	}

	if primary := inferPrimaryDataWebURL(base, data, parentFlowSlug); primary != "" {
		setIfMissing(data, "webUrl", primary)
	}
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
	if u := flowWebURL(base, parentFlowSlug); u != "" {
		return u
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
	if u, _ := first["webUrl"].(string); strings.TrimSpace(u) != "" {
		return strings.TrimSpace(u)
	}

	if extractRunID(first) != "" && extractFlowSlug(first) != "" {
		if parentFlowSlug != "" {
			return flowRunsWebURL(base, parentFlowSlug)
		}
		return runsWebURL(base)
	}
	if extractFlowSlug(first) != "" {
		if parentFlowSlug != "" {
			return flowWebURL(base, parentFlowSlug)
		}
		return flowsWebURL(base)
	}
	if extractConnectionID(first) != "" {
		return connectionsWebURL(base)
	}
	if extractProfileID(first) != "" {
		if parentFlowSlug != "" {
			return flowInstallationsWebURL(base, parentFlowSlug)
		}
		return installationsWebURL(base)
	}
	return ""
}

func enrichRunWebLinks(base string, m map[string]any) {
	flowSlug := extractFlowSlug(m)
	runID := extractRunID(m)
	if flowSlug == "" || runID == "" {
		return
	}
	setIfMissing(m, "webUrl", runWebURL(base, flowSlug, runID))
	setIfMissing(m, "outputWebUrl", runOutputWebURL(base, flowSlug, runID))
}

func enrichFlowWebLinks(base string, m map[string]any) {
	flowSlug := extractFlowSlug(m)
	if flowSlug == "" {
		return
	}
	setIfMissing(m, "webUrl", flowWebURL(base, flowSlug))
}

func enrichInstallationWebLinks(base string, m map[string]any, parentFlowSlug string) {
	profileID := extractProfileID(m)
	flowSlug := coalesceNonBlank(extractFlowSlug(m), parentFlowSlug)
	if profileID == "" {
		return
	}
	if u := installationWebURL(base, flowSlug, profileID); u != "" {
		setIfMissing(m, "webUrl", u)
	}
}

func enrichConnectionWebLinks(base string, m map[string]any) {
	connID := extractConnectionID(m)
	if connID == "" {
		return
	}
	setIfMissing(m, "webUrl", connectionEditWebURL(base, connID))
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
	prefix := "/result/run/"
	i := strings.Index(resourceURI, prefix)
	if i < 0 {
		return "", "", ""
	}
	tail := strings.TrimSpace(resourceURI[i+len(prefix):])
	if tail == "" {
		return "", "", ""
	}
	parts := strings.Split(tail, "/")
	if len(parts) < 2 {
		return "", "", ""
	}
	workflowID = strings.TrimSpace(parts[0])
	if workflowID == "" {
		return "", "", ""
	}
	if parts[1] == "step" && len(parts) >= 4 {
		decodedStepID, err := url.QueryUnescape(parts[2])
		if err != nil {
			decodedStepID = parts[2]
		}
		return workflowID, strings.TrimSpace(decodedStepID), strings.TrimSpace(parts[3])
	}
	if parts[1] == "flow-output" || parts[1] == "flow-error" {
		return workflowID, "", strings.TrimSpace(parts[1])
	}
	return workflowID, "", ""
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

func webURL(base string, segments ...string) string {
	if strings.TrimSpace(base) == "" {
		return ""
	}
	out := strings.TrimRight(base, "/")
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return ""
		}
		out += "/" + url.PathEscape(segment)
	}
	return out
}

func flowsWebURL(base string) string {
	return webURL(base, "flows")
}

func flowWebURL(base, flowSlug string) string {
	return webURL(base, "flows", flowSlug)
}

func flowRunsWebURL(base, flowSlug string) string {
	return webURL(base, "flows", flowSlug, "runs")
}

func flowInstallationsWebURL(base, flowSlug string) string {
	return webURL(base, "flows", flowSlug, "installations")
}

func runsWebURL(base string) string {
	return webURL(base, "runs")
}

func runWebURL(base, flowSlug, runID string) string {
	return webURL(base, "runs", flowSlug, runID)
}

func runOutputWebURL(base, flowSlug, runID string) string {
	return webURL(base, "runs", flowSlug, runID, "output")
}

func runStepWebURL(base, flowSlug, runID, stepID string) string {
	return webURL(base, "runs", flowSlug, runID, "steps", stepID)
}

func installationsWebURL(base string) string {
	return webURL(base, "installations")
}

func installationWebURL(base, flowSlug, profileID string) string {
	if strings.TrimSpace(profileID) == "" {
		return ""
	}
	if strings.TrimSpace(flowSlug) != "" {
		return webURL(base, "flows", flowSlug, "installations", profileID)
	}
	return webURL(base, "installations", profileID)
}

func connectionsWebURL(base string) string {
	return webURL(base, "connections")
}

func connectionEditWebURL(base, connectionID string) string {
	return webURL(base, "connections", connectionID, "edit")
}
