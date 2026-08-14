package cli

import "testing"

func TestValidateEngineCapabilities(t *testing.T) {
	commands := make([]any, 0, len(requiredEngineCommands))
	for _, command := range requiredEngineCommands {
		commands = append(commands, command)
	}
	manifest := map[string]any{
		"apiVersion": float64(2),
		"commands":   commands,
		"workspaceHandoff": map[string]any{
			"format": "breyta.workspace",
			"export": "/api/workspace/export",
			"import": "/api/workspace/import",
		},
		"workspaceSelection": map[string]any{"header": "X-Breyta-Workspace"},
		"authentication": map[string]any{
			"personalTokens":  "/api/auth/personal-tokens",
			"serviceAccounts": "/api/service-accounts",
		},
		"resources": map[string]any{
			"connections": "/api/connections",
			"secrets":     "/api/secrets",
			"triggers":    "/api/triggers",
			"waits":       "/api/waits",
			"resources":   "/api/resources",
			"files":       "/api/files",
		},
	}
	if err := validateEngineCapabilities(manifest); err != nil {
		t.Fatalf("expected engine manifest to be compatible: %v", err)
	}

	manifest["apiVersion"] = float64(3)
	if err := validateEngineCapabilities(manifest); err == nil {
		t.Fatal("expected a future API version to be rejected")
	}
}

func TestCanonicalRootOmitsHostedCommands(t *testing.T) {
	root := NewRootCmd()
	for _, name := range []string{"discover", "jobs", "pricing", "purchases", "payouts", "creator", "analytics", "agent", "dev", "internal"} {
		command, _, err := root.Find([]string{name})
		if err == nil && command != root {
			t.Fatalf("retired command %q remains registered", name)
		}
	}
	for _, name := range []string{"api", "auth", "flows", "runs", "workspace"} {
		command, _, err := root.Find([]string{name})
		if err != nil || command == root {
			t.Fatalf("canonical command %q is missing", name)
		}
	}
	if root.PersistentFlags().Lookup("dev") != nil || root.PersistentFlags().Lookup("state") != nil {
		t.Fatal("retired mode flags remain registered")
	}
	if command, _, err := root.Find([]string{"flows", "steps", "run"}); err == nil && command.Name() == "run" {
		t.Fatal("legacy steps.run command remains registered despite being absent from the engine contract")
	}
}
