package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	compactTemplateDescriptionRunes = 360
	compactTemplateStepsTextRunes   = 640
	compactTemplateSourceRunes      = 900
	compactTemplateStepListLimit    = 5
	compactResourceSnippetRunes     = 360
	compactResourceReadRunes        = 4096
	compactJobSummaryRunes          = 360
	compactDocsDefaultRunes         = 12000
	compactDocsContentsLimit        = 30
)

func compactTemplateSearchEnvelope(out map[string]any) {
	data := mapStringAny(out["data"])
	result := mapStringAny(data["result"])
	hits := sliceAny(result["hits"])
	itemHits := sliceAny(result["items"])
	dataItems := sliceAny(data["items"])
	if len(hits) == 0 && len(itemHits) == 0 && len(dataItems) == 0 {
		return
	}
	changed := false
	for _, hitAny := range hits {
		if compactTemplateSearchHit(mapStringAny(hitAny)) {
			changed = true
		}
	}
	for _, hitAny := range itemHits {
		if compactTemplateSearchHit(mapStringAny(hitAny)) {
			changed = true
		}
	}
	for _, hitAny := range dataItems {
		if compactTemplateSearchHit(mapStringAny(hitAny)) {
			changed = true
		}
	}
	if !changed {
		return
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	meta["outputView"] = "compact"
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = "Template search output is compact by default. Use --full for indexed source previews, or --full --raw-definition only when the complete template definition is required."
	}
}

func compactTemplateSearchHit(hit map[string]any) bool {
	if hit == nil {
		return false
	}
	changed := false

	if s := firstNonBlankString(hit["publish_description"], hit["publishDescription"]); s != "" {
		hit["publishDescriptionPreview"] = truncateRunes(s, compactTemplateDescriptionRunes)
		delete(hit, "publish_description")
		delete(hit, "publishDescription")
		changed = true
	}
	if s := firstNonBlankString(hit["steps_text"], hit["stepsText"]); s != "" {
		hit["stepsTextPreview"] = truncateRunes(s, compactTemplateStepsTextRunes)
		delete(hit, "steps_text")
		delete(hit, "stepsText")
		changed = true
	}
	for _, key := range []string{"rawDefinition", "raw_definition", "definition", "flowLiteral", "flow_literal", "source"} {
		if s := firstNonBlankString(hit[key]); s != "" {
			if _, exists := hit["sourcePreview"]; !exists {
				hit["sourcePreview"] = truncateRunes(s, compactTemplateSourceRunes)
			}
			delete(hit, key)
			changed = true
		}
	}
	if changedSteps := compactStepListField(hit, "step_list"); changedSteps {
		delete(hit, "stepList")
		changed = true
	} else if compactStepListField(hit, "stepList") {
		changed = true
	}
	compacted := map[string]any{}
	setCompactField(compacted, "flow_slug", firstNonBlankString(hit["flow_slug"], hit["flowSlug"], hit["slug"]))
	setCompactField(compacted, "scope", firstNonBlankString(hit["scope"]))
	setCompactField(compacted, "name", firstNonBlankString(hit["name"], hit["title"]))
	setCompactField(compacted, "description", firstNonBlankString(hit["description"]))
	setCompactField(compacted, "tags", firstPresentAny(hit["tags"]))
	setCompactField(compacted, "providers", firstPresentAny(hit["providers"]))
	setCompactField(compacted, "tool_names", firstPresentAny(hit["tool_names"], hit["toolNames"]))
	setCompactField(compacted, "connection_slots", firstPresentAny(hit["connection_slots"], hit["connectionSlots"]))
	setCompactField(compacted, "step_types", firstPresentAny(hit["step_types"], hit["stepTypes"]))
	setCompactField(compacted, "step_count", firstPresentAny(hit["step_count"], hit["stepCount"]))
	setCompactField(compacted, "step_list", firstPresentAny(hit["step_list"], hit["stepList"]))
	setCompactField(compacted, "stepListOmitted", firstPresentAny(hit["stepListOmitted"]))
	setCompactField(compacted, "publishDescriptionPreview", firstNonBlankString(hit["publishDescriptionPreview"]))
	setCompactField(compacted, "stepsTextPreview", firstNonBlankString(hit["stepsTextPreview"]))
	setCompactField(compacted, "sourcePreview", firstNonBlankString(hit["sourcePreview"]))
	setCompactField(compacted, "matchedSurfaces", firstPresentAny(hit["matchedSurfaces"], hit["matched_surfaces"]))
	setCompactField(compacted, "matchedPatterns", firstPresentAny(hit["matchedPatterns"], hit["matched_patterns"]))
	setCompactField(compacted, "matchPreviews", firstPresentAny(hit["matchPreviews"], hit["match_previews"]))
	setCompactField(compacted, "flow_web_url", firstNonBlankString(hit["flow_web_url"], hit["flowWebUrl"], hit["webUrl"]))
	setCompactField(compacted, "workspace_name", firstNonBlankString(hit["workspace_name"], hit["workspaceName"]))
	setCompactField(compacted, "score", firstPresentAny(hit["score"]))
	addFlowSearchHitRefs(compacted)
	if len(compacted) == 0 {
		return changed
	}
	for key := range hit {
		delete(hit, key)
	}
	for key, value := range compacted {
		hit[key] = value
	}
	return true
}

