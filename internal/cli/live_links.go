package cli

import (
	"net/url"
	"strings"

	"github.com/breyta/breyta-cli/internal/live"
)

func enrichLiveDisplayFrameWebLinks(app *App, frame live.DisplayFrame) live.DisplayFrame {
	base := normalizeLocalhostWebURL(workspaceWebBaseURL(app))
	if strings.TrimSpace(base) == "" || len(frame.Lines) == 0 {
		return frame
	}
	out := frame
	out.Lines = make([]live.DisplayLine, len(frame.Lines))
	for i, line := range frame.Lines {
		if line.Planned {
			line.WebURL = ""
			out.Lines[i] = line
			continue
		}
		if strings.TrimSpace(line.WebURL) == "" {
			line.WebURL = liveResourceWebURL(base, line)
		}
		out.Lines[i] = line
	}
	return out
}

func liveResourceWebURL(base string, line live.DisplayLine) string {
	resourceURI := strings.TrimSpace(line.ResourceURI)
	if line.Planned || resourceURI == "" {
		return ""
	}
	workflowID, _, kind := parseRunResourceURI(resourceURI)
	runID := coalesceNonBlank(workflowID, line.WorkflowID)
	flowSlug := strings.TrimSpace(line.FlowSlug)
	if runID == "" || flowSlug == "" {
		return ""
	}
	if kind == "flow-output" {
		return runOutputWebURL(base, flowSlug, runID)
	}
	return runArtifactWebURL(base, flowSlug, runID, resourceURI)
}

func runArtifactWebURL(base, flowSlug, runID, resourceURI string) string {
	runURL := runWebURL(base, flowSlug, runID)
	resourceURI = strings.TrimSpace(resourceURI)
	if runURL == "" || resourceURI == "" {
		return ""
	}
	query := url.Values{}
	query.Set("artifactUri", resourceURI)
	query.Set("output", "fullscreen")
	return runURL + "?" + query.Encode()
}
