package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeProfilePayload_EDN(t *testing.T) {
	ednInput := []byte(`{:profile {:type :prod
                                 :autoUpgrade true}
                       :bindings {:api {:name "Users API"
                                        :url "https://api.example.com"
                                        :apikey :redacted
                                        :headers {:apikey "anon"
                                                  :x-api-key "svc"}
                                        :query-param "api_key"
                                        :location "query"}
                                  :ai {:provider "openai"
                                       :apiKey "sk_live_456"}}
                       :activation {:region "EU"
                                    :batch-size 500}}`)
	payload, err := decodeProfilePayload(ednInput, "edn")
	if err != nil {
		t.Fatalf("decodeProfilePayload failed: %v", err)
	}
	if payload.ProfileType != "prod" {
		t.Fatalf("expected profile type prod, got %q", payload.ProfileType)
	}
	if payload.AutoUpgrade == nil || *payload.AutoUpgrade != true {
		t.Fatalf("expected autoUpgrade true, got %#v", payload.AutoUpgrade)
	}
	want := map[string]any{
		"name-api":        "Users API",
		"url-api":         "https://api.example.com",
		"query-param-api": "api_key",
		"location-api":    "query",
		"provider-ai":     "openai",
		"apikey-ai":       "sk_live_456",
		"form-region":     "EU",
		"form-batch-size": 500,
	}
	for k, v := range want {
		got := payload.Inputs[k]
		if fmt.Sprint(got) != fmt.Sprint(v) {
			t.Fatalf("expected inputs[%s]=%v, got %v", k, v, got)
		}
	}
	headers, ok := payload.Inputs["headers-api"].(map[string]any)
	if !ok {
		t.Fatalf("expected headers-api to be a map, got %#v", payload.Inputs["headers-api"])
	}
	if headers["apikey"] != "anon" {
		t.Fatalf("expected headers-api.apikey to be anon, got %#v", headers["apikey"])
	}
	if headers["x-api-key"] != "svc" {
		t.Fatalf("expected headers-api.x-api-key to be svc, got %#v", headers["x-api-key"])
	}
}

func TestDecodeProfilePayload_RejectsUnknownTopLevelKey(t *testing.T) {
	jsonInput := []byte(`{"bindings": {}, "unknown": {}}`)
	_, err := decodeProfilePayload(jsonInput, "json")
	if err == nil {
		t.Fatalf("expected error for unknown top-level key")
	}
}

func TestDecodeProfilePayload_IgnoresGeneratePlaceholders(t *testing.T) {
	ednInput := []byte(`{:bindings {:webhook-secret {:secret :generate}}}`)
	payload, err := decodeProfilePayload(ednInput, "edn")
	if err != nil {
		t.Fatalf("decodeProfilePayload failed: %v", err)
	}
	if _, ok := payload.Inputs["secret-webhook-secret"]; ok {
		t.Fatalf("expected secret placeholder to be ignored")
	}
}

func TestParseSetAssignments(t *testing.T) {
	items := []string{
		"api.apikey=sk_live_123",
		"api.url=https://api.example.com",
		"api.headers.apikey=anon",
		"api.headers.x-api-key=svc",
		"api.query-param=api_key",
		"api.location=query",
		"activation.region=EU",
		"activation.batch-size=500",
	}
	out, err := parseSetAssignments(items)
	if err != nil {
		t.Fatalf("parseSetAssignments failed: %v", err)
	}
	if out["apikey-api"] != "sk_live_123" {
		t.Fatalf("expected apikey-api to be set")
	}
	if out["url-api"] != "https://api.example.com" {
		t.Fatalf("expected url-api to be set")
	}
	headers, ok := out["headers-api"].(map[string]any)
	if !ok {
		t.Fatalf("expected headers-api to be a map")
	}
	if headers["apikey"] != "anon" {
		t.Fatalf("expected headers-api.apikey to be set")
	}
	if headers["x-api-key"] != "svc" {
		t.Fatalf("expected headers-api.x-api-key to be set")
	}
	if out["query-param-api"] != "api_key" {
		t.Fatalf("expected query-param-api to be set")
	}
	if out["location-api"] != "query" {
		t.Fatalf("expected location-api to be set")
	}
	if out["form-region"] != "EU" {
		t.Fatalf("expected form-region to be set")
	}
	if fmt.Sprint(out["form-batch-size"]) != "500" {
		t.Fatalf("expected form-batch-size to be 500")
	}
}