func compactStepListField(hit map[string]any, key string) bool {
	steps := sliceAny(hit[key])
	if len(steps) <= compactTemplateStepListLimit {
		return false
	}
	hit[key] = append([]any{}, steps[:compactTemplateStepListLimit]...)
	hit["stepListOmitted"] = len(steps) - compactTemplateStepListLimit
	return true
}

func setCompactField(out map[string]any, key string, value any) {
	if out == nil || strings.TrimSpace(key) == "" || value == nil {
		return
	}
	if s, ok := value.(string); ok {
		if strings.TrimSpace(s) == "" {
			return
		}
		out[key] = s
		return
	}
	if items, ok := value.([]any); ok {
		if len(items) == 0 {
			return
		}
		out[key] = items
		return
	}
	out[key] = value
}

func compactResourceListPayload(payload any) any {
	out := mapStringAny(payload)
	if out == nil {
		return payload
	}
	items := sliceAny(out["items"])
	if len(items) == 0 {
		return payload
	}
	for i, itemAny := range items {
		if compacted := compactResourceListItem(mapStringAny(itemAny)); compacted != nil {
			items[i] = compacted
		}
	}
	out["items"] = items
	out["outputView"] = "compact"
	if _, exists := out["hint"]; !exists {
		out["hint"] = "Resource list output omits storage paths and verbose metadata by default. Use `breyta resources get <uri>` for one resource's metadata or `breyta resources read <uri> --full` for full content."
	}
	return out
}

func compactResourceListItem(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	out := compactNonEmptyFields(map[string]any{
		"uri":            firstNonBlankString(item["uri"]),
		"type":           firstNonBlankString(item["type"]),
		"displayName":    resourceDisplayNameForCLI(item),
		"sourceLabel":    resourceSourceLabelForCLI(item),
		"contentType":    firstNonBlankString(item["contentType"], item["content-type"], item["mimeType"], item["mime-type"]),
		"flowSlug":       resourceFlowSlugForCLI(item),
		"workflowId":     resourceWorkflowIDForCLI(item),
		"stepId":         firstNonBlankString(item["stepId"], item["step-id"], resourceAdapterDetails(item)["step-id"], resourceAdapterDetails(item)["stepId"]),
		"tableName":      firstNonBlankString(item["tableName"], item["table-name"]),
		"rowCount":       firstPresentAny(item["rowCount"], item["row-count"], item["rowsWritten"], item["rows-written"]),
		"sizeBytes":      firstPresentAny(item["sizeBytes"], item["size-bytes"], item["bytes"], item["length"]),
		"score":          firstPresentAny(item["score"], item["rank"]),
		"storageBackend": firstNonBlankString(item["storageBackend"], item["storage-backend"]),
		"storageRoot":    firstNonBlankString(item["storageRoot"], item["storage-root"]),
		"updatedAt":      firstNonBlankString(item["updatedAt"], item["updated-at"], item["createdAt"], item["created-at"]),
		"webUrl":         firstNonBlankString(item["webUrl"], item["web-url"], item["url"]),
	})
	if uri := firstNonBlankString(out["uri"]); uri != "" {
		out["hitRef"] = "resource:" + uri
		out["nextCommand"] = "breyta resources read " + shellSingleQuote(uri) + " --limit 5"
	}
	if tags := sliceAny(item["tags"]); len(tags) > 0 {
		out["tags"] = tags
	}
	if snippet := firstNonBlankString(item["snippet"], item["textPreview"], item["text-preview"], item["preview"]); snippet != "" {
		out["snippet"] = truncateRunes(snippet, compactResourceSnippetRunes)
	}
	return out
}

