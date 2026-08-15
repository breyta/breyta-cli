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
		{
			name: "installation without profile falls back to flow",
			action: map[string]any{
				"kind":     "installation",
				"url":      "/ws-acme/flows/daily-report/installations",
				"flowSlug": "daily-report",
			},
			want: "https://example.test/breyta/ui?workspace=ws-acme&page=flows&flow=daily-report",
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

func TestServerRecoveryActionsUseHostedRouteAfterProxyPrefix(t *testing.T) {
	app := &App{APIURL: "https://example.test/flows", WorkspaceID: "ws-acme"}
	action := map[string]any{
		"kind": "draft-bindings",
		"url":  "/ws-acme/flows/daily-report/draft-bindings",
	}
	got := normalizeRecoveryAction(app, action)
	want := "https://example.test/flows/ui?workspace=ws-acme&page=flows&flow=daily-report"
	if gotURL := firstNonBlankString(got["url"]); gotURL != want {
		t.Fatalf("unexpected recovery URL: got %q want %q", gotURL, want)
	}
}

func TestUnrecognizedListKeepsTopLevelWebURL(t *testing.T) {
	app := &App{APIURL: "https://example.test", WorkspaceID: "ws-acme"}
	envelope := map[string]any{
		"meta": map[string]any{
			"webUrl": "https://external.example/digests",
		},
		"data": map[string]any{
			"items": []any{map[string]any{
				"title":  "Digest",
				"webUrl": "https://external.example/digests/first",
			}},
		},
	}

	enrichEnvelopeWebLinks(app, envelope)
	meta := envelope["meta"].(map[string]any)
	if got := firstNonBlankString(meta["webUrl"]); got != "https://external.example/digests" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
}
