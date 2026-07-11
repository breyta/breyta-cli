package cli

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const proactiveActivityTimeout = 2 * time.Second

func recordProactiveFlowActivity(ctx context.Context, app *App, kind string, flowSlug string, extra map[string]any) {
	if app == nil || !isAPIMode(app) {
		return
	}
	flowSlug = strings.TrimSpace(flowSlug)
	if flowSlug == "" {
		return
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "flow-activity"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, proactiveActivityTimeout)
	defer cancel()

	payload := map[string]any{
		"source":   "cli",
		"kind":     kind,
		"flowSlug": flowSlug,
	}
	for k, v := range extra {
		if strings.TrimSpace(k) == "" || v == nil {
			continue
		}
		payload[k] = v
	}
	_, _, _ = apiClient(app).DoREST(ctx, http.MethodPost, "/api/proactive-agent/activity", nil, payload)
}