func addFlowSearchHitRefs(hit map[string]any) {
	if hit == nil {
		return
	}
	slug := firstNonBlankString(hit["flow_slug"], hit["slug"])
	if slug == "" {
		return
	}
	scope := strings.ToLower(firstNonBlankString(hit["scope"]))
	if scope == "" {
		return
	}
	hit["hitRef"] = scope + ":" + slug
	switch scope {
	case "workspace":
		hit["nextCommand"] = "breyta flows show " + shellSingleQuote(slug)
	default:
		hit["nextCommand"] = "breyta flows templates search " + shellSingleQuote(slug) + " --full"
	}
}

func compactResourceReadPayload(payload any, uri string) any {
	if resourceReadLooksLikeTable(payload, uri) {
		return payload
	}
	source := resourceReadDataPayload(payload)
	contentType := resourceReadContentType(source)
	previewSource := resourceReadPreviewSource(source)
	rendered := renderCompactPreview(previewSource)
	preview, truncated := truncateRunesWithFlag(rendered, compactResourceReadRunes)
	return compactNonEmptyFields(map[string]any{
		"uri":          strings.TrimSpace(uri),
		"contentType":  contentType,
		"shape":        valueShape(previewSource),
		"keys":         objectPreviewKeys(previewSource, 20),
		"items":        arrayPreviewCount(previewSource),
		"preview":      preview,
		"truncated":    truncated,
		"previewRunes": len([]rune(preview)),
		"fullBytes":    len([]byte(rendered)),
		"hint":         "Resource content is compact by default. Use `breyta resources read " + strings.TrimSpace(uri) + " --full` when the full payload is required.",
	})
}

func compactJobsListEnvelope(out map[string]any) {
	data := mapStringAny(out["data"])
	if data == nil {
		return
	}
	changed := compactJobItemsField(data, "items")
	changed = compactJobItemsField(data, "jobs") || changed
	if batch := mapStringAny(data["batch"]); batch != nil {
		changed = compactJobItemsField(batch, "jobs") || changed
	}
	if !changed {
		return
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	meta["outputView"] = "compact"
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = "Job list output omits payload/result/attempt details by default. Use `breyta jobs show <job-id>` for one job or rerun the list command with --full for the raw response."
	}
}

func compactJobItemsField(parent map[string]any, key string) bool {
	if parent == nil {
		return false
	}
	items := sliceAny(parent[key])
	if len(items) == 0 {
		return false
	}
	changed := false
	for i, itemAny := range items {
		if compacted := compactJobListItem(mapStringAny(itemAny)); compacted != nil {
			items[i] = compacted
			changed = true
		}
	}
	if changed {
		parent[key] = items
	}
	return changed
}

func compactJobListItem(job map[string]any) map[string]any {
	if job == nil {
		return nil
	}
	result := mapStringAny(job["result"])
	outputs := firstMapStringAny(job["outputs"], result["outputs"])
	metrics := firstMapStringAny(job["metrics"], result["metrics"])
	artifacts := sliceAny(firstPresentAny(job["artifacts"], result["artifacts"]))
	attempts := sliceAny(job["attempts"])
	progress := compactJobProgress(mapStringAny(job["progress"]))
	summary, _ := truncateRunesWithFlag(firstNonBlankString(job["resultSummary"], job["result-summary"], result["summary"]), compactJobSummaryRunes)

	return compactNonEmptyFields(map[string]any{
		"jobId":             firstNonBlankString(job["jobId"], job["job-id"], job["id"]),
		"jobType":           firstNonBlankString(job["jobType"], job["job-type"], job["type"]),
		"status":            firstNonBlankString(job["status"]),
		"batchId":           firstNonBlankString(job["batchId"], job["batch-id"]),
		"rootWorkflowId":    firstNonBlankString(job["rootWorkflowId"], job["root-workflow-id"]),
		"parentStepId":      firstNonBlankString(job["parentStepId"], job["parent-step-id"]),
		"attempt":           firstPresentAny(job["attempt"]),
		"attemptCount":      positiveCount(len(attempts)),
		"maxAttempts":       firstPresentAny(job["maxAttempts"], job["max-attempts"]),
		"createdAt":         firstNonBlankString(job["createdAt"], job["created-at"]),
		"startedAt":         firstNonBlankString(job["startedAt"], job["started-at"]),
		"updatedAt":         firstNonBlankString(job["updatedAt"], job["updated-at"]),
		"completedAt":       firstNonBlankString(job["completedAt"], job["completed-at"]),
		"lastHeartbeatAt":   firstNonBlankString(job["lastHeartbeatAt"], job["last-heartbeat-at"]),
		"workerId":          firstNonBlankString(job["workerId"], job["worker-id"]),
		"summary":           summary,
		"progress":          progress,
		"metadataKeys":      nonEmptyObjectPreviewKeys(job["metadata"], 12),
		"payloadKeys":       nonEmptyObjectPreviewKeys(job["payload"], 12),
		"outputKeys":        nonEmptyObjectPreviewKeys(outputs, 12),
		"metricKeys":        nonEmptyObjectPreviewKeys(metrics, 12),
		"artifactCount":     positiveCount(len(artifacts)),
		"prUrl":             firstNonBlankString(outputs["pr-url"], outputs["prUrl"]),
		"reportResourceUri": firstNonBlankString(outputs["report-resource-uri"], outputs["reportResourceUri"]),
	})
}

