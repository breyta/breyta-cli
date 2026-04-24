package cli

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/breyta/breyta-cli/internal/api"
)

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

	t.Run("rejects unrelated connection types even with llm-like backend", func(t *testing.T) {
		conn := connectionSummary{
			ID:      "conn-db-openai",
			Name:    "OpenAI Warehouse",
			Type:    "postgres",
			Backend: "openai",
			BaseURL: "https://api.openai.com/v1",
		}
		if got := compatibleLLMBackend(conn); got != "" {
			t.Fatalf("expected non-http/non-llm connection to be rejected, got %q", got)
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

func TestBuildConfigureSuggestionsRespectsRequiredLLMBackends(t *testing.T) {
	requirements := []any{
		map[string]any{
			"slot":     "gpt-ai",
			"type":     "llm-provider",
			"backends": []any{"openai"},
		},
		map[string]any{
			"slot":     "gemini-ai",
			"type":     "llm-provider",
			"backends": []any{"google"},
		},
		map[string]any{
			"slot":     "claude-ai",
			"type":     "llm-provider",
			"backends": []any{"anthropic"},
		},
	}
	connectionsByType := map[string][]connectionSummary{
		"llm-provider": {
			{ID: "conn-openai", Name: "GPT Local", Type: "llm-provider", Backend: "openai", BaseURL: "https://api.openai.com/v1"},
			{ID: "conn-gemini", Name: "Gemini Local", Type: "llm-provider", Backend: "google-ai", BaseURL: "https://generativelanguage.googleapis.com/v1beta"},
			{ID: "conn-claude", Name: "Claude Local", Type: "llm-provider", Backend: "anthropic", BaseURL: "https://api.anthropic.com"},
		},
	}

	rows, setArgs, unresolved := buildConfigureSuggestions(requirements, map[string]string{}, connectionsByType)
	if len(rows) != 3 {
		t.Fatalf("expected three suggestion rows, got %#v", rows)
	}
	got := map[string]string{}
	for _, row := range rows {
		got[row.Slot] = row.SuggestedConnectionID
		if row.Status != "suggested" {
			t.Fatalf("expected suggested status for %s, got %#v", row.Slot, row)
		}
	}
	if got["gpt-ai"] != "conn-openai" || got["gemini-ai"] != "conn-gemini" || got["claude-ai"] != "conn-claude" {
		t.Fatalf("unexpected backend-specific suggestions: %#v", got)
	}
	if len(setArgs) != 3 {
		t.Fatalf("expected three set args, got %#v", setArgs)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved slots, got %#v", unresolved)
	}
}

func TestBuildConfigureSuggestionsReportsMissingRequiredLLMBackend(t *testing.T) {
	requirements := []any{
		map[string]any{
			"slot":     "gpt-ai",
			"type":     "llm-provider",
			"backends": []any{"openai"},
		},
	}
	connectionsByType := map[string][]connectionSummary{
		"llm-provider": {
			{ID: "conn-gemini", Name: "Gemini Local", Type: "llm-provider", Backend: "google-ai", BaseURL: "https://generativelanguage.googleapis.com/v1beta"},
			{ID: "conn-claude", Name: "Claude Local", Type: "llm-provider", Backend: "anthropic", BaseURL: "https://api.anthropic.com"},
		},
	}

	rows, setArgs, unresolved := buildConfigureSuggestions(requirements, map[string]string{}, connectionsByType)
	if len(rows) != 1 {
		t.Fatalf("expected one suggestion row, got %#v", rows)
	}
	if rows[0].Status != "unresolved" {
		t.Fatalf("expected unresolved status, got %#v", rows[0])
	}
	if rows[0].Reason != "no matching connections found for required backends openai" {
		t.Fatalf("unexpected reason: %#v", rows[0].Reason)
	}
	if len(setArgs) != 0 {
		t.Fatalf("expected no set args, got %#v", setArgs)
	}
	if len(unresolved) != 1 || unresolved[0] != "gpt-ai" {
		t.Fatalf("expected unresolved gpt-ai, got %#v", unresolved)
	}
}

func TestBuildConfigureSuggestionsCanonicalizesRequiredLLMBackends(t *testing.T) {
	requirements := []any{
		map[string]any{
			"slot":     "azure-ai",
			"type":     "llm-provider",
			"backends": []any{"azure-openai"},
		},
	}
	connectionsByType := map[string][]connectionSummary{
		"llm-provider": {
			{ID: "conn-azure", Name: "Azure OpenAI", Type: "llm-provider", Backend: "azure-openai", BaseURL: "https://example.openai.azure.com"},
		},
	}

	rows, setArgs, unresolved := buildConfigureSuggestions(requirements, map[string]string{}, connectionsByType)
	if len(rows) != 1 {
		t.Fatalf("expected one suggestion row, got %#v", rows)
	}
	if rows[0].Status != "suggested" {
		t.Fatalf("expected suggested status, got %#v", rows[0])
	}
	if rows[0].SuggestedConnectionID != "conn-azure" {
		t.Fatalf("unexpected suggested connection: %#v", rows[0])
	}
	if len(setArgs) != 1 || setArgs[0] != "azure-ai.conn=conn-azure" {
		t.Fatalf("unexpected set args: %#v", setArgs)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved slots, got %#v", unresolved)
	}
}

func TestBuildConfigureSuggestionsPreservesExplicitOpenAICompatibleBackends(t *testing.T) {
	requirements := []any{
		map[string]any{
			"slot":     "router",
			"type":     "llm-provider",
			"backends": []any{"openrouter"},
		},
		map[string]any{
			"slot":     "mistral",
			"type":     "llm-provider",
			"backends": []any{"mistral"},
		},
	}
	connectionsByType := map[string][]connectionSummary{
		"llm-provider": {
			{ID: "conn-router", Name: "OpenRouter", Type: "llm-provider", Backend: "openrouter"},
			{ID: "conn-mistral", Name: "Mistral", Type: "llm-provider", Backend: "mistral"},
		},
	}

	rows, setArgs, unresolved := buildConfigureSuggestions(requirements, map[string]string{}, connectionsByType)
	if len(rows) != 2 {
		t.Fatalf("expected two suggestion rows, got %#v", rows)
	}
	bySlot := map[string]configureSuggestRow{}
	for _, row := range rows {
		bySlot[row.Slot] = row
	}
	if bySlot["router"].SuggestedConnectionID != "conn-router" {
		t.Fatalf("unexpected router suggestion: %#v", bySlot["router"])
	}
	if bySlot["mistral"].SuggestedConnectionID != "conn-mistral" {
		t.Fatalf("unexpected mistral suggestion: %#v", bySlot["mistral"])
	}
	if len(setArgs) != 2 {
		t.Fatalf("expected two set args, got %#v", setArgs)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved slots, got %#v", unresolved)
	}
}

func TestListConnectionsByTypeSkipsHTTPFallbackWhenExactLLMProviderExists(t *testing.T) {
	var calls []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Query().Get("type"))
		if r.URL.Query().Get("type") != "llm-provider" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "unexpected type"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{
					"connection-id": "conn-llm-openai",
					"name":          "OpenAI LLM",
					"type":          "llm-provider",
					"backend":       "openai",
				},
			},
		})
	}))
	defer srv.Close()

	connectionsByType, err := listConnectionsByType(api.Client{
		BaseURL:     srv.URL,
		WorkspaceID: "ws-test",
		HTTP:        srv.Client(),
	}, []any{
		map[string]any{"slot": "llm", "type": "llm-provider"},
	})
	if err != nil {
		t.Fatalf("listConnectionsByType returned error: %v", err)
	}
	if len(calls) != 1 || calls[0] != "llm-provider" {
		t.Fatalf("expected only llm-provider query, got %#v", calls)
	}
	if got := len(connectionsByType["llm-provider"]); got != 1 {
		t.Fatalf("expected one llm-provider candidate, got %d", got)
	}
}

