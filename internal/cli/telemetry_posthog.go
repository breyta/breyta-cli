package cli

import (
	"net/url"
	"strings"
)

// These temporary call sites are retained while the surrounding flow commands
// are simplified. The canonical self-hosted CLI never emits product telemetry.
func trackAuthLoginTelemetry(_ *App, _ string, _ string, _ any) {}

func trackCLIEvent(_ *App, _ string, _ any, _ string, _ map[string]any) {}

func trackCommandTelemetry(_ *App, _ string, _ map[string]any, _ int, _ bool) {}

func argString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func apiHostname(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}