func compactJobProgress(progress map[string]any) map[string]any {
	if progress == nil {
		return nil
	}
	message, _ := truncateRunesWithFlag(firstNonBlankString(progress["message"]), compactJobSummaryRunes)
	return compactNonEmptyFields(map[string]any{
		"status":    firstNonBlankString(progress["status"]),
		"message":   message,
		"updatedAt": firstNonBlankString(progress["updatedAt"], progress["updated-at"]),
	})
}

func firstMapStringAny(values ...any) map[string]any {
	for _, value := range values {
		if m := mapStringAny(value); m != nil {
			return m
		}
	}
	return nil
}

func nonEmptyObjectPreviewKeys(value any, max int) []string {
	keys := objectPreviewKeys(value, max)
	if len(keys) == 0 {
		return nil
	}
	return keys
}

func positiveCount(n int) any {
	if n <= 0 {
		return nil
	}
	return n
}

func compactConnectionsListPayload(payload any) any {
	out := mapStringAny(payload)
	if out == nil {
		return payload
	}
	items := sliceAny(firstPresentAny(out["items"], out["connections"]))
	if len(items) == 0 {
		return payload
	}
	for i, itemAny := range items {
		if compacted := compactConnectionListItem(mapStringAny(itemAny)); compacted != nil {
			items[i] = compacted
		}
	}
	if _, exists := out["items"]; exists {
		out["items"] = items
	} else {
		out["connections"] = items
	}
	out["outputView"] = "compact"
	if _, exists := out["hint"]; !exists {
		out["hint"] = "Connection list output omits raw config/auth by default. Use `breyta connections show <connection-id>` or rerun with --full for full connection details."
	}
	return out
}

func compactConnectionListItem(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	health := mapStringAny(item["health"])
	stats := mapStringAny(item["stats"])
	return compactNonEmptyFields(map[string]any{
		"connectionId":   firstNonBlankString(item["connection-id"], item["connectionId"], item["id"]),
		"name":           firstNonBlankString(item["name"]),
		"type":           firstNonBlankString(item["type"]),
		"status":         firstNonBlankString(item["status"]),
		"backend":        firstNonBlankString(item["backend"]),
		"baseUrl":        firstNonBlankString(item["base-url"], item["baseUrl"]),
		"description":    truncateNonBlank(firstNonBlankString(item["description"]), compactTemplateDescriptionRunes),
		"health":         compactConnectionHealth(health),
		"referenceCount": firstPresentAny(stats["reference-count"], stats["referenceCount"]),
		"updatedAt":      firstNonBlankString(item["updated-at"], item["updatedAt"]),
		"webUrl":         firstNonBlankString(item["webUrl"], item["web-url"]),
		"configKeys":     nonEmptyObjectPreviewKeys(item["config"], 8),
	})
}

func compactConnectionHealth(health map[string]any) map[string]any {
	if health == nil {
		return nil
	}
	return compactNonEmptyFields(map[string]any{
		"success": firstPresentAny(health["success"]),
		"status":  firstNonBlankString(health["status"]),
		"message": truncateNonBlank(firstNonBlankString(health["message"]), compactTemplateDescriptionRunes),
	})
}

