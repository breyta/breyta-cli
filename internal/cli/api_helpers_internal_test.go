package cli

import (
	"path/filepath"
	"testing"

	"github.com/breyta/breyta-cli/internal/authstore"
)

func TestRequireAPILoadsStoredPersonalToken(t *testing.T) {
	apiURL := "https://example.test"
	storePath := filepath.Join(t.TempDir(), "auth.json")
	store := &authstore.Store{}
	store.SetRecord(apiURL, authstore.Record{Token: "personal-token"})
	if err := authstore.SaveAtomic(storePath, store); err != nil {
		t.Fatalf("save auth store: %v", err)
	}
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	app := &App{APIURL: apiURL}
	if err := requireAPI(app); err != nil {
		t.Fatalf("requireAPI: %v", err)
	}
	if app.Token != "personal-token" {
		t.Fatalf("expected stored personal token, got %q", app.Token)
	}
}

func TestRequireAPIPreservesExplicitToken(t *testing.T) {
	app := &App{APIURL: "https://example.test", Token: "explicit-token", TokenExplicit: true}
	if err := requireAPI(app); err != nil {
		t.Fatalf("requireAPI: %v", err)
	}
	if app.Token != "explicit-token" {
		t.Fatalf("expected explicit token, got %q", app.Token)
	}
}

func TestRequireAPIRejectsMissingToken(t *testing.T) {
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(t.TempDir(), "missing.json"))
	app := &App{APIURL: "https://example.test"}
	if err := requireAPI(app); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestServerRecoveryActionsReplaceHostedRoutes(t *testing.T) {
	app := &App{APIURL: "https://example.test/breyta", WorkspaceID: "ws-acme"}
	tests := []struct {
		name   string
		action map[string]any
		want   string
	}{
		{
			name: "draft bindings",
			action: map[string]any{
				"kind": "draft-bindings",
				"url":  "/ws-acme/flows/daily-report/draft-bindings",
			},
			want: "https://example.test/breyta/ui?workspace=ws-acme&page=flows&flow=daily-report",
		},
		{
			name: "connection edit",
			action: map[string]any{
				"kind":         "connection-edit",
				"url":          "/ws-acme/connections/conn-123/edit",
				"connectionId": "conn-123",
			},
			want: "https://example.test/breyta/ui?workspace=ws-acme&page=connections&connection=conn-123",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeRecoveryAction(app, test.action)
			if url := firstNonBlankString(got["url"]); url != test.want {
				t.Fatalf("unexpected recovery URL: got %q want %q", url, test.want)
			}
		})
	}
}