func TestListConnectionsByTypeFallsBackToHTTPForLLMProvider(t *testing.T) {
	var calls []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typ := r.URL.Query().Get("type")
		calls = append(calls, typ)
		switch typ {
		case "llm-provider":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
		case "http-api":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{
						"connection-id": "conn-http-openai",
						"name":          "OpenAI HTTP",
						"type":          "http-api",
						"config": map[string]any{
							"base-url": "https://api.openai.com/v1",
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "unexpected type"}})
		}
	}))
	defer srv.Close()

	connectionsByType, err := listConnectionsByType(api.Client{
		BaseURL:     srv.URL,
		WorkspaceID: "ws-test",
		HTTP:        srv.Client(),
	}, []any{
		map[string]any{"slot": "llm", "type": "llm-provider"},
	})
	if err != nil {
		t.Fatalf("listConnectionsByType returned error: %v", err)
	}
	if len(calls) != 2 || calls[0] != "llm-provider" || calls[1] != "http-api" {
		t.Fatalf("expected llm-provider then http-api fallback, got %#v", calls)
	}
	if got := len(connectionsByType["llm-provider"]); got != 1 {
		t.Fatalf("expected one llm-provider candidate from HTTP fallback, got %d", got)
	}
	if connectionsByType["llm-provider"][0].ID != "conn-http-openai" {
		t.Fatalf("expected HTTP fallback candidate, got %#v", connectionsByType["llm-provider"][0])
	}
}

func TestListConnectionsByTypeSkipsInvalidConnections(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "http-api" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "unexpected type"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{
					"connection-id": "conn-invalid",
					"name":          "GitHub Broken",
					"type":          "http-api",
					"backend":       "rest",
					"_invalid?":     true,
				},
				map[string]any{
					"connection-id": "conn-valid",
					"name":          "GitHub Public",
					"type":          "http-api",
					"backend":       "rest",
					"config": map[string]any{
						"auth": map[string]any{"type": "none"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	connectionsByType, err := listConnectionsByType(api.Client{
		BaseURL:     srv.URL,
		WorkspaceID: "ws-test",
		HTTP:        srv.Client(),
	}, []any{
		map[string]any{"slot": "github-api", "type": "http-api"},
	})
	if err != nil {
		t.Fatalf("listConnectionsByType returned error: %v", err)
	}
	got := connectionsByType["http-api"]
	if len(got) != 1 {
		t.Fatalf("expected one valid http-api connection, got %#v", got)
	}
	if got[0].ID != "conn-valid" {
		t.Fatalf("expected invalid connection to be skipped, got %#v", got)
	}
}