func compactIncidentsListPayload(payload any) any {
	out := mapStringAny(payload)
	if out == nil {
		return payload
	}
	items := sliceAny(out["items"])
	if len(items) == 0 {
		return payload
	}
	for i, itemAny := range items {
		if compacted := compactIncidentListItem(mapStringAny(itemAny)); compacted != nil {
			items[i] = compacted
		}
	}
	out["items"] = items
	out["outputView"] = "compact"
	if _, exists := out["hint"]; !exists {
		out["hint"] = "Incident list output is compact by default. Use `breyta incidents show <incident-id>` or rerun with --full for full incident details."
	}
	return out
}

func compactIncidentListItem(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	return compactNonEmptyFields(map[string]any{
		"incidentId":           firstNonBlankString(item["incident-id"], item["incidentId"], item["id"]),
		"flowSlug":             firstNonBlankString(item["flow-slug"], item["flowSlug"]),
		"status":               firstNonBlankString(item["status"]),
		"severity":             firstNonBlankString(item["severity"]),
		"target":               firstNonBlankString(item["target"]),
		"surfaceKind":          firstNonBlankString(item["surface-kind"], item["surfaceKind"]),
		"surfaceReason":        firstNonBlankString(item["surface-reason"], item["surfaceReason"]),
		"failedRunCount":       firstPresentAny(item["failed-run-count"], item["failedRunCount"]),
		"affectedLaneCount":    firstPresentAny(item["affected-lane-count"], item["affectedLaneCount"]),
		"unresolvedLaneCount":  firstPresentAny(item["unresolved-lane-count"], item["unresolvedLaneCount"]),
		"reopenCount":          firstPresentAny(item["reopen-count"], item["reopenCount"]),
		"latestWorkflowId":     firstNonBlankString(item["latest-workflow-id"], item["latestWorkflowId"]),
		"latestRunId":          firstNonBlankString(item["latest-run-id"], item["latestRunId"]),
		"latestRunWebUrl":      firstNonBlankString(item["latest-run-web-url"], item["latestRunWebUrl"]),
		"firstSeenAt":          firstNonBlankString(item["first-seen-at"], item["firstSeenAt"]),
		"lastSeenAt":           firstNonBlankString(item["last-seen-at"], item["lastSeenAt"]),
		"updatedAt":            firstNonBlankString(item["updatedAt"], item["updated-at"]),
		"webUrl":               firstNonBlankString(item["web-url"], item["webUrl"]),
		"settingsWebUrl":       firstNonBlankString(item["settings-web-url"], item["settingsWebUrl"]),
		"flowCreatedByDisplay": firstNonBlankString(item["flow-created-by-display"], item["flowCreatedByDisplay"]),
	})
}

func compactDigestsListPayload(payload any) any {
	out := mapStringAny(payload)
	if out == nil {
		return payload
	}
	items := sliceAny(out["items"])
	if len(items) == 0 {
		return payload
	}
	for i, itemAny := range items {
		if compacted := compactDigestListItem(mapStringAny(itemAny)); compacted != nil {
			items[i] = compacted
		}
	}
	out["items"] = items
	out["outputView"] = "compact"
	if _, exists := out["hint"]; !exists {
		out["hint"] = "Digest list output is compact by default. Use `breyta digests show <digest-id>` or rerun with --full for full digest details."
	}
	return out
}

func compactDigestListItem(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	incidentIDs := sliceAny(item["incident-ids"])
	incidentSummaries := sliceAny(item["incident-summaries"])
	return compactNonEmptyFields(map[string]any{
		"digestId":                 firstNonBlankString(item["digest-id"], item["digestId"], item["id"]),
		"kind":                     firstNonBlankString(item["kind"]),
		"status":                   firstNonBlankString(item["status"]),
		"triggerKind":              firstNonBlankString(item["trigger-kind"], item["triggerKind"]),
		"incidentCount":            firstPresentAny(item["incident-count"], item["incidentCount"], positiveCount(len(incidentSummaries))),
		"incidentIds":              firstNAny(incidentIDs, 5),
		"incidentIdsOmitted":       omittedCount(len(incidentIDs), 5),
		"incidentSummaries":        compactDigestIncidentSummaries(incidentSummaries, 3),
		"incidentSummariesOmitted": omittedCount(len(incidentSummaries), 3),
		"summary":                  compactDigestSummary(mapStringAny(item["summary"])),
		"sourceIncidentId":         firstNonBlankString(item["source-incident-id"], item["sourceIncidentId"]),
		"sourceWorkItemId":         firstNonBlankString(item["source-work-item-id"], item["sourceWorkItemId"]),
		"windowStart":              firstNonBlankString(item["window-start"], item["windowStart"]),
		"windowEnd":                firstNonBlankString(item["window-end"], item["windowEnd"]),
		"updatedAt":                firstNonBlankString(item["updatedAt"], item["updated-at"]),
		"webUrl":                   firstNonBlankString(item["web-url"], item["webUrl"]),
		"settingsWebUrl":           firstNonBlankString(item["settings-web-url"], item["settingsWebUrl"]),
	})
}

