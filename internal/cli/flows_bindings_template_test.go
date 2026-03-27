package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestListConnectionsByTypeSkipsHTTPFallbackWhenExactLLMProviderExists(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
