package cli

import (
	"errors"
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
	if app.DevMode {
		return
	}

	allowRoot := map[string]bool{
		"flows":      true,
		"flow":       true, // alias
		"runs":       true,
		"run":        true, // alias
		"resources":  true,
		"docs":       true,
		"feedback":   true,
		"auth":       true,
		"skills":     true,
		"workspaces": true,
		"upgrade":    true,
		"version":    true,
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
		"list":          true,
		"search":        true,
		"show":          true,
		"create":        true,
		"configure":     true,
		"diff":          true,
		"pull":          true,
		"push":          true,
		"validate":      true,
		"release":       true,
		"promote":       true,
		"run":           true,
		"delete":        true,
		"installations": true,
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

func hideDevOnlyCommandTree(cmd *cobra.Command, app *App) *cobra.Command {
	if cmd == nil {
		return nil
	}
	var wrap func(*cobra.Command)
	wrap = func(current *cobra.Command) {
		if current == nil {
			return
		}
		current.Hidden = true

		prev := current.PreRunE
		current.PreRunE = func(cmd *cobra.Command, args []string) error {
			if app == nil || (!app.DevMode && !devModeEnabled()) {
				return writeErr(cmd, errors.New("this command is not part of the public CLI surface"))
			}
			if prev != nil {
				return prev(cmd, args)
			}
			return nil
		}

		for _, child := range current.Commands() {
			wrap(child)
		}
	}

	wrap(cmd)
	return cmd
}