func compactDigestIncidentSummaries(items []any, limit int) []any {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	keep := items
	if len(keep) > limit {
		keep = keep[:limit]
	}
	out := make([]any, 0, len(keep))
	for _, itemAny := range keep {
		item := mapStringAny(itemAny)
		if item == nil {
			continue
		}
		out = append(out, compactNonEmptyFields(map[string]any{
			"incidentId":      firstNonBlankString(item["incident-id"], item["incidentId"], item["id"]),
			"flowSlug":        firstNonBlankString(item["flow-slug"], item["flowSlug"]),
			"status":          firstNonBlankString(item["status"]),
			"severity":        firstNonBlankString(item["severity"]),
			"target":          firstNonBlankString(item["target"]),
			"failedRunCount":  firstPresentAny(item["failed-run-count"], item["failedRunCount"]),
			"latestRunId":     firstNonBlankString(item["latest-run-id"], item["latestRunId"]),
			"latestRunWebUrl": firstNonBlankString(item["latest-run-web-url"], item["latestRunWebUrl"]),
			"lastSeenAt":      firstNonBlankString(item["last-seen-at"], item["lastSeenAt"]),
			"webUrl":          firstNonBlankString(item["incident-web-url"], item["incidentWebUrl"], item["web-url"], item["webUrl"]),
		}))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compactDigestSummary(summary map[string]any) map[string]any {
	if summary == nil {
		return nil
	}
	return compactNonEmptyFields(map[string]any{
		"affectedFlowCount": firstPresentAny(summary["affected-flow-count"], summary["affectedFlowCount"]),
		"immediateCount":    firstPresentAny(summary["immediate-count"], summary["immediateCount"]),
		"ongoingCount":      firstPresentAny(summary["ongoing-count"], summary["ongoingCount"]),
		"openedCount":       firstPresentAny(summary["opened-count"], summary["openedCount"]),
		"recoveredCount":    firstPresentAny(summary["recovered-count"], summary["recoveredCount"]),
		"reopenedCount":     firstPresentAny(summary["reopened-count"], summary["reopenedCount"]),
	})
}

func truncateNonBlank(s string, max int) string {
	preview, _ := truncateRunesWithFlag(strings.TrimSpace(s), max)
	return preview
}

func firstNAny(items []any, limit int) []any {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func omittedCount(total int, shown int) any {
	if total <= shown {
		return nil
	}
	return total - shown
}

func resourceReadDataPayload(payload any) any {
	if m := mapStringAny(payload); m != nil {
		if data, exists := m["data"]; exists {
			return data
		}
	}
	return payload
}

func resourceReadLooksLikeTable(payload any, uri string) bool {
	if strings.Contains(strings.TrimSpace(uri), "/table/") {
		return true
	}
	source := resourceReadDataPayload(payload)
	m := mapStringAny(source)
	if m == nil {
		return false
	}
	contentType := resourceReadContentType(m)
	if strings.Contains(strings.ToLower(contentType), "breyta.table") {
		return true
	}
	if mapStringAny(m["query"]) != nil && firstNonBlankString(m["tableName"], m["table-name"], m["tableId"], m["table-id"]) != "" {
		return true
	}
	return false
}

func resourceReadContentType(source any) string {
	m := mapStringAny(source)
	if m == nil {
		return ""
	}
	return firstNonBlankString(m["contentType"], m["content-type"], m["mimeType"], m["mime-type"])
}

func resourceReadPreviewSource(source any) any {
	m := mapStringAny(source)
	if m == nil {
		return source
	}
	for _, key := range []string{"body", "content", "text", "value"} {
		if value, exists := m[key]; exists {
			return value
		}
	}
	return source
}

func renderCompactPreview(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return scalarString(value)
		}
		return string(b)
	}
}

func valueShape(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case nil:
		return ""
	default:
		return "value"
	}
}

