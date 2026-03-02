package cli

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	resourceRunURIWorkflowIDPattern = regexp.MustCompile(`^res://v1/ws/[^/]+/result/run/([^/]+)(?:/|$)`)
	resourcePathWorkflowIDPattern   = regexp.MustCompile(`/runs/([^/]+)(?:/|$)`)
	resourcePathFlowSlugPattern     = regexp.MustCompile(`/persist/([^/]+)(?:/|$)`)
)

func enrichResourceListPayload(payload any) any {
	out, _ := payload.(map[string]any)
	if out == nil {
		return payload
	}
	items, _ := out["items"].([]any)
	if len(items) == 0 {
		return payload
	}
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item == nil {
			continue
		}
		enrichResourceListItem(item)
	}
	return payload
}

func enrichResourceListItem(item map[string]any) {
	displayName := resourceDisplayNameForCLI(item)
	if displayName != "" {
		setIfMissing(item, "display-name", displayName)
		setIfMissing(item, "displayName", displayName)
	}

	sourceLabel := resourceSourceLabelForCLI(item)
	if sourceLabel != "" {
		setIfMissing(item, "source-label", sourceLabel)
		setIfMissing(item, "sourceLabel", sourceLabel)
	}
}

func resourceDisplayNameForCLI(item map[string]any) string {
	resourceURI := resourceURIForCLI(item)
	explicitLabel := coalesceNonBlank(
		asString(item, "label"),
		asString(item, "display-name"),
		asString(item, "displayName"),
		asString(item, "name"),
		asString(item, "title"),
	)
	explicitFilename := coalesceNonBlank(
		asString(item, "filename"),
		asString(item, "file-name"),
		asString(item, "fileName"),
		asString(resourceAdapterDetails(item), "filename"),
	)
	filenameFromPath := pathBasename(resourcePathForCLI(item))
	uriSegment := pathBasename(decodeURISegment(uriLastSegment(resourceURI)))
	return coalesceNonBlank(
		explicitLabel,
		explicitFilename,
		filenameFromPath,
		uriSegment,
		resourceURI,
	)
}

func resourceSourceLabelForCLI(item map[string]any) string {
	explicitSource := coalesceNonBlank(
		asString(item, "source-label"),
		asString(item, "sourceLabel"),
		asString(item, "source"),
	)
	if explicitSource != "" {
		return explicitSource
	}

	flowSlug := resourceFlowSlugForCLI(item)
	workflowID := resourceWorkflowIDForCLI(item)
	switch {
	case flowSlug != "" && workflowID != "":
		return fmt.Sprintf("flow %s • run %s", flowSlug, workflowID)
	case flowSlug != "":
		return fmt.Sprintf("flow %s", flowSlug)
	case workflowID != "":
		return fmt.Sprintf("run %s", workflowID)
	}

	switch resourceTypeForCLI(item) {
	case "file":
		return "workspace file"
	case "result":
		return "saved result"
	default:
		return ""
	}
}

func resourceURIForCLI(item map[string]any) string {
	return coalesceNonBlank(asString(item, "uri"))
}

func resourcePathForCLI(item map[string]any) string {
	details := resourceAdapterDetails(item)
	return coalesceNonBlank(
		asString(details, "path"),
		asString(item, "path"),
	)
}

func resourceAdapterDetails(item map[string]any) map[string]any {
	if adapter, _ := item["adapter"].(map[string]any); adapter != nil {
		if details, _ := adapter["details"].(map[string]any); details != nil {
			return details
		}
	}
	if details, _ := item["details"].(map[string]any); details != nil {
		return details
	}
	return nil
}

func resourceTypeForCLI(item map[string]any) string {
	raw := strings.TrimSpace(asString(item, "type"))
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, ":")
	return strings.ToLower(raw)
}

func resourceWorkflowIDForCLI(item map[string]any) string {
	details := resourceAdapterDetails(item)
	return coalesceNonBlank(
		asString(details, "workflow-id"),
		asString(details, "workflowId"),
		asString(item, "workflow-id"),
		asString(item, "workflowId"),
		workflowIDFromURI(resourceURIForCLI(item)),
		workflowIDFromPath(resourcePathForCLI(item)),
	)
}

func resourceFlowSlugForCLI(item map[string]any) string {
	details := resourceAdapterDetails(item)
	flowSlug := coalesceNonBlank(
		asString(details, "flow-slug"),
		asString(details, "flowSlug"),
		asString(item, "flow-slug"),
		asString(item, "flowSlug"),
		flowSlugFromPath(resourcePathForCLI(item)),
	)
	if flowSlug == "flow" {
		return ""
	}
	return flowSlug
}

func workflowIDFromURI(resourceURI string) string {
	m := resourceRunURIWorkflowIDPattern.FindStringSubmatch(strings.TrimSpace(resourceURI))
	if len(m) != 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func workflowIDFromPath(path string) string {
	m := resourcePathWorkflowIDPattern.FindStringSubmatch(normalizePath(path))
	if len(m) != 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func flowSlugFromPath(path string) string {
	m := resourcePathFlowSlugPattern.FindStringSubmatch(normalizePath(path))
	if len(m) != 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return strings.ReplaceAll(path, "\\", "/")
}

func pathBasename(path string) string {
	normalized := normalizePath(path)
	if normalized == "" {
		return ""
	}
	parts := strings.Split(normalized, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part != "" {
			return part
		}
	}
	return ""
}

func uriLastSegment(resourceURI string) string {
	resourceURI = strings.TrimSpace(resourceURI)
	if resourceURI == "" {
		return ""
	}
	if idx := strings.Index(resourceURI, "?"); idx >= 0 {
		resourceURI = resourceURI[:idx]
	}
	if idx := strings.Index(resourceURI, "#"); idx >= 0 {
		resourceURI = resourceURI[:idx]
	}
	return pathBasename(resourceURI)
}

func decodeURISegment(segment string) string {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return ""
	}
	decoded, err := url.PathUnescape(segment)
	if err != nil {
		return segment
	}
	return strings.TrimSpace(decoded)
}
