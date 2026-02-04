package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func configureVisibility(root *cobra.Command, app *App) {
	if root == nil || app == nil {
		return
	}
	if app.visibilityConfigured {
		return
	}
	app.visibilityConfigured = true

	// Default: keep a minimal surface area for agents and humans.
	// The larger mocked/future CLI surface stays available behind --dev.
	if app.DevMode {
		return
	}

	allowRoot := map[string]bool{
		"flows":     true,
		"flow":      true, // alias
		"runs":      true,
		"run":       true, // alias
		"resources": true,
		"docs":      true,
		"auth":      true,
		"agents":    true,
	}

	for _, c := range root.Commands() {
		if !allowRoot[c.Name()] {
			c.Hidden = true
		}
	}

	flows := root.Commands()
	var flowsCmd *cobra.Command
	for _, c := range flows {
		if c.Name() == "flows" {
			flowsCmd = c
			break
		}
	}
	if flowsCmd == nil {
		return
	}

	allowFlows := map[string]bool{
		"list":               true,
		"show":               true,
		"create":             true,
		"pull":               true,
		"push":               true,
		"deploy":             true,
		"bindings":           true,
		"activate":           true,
		"draft":              true,
		"draft-bindings-url": true,
	}
	for _, sc := range flowsCmd.Commands() {
		if !allowFlows[sc.Name()] {
			sc.Hidden = true
		}
		// Hide nested trees like "steps", "versions" entirely.
		if strings.TrimSpace(sc.Name()) == "" {
			sc.Hidden = true
		}
	}
}