func objectPreviewKeys(value any, max int) []string {
	m := mapStringAny(value)
	if m == nil || max <= 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > max {
		keys = keys[:max]
	}
	return keys
}

func arrayPreviewCount(value any) any {
	items := sliceAny(value)
	if len(items) == 0 {
		return nil
	}
	return len(items)
}

func truncateRunesWithFlag(s string, max int) (string, bool) {
	if max <= 0 {
		return "", strings.TrimSpace(s) != ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s, false
	}
	return string(runes[:max-1]) + "…", true
}

func compactDocsMarkdown(markdown string, slug string, section string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = compactDocsDefaultRunes
	}
	if strings.TrimSpace(section) != "" {
		if selected, ok := extractMarkdownSection(markdown, section); ok {
			if len([]rune(selected)) <= maxRunes {
				return strings.TrimRight(selected, "\n")
			}
			preview, _ := truncateRunesWithFlag(selected, maxRunes)
			return strings.TrimRight(preview, "\n") + "\n\n" + docsCompactHint(slug)
		}
	}
	if len([]rune(markdown)) <= maxRunes {
		return strings.TrimRight(markdown, "\n")
	}

	var b strings.Builder
	title := firstMarkdownHeading(markdown)
	if title == "" {
		title = strings.TrimSpace(slug)
	}
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	if summary := summarizeMarkdown(markdown); summary != "" {
		b.WriteString(summary)
		b.WriteString("\n\n")
	}
	if contents := markdownContents(markdown, compactDocsContentsLimit); len(contents) > 0 {
		b.WriteString("## Contents\n\n")
		for _, line := range contents {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	preview, _ := truncateRunesWithFlag(strings.TrimSpace(markdown), maxRunes)
	b.WriteString("## Preview\n\n")
	b.WriteString(preview)
	b.WriteString("\n\n")
	b.WriteString(docsCompactHint(slug))
	return strings.TrimRight(b.String(), "\n")
}

func docsCompactHint(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = "<slug>"
	}
	return "Compact docs preview. Use `breyta docs show " + slug + " --full` for the full page or `breyta docs show " + slug + " --section <heading>` for a focused section."
}

type markdownHeading struct {
	Level int
	Text  string
	Line  int
}

func markdownHeadings(markdown string) []markdownHeading {
	lines := strings.Split(markdown, "\n")
	headings := make([]markdownHeading, 0)
	inCode := false
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			continue
		}
		if inCode || !strings.HasPrefix(line, "#") {
			continue
		}
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
			continue
		}
		text := strings.TrimSpace(line[level+1:])
		if text == "" {
			continue
		}
		headings = append(headings, markdownHeading{Level: level, Text: text, Line: i})
	}
	return headings
}

func firstMarkdownHeading(markdown string) string {
	headings := markdownHeadings(markdown)
	if len(headings) == 0 {
		return ""
	}
	return strings.TrimSpace(headings[0].Text)
}

func markdownContents(markdown string, max int) []string {
	headings := markdownHeadings(markdown)
	if max > 0 && len(headings) > max {
		headings = headings[:max]
	}
	out := make([]string, 0, len(headings))
	for _, h := range headings {
		out = append(out, strings.Repeat("  ", h.Level-1)+h.Text)
	}
	return out
}

func extractMarkdownSection(markdown string, section string) (string, bool) {
	needle := normalizeHeadingText(section)
	if needle == "" {
		return "", false
	}
	lines := strings.Split(markdown, "\n")
	headings := markdownHeadings(markdown)
	for i, h := range headings {
		normalized := normalizeHeadingText(h.Text)
		if normalized != needle && !strings.Contains(normalized, needle) {
			continue
		}
		end := len(lines)
		for _, next := range headings[i+1:] {
			if next.Level <= h.Level {
				end = next.Line
				break
			}
		}
		return strings.Join(lines[h.Line:end], "\n"), true
	}
	return "", false
}

func normalizeHeadingText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if unicode.IsSpace(r) || r == '-' || r == '_' {
			if !lastSpace && b.Len() > 0 {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
