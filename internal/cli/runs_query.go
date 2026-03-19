package cli

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type runsListFilters struct {
	Flow           string
	InstallationID string
	Status         string
	Version        int
	HasVersion     bool
}

var supportedRunsQueryStatuses = map[string]struct{}{
	"running":   {},
	"completed": {},
	"failed":    {},
	"waiting":   {},
}

func parseRunsListQuery(raw string) (runsListFilters, error) {
	var filters runsListFilters
	query := strings.TrimSpace(raw)
	if query == "" {
		return filters, nil
	}
	for _, token := range strings.Fields(query) {
		parts := strings.SplitN(token, ":", 2)
		if len(parts) != 2 {
			return runsListFilters{}, fmt.Errorf("invalid runs query token %q; use status:, flow:, installation:, or version:", token)
		}
		field := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if value == "" {
			return runsListFilters{}, fmt.Errorf("invalid runs query token %q; missing value", token)
		}
		switch field {
		case "status":
			status := strings.ToLower(value)
			if _, ok := supportedRunsQueryStatuses[status]; !ok {
				return runsListFilters{}, fmt.Errorf("invalid runs query status %q; use running, completed, failed, or waiting", value)
			}
			if filters.Status != "" && filters.Status != status {
				return runsListFilters{}, fmt.Errorf("multiple status: filters are not supported in CLI/API mode yet; got %q and %q", filters.Status, status)
			}
			filters.Status = status
		case "flow":
			filters.Flow = normalizeRunsQueryFlowSlug(value)
			if filters.Flow == "" {
				return runsListFilters{}, fmt.Errorf("invalid flow: token %q", token)
			}
		case "installation":
			filters.InstallationID = value
		case "version":
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				return runsListFilters{}, fmt.Errorf("invalid version: token %q; use a non-negative integer", token)
			}
			filters.Version = parsed
			filters.HasVersion = true
		default:
			return runsListFilters{}, fmt.Errorf("unsupported runs query token %q; use status:, flow:, installation:, or version:", token)
		}
	}
	return filters, nil
}

func normalizeRunsQueryFlowSlug(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, ":")
	return strings.TrimSpace(value)
}

func buildRunsListQuery(filters runsListFilters) string {
	tokens := make([]string, 0, 4)
	if strings.TrimSpace(filters.Status) != "" {
		tokens = append(tokens, "status:"+strings.TrimSpace(filters.Status))
	}
	if flow := normalizeRunsQueryFlowSlug(filters.Flow); flow != "" {
		tokens = append(tokens, "flow:"+flow)
	}
	if installationID := strings.TrimSpace(filters.InstallationID); installationID != "" {
		tokens = append(tokens, "installation:"+installationID)
	}
	if filters.HasVersion {
		tokens = append(tokens, "version:"+strconv.Itoa(filters.Version))
	}
	return strings.Join(tokens, " ")
}

func runsListWebURL(base string, filters runsListFilters) string {
	baseURL := runsWebURL(base)
	query := buildRunsListQuery(filters)
	if baseURL == "" || strings.TrimSpace(query) == "" {
		return baseURL
	}
	return baseURL + "?query=" + url.QueryEscape(query)
}

func annotateRunsListResult(app *App, out map[string]any, args map[string]any) {
	if out == nil || app == nil {
		return
	}
	data, _ := out["data"].(map[string]any)
	if data == nil {
		return
	}
	queryFilters, err := parseRunsListQuery(firstNonBlankString(args["query"]))
	if err != nil {
		queryFilters = runsListFilters{}
	}
	if flow := normalizeRunsQueryFlowSlug(firstNonBlankString(args["flowSlug"], args["flow-slug"])); flow != "" {
		queryFilters.Flow = flow
	}
	if installationID := firstNonBlankString(args["profileId"], args["profile-id"], args["installationId"], args["installation-id"]); installationID != "" {
		queryFilters.InstallationID = installationID
	}
	if status := strings.TrimSpace(firstNonBlankString(args["status"], args["runStatus"], args["run-status"])); status != "" {
		queryFilters.Status = strings.ToLower(status)
	}
	if versionAny, ok := args["version"]; ok {
		queryFilters.Version = anyInt(versionAny)
		queryFilters.HasVersion = true
	}
	if webURL := runsListWebURL(workspaceWebBaseURL(app), queryFilters); webURL != "" {
		setIfMissing(data, "webUrl", webURL)
		meta := ensureMeta(out)
		if _, exists := meta["webUrl"]; !exists {
			meta["webUrl"] = webURL
		}
	}
}
