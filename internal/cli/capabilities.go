package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const engineAPIVersion = 2

var requiredEngineCommands = []string{
	"flows.archive",
	"flows.compile",
	"flows.delete",
	"flows.deploy",
	"flows.diff",
	"flows.draft.reset",
	"flows.get",
	"flows.list",
	"flows.put_draft",
	"flows.release",
	"flows.run",
	"flows.run_step",
	"flows.update",
	"flows.validate",
	"flows.versions.activate",
	"flows.versions.list",
	"flows.versions.publish",
	"flows.versions.update",
	"runs.cancel",
	"runs.events",
	"runs.get",
	"runs.list",
	"runs.replay",
	"runs.start",
	"workspace.export",
	"workspace.import",
}

func newAPICheckCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check compatibility with a Breyta engine",
		RunE: func(cmd *cobra.Command, args []string) error {
			ensureAPIURL(app)
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			manifest, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/capabilities", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status != http.StatusOK {
				return writeErr(cmd, fmt.Errorf("capability discovery failed (status=%d)", status))
			}
			if err := validateEngineCapabilities(manifest); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, map[string]any{"httpStatus": status}, map[string]any{
				"compatible":   true,
				"capabilities": manifest,
			})
		},
	}
}

func validateEngineCapabilities(raw any) error {
	manifest, ok := raw.(map[string]any)
	if !ok {
		return errors.New("engine returned an invalid capability manifest")
	}
	version, ok := manifest["apiVersion"].(float64)
	if !ok || version != float64(engineAPIVersion) {
		return fmt.Errorf("unsupported engine API version %v (CLI requires %d)", manifest["apiVersion"], engineAPIVersion)
	}

	commands := map[string]bool{}
	commandValues, ok := manifest["commands"].([]any)
	if !ok {
		return errors.New("engine capability manifest has an invalid command list")
	}
	for _, value := range commandValues {
		if command, ok := value.(string); ok {
			commands[command] = true
		}
	}
	for _, command := range requiredEngineCommands {
		if !commands[command] {
			return fmt.Errorf("engine capability manifest is missing %q", command)
		}
	}

	handoff, ok := manifest["workspaceHandoff"].(map[string]any)
	if !ok || handoff["format"] != "breyta.workspace" || handoff["export"] != "/api/workspace/export" || handoff["import"] != "/api/workspace/import" {
		return errors.New("engine capability manifest has an incompatible workspace handoff contract")
	}
	selection, ok := manifest["workspaceSelection"].(map[string]any)
	if !ok || !strings.EqualFold(fmt.Sprint(selection["header"]), "X-Breyta-Workspace") {
		return errors.New("engine capability manifest has an incompatible workspace selection contract")
	}
	authentication, ok := manifest["authentication"].(map[string]any)
	if !ok || authentication["personalTokens"] != "/api/auth/personal-tokens" || authentication["serviceAccounts"] != "/api/service-accounts" {
		return errors.New("engine capability manifest has an incompatible authentication contract")
	}
	resources, ok := manifest["resources"].(map[string]any)
	if !ok {
		return errors.New("engine capability manifest has an invalid resource contract")
	}
	for name, path := range map[string]string{
		"connections": "/api/connections",
		"secrets":     "/api/secrets",
		"triggers":    "/api/triggers",
		"waits":       "/api/waits",
		"resources":   "/api/resources",
		"files":       "/api/files",
	} {
		if resources[name] != path {
			return fmt.Errorf("engine capability manifest has an incompatible %s endpoint", name)
		}
	}
	return nil
}
