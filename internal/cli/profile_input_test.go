package cli

import (
	"fmt"
	"testing"
)

func TestDecodeProfilePayload_EDN(t *testing.T) {
	ednInput := []byte(`{:profile {:type :prod
                                 :autoUpgrade true}
                       :bindings {:api {:name "Users API"
                                        :url "https://api.example.com"
                                        :apikey :redacted}
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
	if out["form-region"] != "EU" {
		t.Fatalf("expected form-region to be set")
	}
	if fmt.Sprint(out["form-batch-size"]) != "500" {
		t.Fatalf("expected form-batch-size to be 500")
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