func TestParseSetAssignments_AllowsFileValues(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("line1\nline2\n"), 0o600); err != nil {
		t.Fatalf("write temp secret file: %v", err)
	}
	items := []string{
		"webhook-secret.secret=@" + secretPath,
	}
	out, err := parseSetAssignments(items)
	if err != nil {
		t.Fatalf("parseSetAssignments failed: %v", err)
	}
	if out["secret-webhook-secret"] != "line1\nline2\n" {
		t.Fatalf("expected secret-webhook-secret to match file content, got %#v", out["secret-webhook-secret"])
	}
}

func TestParseSetAssignments_AllowsEscapedAt(t *testing.T) {
	items := []string{
		"webhook-secret.secret=@@not-a-file",
	}
	out, err := parseSetAssignments(items)
	if err != nil {
		t.Fatalf("parseSetAssignments failed: %v", err)
	}
	if out["secret-webhook-secret"] != "@not-a-file" {
		t.Fatalf("expected secret-webhook-secret to be literal @not-a-file, got %#v", out["secret-webhook-secret"])
	}
}
func TestBuildProfileTemplate_FromRequirements(t *testing.T) {
	reqs := []any{
		map[string]any{
			"slot":  "api",
			"type":  "http-api",
			"label": "Users API",
			"auth": map[string]any{
				"type": "api-key",
			},
			"base-url": "https://api.example.com",
		},
		map[string]any{
			"slot": "ai",
			"type": "llm-provider",
		},
		map[string]any{
			"kind": "form",
			"fields": []any{
				map[string]any{"key": "region", "default": "EU"},
				map[string]any{"key": "batch-size"},
			},
		},
	}
	template, err := buildProfileTemplate(reqs, "prod", nil)
	if err != nil {
		t.Fatalf("buildProfileTemplate failed: %v", err)
	}
	bindings, _ := template["bindings"].(map[string]any)
	api, _ := bindings["api"].(map[string]any)
	if api["url"] != "https://api.example.com" {
		t.Fatalf("expected api.url to be set, got %#v", api["url"])
	}
	if _, ok := api["apikey"]; !ok {
		t.Fatalf("expected api.apikey to be present")
	}
	activation, _ := template["activation"].(map[string]any)
	if activation["region"] != "EU" {
		t.Fatalf("expected activation.region to be EU, got %#v", activation["region"])
	}
	if _, ok := activation["batch-size"]; !ok {
		t.Fatalf("expected activation.batch-size to be present")
	}
}

func TestBuildProfileTemplate_WithBindingValues(t *testing.T) {
	reqs := []any{
		map[string]any{
			"slot":  "api",
			"type":  "http-api",
			"label": "Users API",
			"auth": map[string]any{
				"type": "api-key",
			},
		},
	}
	bindingValues := map[string]any{"api": "conn-123"}
	template, err := buildProfileTemplate(reqs, "prod", bindingValues)
	if err != nil {
		t.Fatalf("buildProfileTemplate failed: %v", err)
	}
	bindings, _ := template["bindings"].(map[string]any)
	api, _ := bindings["api"].(map[string]any)
	if api["conn"] != "conn-123" {
		t.Fatalf("expected api.conn to be set, got %#v", api["conn"])
	}
	if _, ok := api["apikey"]; ok {
		t.Fatalf("expected api.apikey to be omitted when conn is set")
	}
}
