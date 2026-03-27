package cli

import "testing"

func TestCompatibleLLMBackend(t *testing.T) {
	t.Run("infers LLM backend from compatible HTTP base URL", func(t *testing.T) {
		conn := connectionSummary{
			ID:      "conn-http-openai",
			Name:    "OpenAI HTTP",
			Type:    "http-api",
			Backend: "rest",
			BaseURL: "https://api.openai.com/v1",
		}
		if got := compatibleLLMBackend(conn); got != "openai" {
			t.Fatalf("expected openai backend, got %q", got)
		}
	})

	t.Run("ignores unrelated HTTP endpoints", func(t *testing.T) {
		conn := connectionSummary{
			ID:      "conn-http-generic",
			Name:    "Generic HTTP",
			Type:    "http-api",
			Backend: "rest",
			BaseURL: "https://api.example.com",
		}
		if got := compatibleLLMBackend(conn); got != "" {
			t.Fatalf("expected no compatible backend, got %q", got)
		}
	})
}

func TestConnectionBucketsForRequirements(t *testing.T) {
	conn := connectionSummary{
		ID:      "conn-http-openai",
		Name:    "OpenAI HTTP",
		Type:    "http-api",
		Backend: "rest",
		BaseURL: "https://api.openai.com/v1",
	}
	requiredTypes := map[string]struct{}{
		"http-api":     {},
		"llm-provider": {},
	}
	buckets := connectionBucketsForRequirements(conn, requiredTypes)
	if len(buckets) != 2 {
		t.Fatalf("expected both http-api and llm-provider buckets, got %#v", buckets)
	}
	if buckets[0] != "http-api" || buckets[1] != "llm-provider" {
		t.Fatalf("unexpected bucket order/content: %#v", buckets)
	}
}

func TestPickPreferredLLMConnectionPrefersExactLLMProvider(t *testing.T) {
	conns := []connectionSummary{
		{ID: "conn-http-openai", Name: "OpenAI HTTP", Type: "http-api", Backend: "rest", BaseURL: "https://api.openai.com/v1"},
		{ID: "conn-llm-openai", Name: "OpenAI LLM", Type: "llm-provider", Backend: "openai", BaseURL: "https://api.openai.com/v1"},
	}
	got, ok := pickPreferredLLMConnection(conns)
	if !ok {
		t.Fatalf("expected a preferred connection")
	}
	if got.ID != "conn-llm-openai" {
		t.Fatalf("expected exact llm-provider to win, got %#v", got)
	}
}

func TestApplyDefaultConnectionReuseAllowsCompatibleHTTPConnectionForLLM(t *testing.T) {
	template := map[string]any{
		"bindings": map[string]any{},
	}
	requirements := []any{
		map[string]any{
			"slot": "llm",
			"type": "llm-provider",
		},
	}
	connectionsByType := map[string][]connectionSummary{
		"llm-provider": {
			{ID: "conn-http-openai", Name: "OpenAI HTTP", Type: "http-api", Backend: "rest", BaseURL: "https://api.openai.com/v1"},
		},
	}

	applyDefaultConnectionReuse(template, requirements, connectionsByType)

	bindings, _ := template["bindings"].(map[string]any)
	llmBinding, _ := bindings["llm"].(map[string]any)
	if llmBinding["conn"] != "conn-http-openai" {
		t.Fatalf("expected compatible http connection to be reused, got %#v", llmBinding)
	}
}

func TestBuildConfigureSuggestionsAllowsCompatibleHTTPConnectionForLLM(t *testing.T) {
	requirements := []any{
		map[string]any{
			"slot": "llm",
			"type": "llm-provider",
		},
	}
	connectionsByType := map[string][]connectionSummary{
		"llm-provider": {
			{ID: "conn-http-openai", Name: "OpenAI HTTP", Type: "http-api", Backend: "rest", BaseURL: "https://api.openai.com/v1"},
		},
	}

	rows, setArgs, unresolved := buildConfigureSuggestions(requirements, map[string]string{}, connectionsByType)
	if len(rows) != 1 {
		t.Fatalf("expected one suggestion row, got %#v", rows)
	}
	if rows[0].Status != "suggested" {
		t.Fatalf("expected llm slot to be suggested, got %#v", rows[0])
	}
	if rows[0].SuggestedConnectionID != "conn-http-openai" {
		t.Fatalf("unexpected suggested connection: %#v", rows[0])
	}
	if len(setArgs) != 1 || setArgs[0] != "llm.conn=conn-http-openai" {
		t.Fatalf("unexpected set args: %#v", setArgs)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved slots, got %#v", unresolved)
	}
}
