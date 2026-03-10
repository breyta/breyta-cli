package cli

import "testing"

func TestResourceDisplayNameForCLI_Precedence(t *testing.T) {
	t.Run("prefers explicit label over filename/path", func(t *testing.T) {
		item := map[string]any{
			"uri":          "res://v1/ws/ws-1/result/blob/demo/results%2Foutput.edn",
			"display-name": "Quarterly report",
			"adapter": map[string]any{
				"details": map[string]any{
					"filename": "report.json",
					"path":     "workspaces/ws-1/persist/demo/report.json",
				},
			},
		}
		if got := resourceDisplayNameForCLI(item); got != "Quarterly report" {
			t.Fatalf("expected explicit display-name, got %q", got)
		}
	})

	t.Run("falls back to adapter filename", func(t *testing.T) {
		item := map[string]any{
			"uri": "res://v1/ws/ws-1/result/blob/demo/results%2Foutput.edn",
			"adapter": map[string]any{
				"details": map[string]any{
					"filename": "README-demo.txt",
				},
			},
		}
		if got := resourceDisplayNameForCLI(item); got != "README-demo.txt" {
			t.Fatalf("expected adapter filename fallback, got %q", got)
		}
	})

	t.Run("falls back to path basename before URI segment", func(t *testing.T) {
		item := map[string]any{
			"uri": "res://v1/ws/ws-1/result/blob/demo/results%2Foutput.edn",
			"adapter": map[string]any{
				"details": map[string]any{
					"path": "workspaces/ws-1/persist/demo/weekly-summary.md",
				},
			},
		}
		if got := resourceDisplayNameForCLI(item); got != "weekly-summary.md" {
			t.Fatalf("expected path basename fallback, got %q", got)
		}
	})

	t.Run("falls back to decoded URI segment and preserves plus", func(t *testing.T) {
		item := map[string]any{
			"uri": "res://v1/ws/ws-1/result/blob/demo/results%2Ffoo+bar.csv",
		}
		if got := resourceDisplayNameForCLI(item); got != "foo+bar.csv" {
			t.Fatalf("expected decoded URI segment fallback, got %q", got)
		}
	})
}

func TestResourceSourceLabelForCLI(t *testing.T) {
	t.Run("uses flow and run provenance when available", func(t *testing.T) {
		item := map[string]any{
			"type":     "result",
			"flowSlug": "end-user-install-demo",
			"uri":      "res://v1/ws/ws-1/result/run/wf-123/flow-output",
			"adapter": map[string]any{
				"details": map[string]any{
					"path": "workspaces/ws-1/runs/wf-123/demo-result.json",
				},
			},
		}
		if got := resourceSourceLabelForCLI(item); got != "flow end-user-install-demo • run wf-123" {
			t.Fatalf("expected provenance source-label, got %q", got)
		}
	})

	t.Run("falls back to workspace file/saved result by type", func(t *testing.T) {
		fileItem := map[string]any{"type": "file", "uri": "res://v1/ws/ws-1/file/demo"}
		if got := resourceSourceLabelForCLI(fileItem); got != "workspace file" {
			t.Fatalf("expected workspace file fallback, got %q", got)
		}

		resultItem := map[string]any{"type": "result", "uri": "res://v1/ws/ws-1/result/blob/demo"}
		if got := resourceSourceLabelForCLI(resultItem); got != "saved result" {
			t.Fatalf("expected saved result fallback, got %q", got)
		}
	})

	t.Run("respects explicit source label", func(t *testing.T) {
		item := map[string]any{
			"type":         "result",
			"uri":          "res://v1/ws/ws-1/result/blob/demo",
			"source-label": "custom source",
		}
		if got := resourceSourceLabelForCLI(item); got != "custom source" {
			t.Fatalf("expected explicit source-label, got %q", got)
		}
	})
}

func TestEnrichResourceListPayload_AddsFriendlyFields(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{
				"type":     "result",
				"flowSlug": "end-user-install-demo",
				"uri":      "res://v1/ws/ws-1/result/run/wf-123/flow-output",
				"adapter": map[string]any{
					"details": map[string]any{
						"path": "workspaces/ws-1/runs/wf-123/demo-result.json",
					},
				},
			},
		},
	}

	enrichResourceListPayload(payload)

	items, _ := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	item, _ := items[0].(map[string]any)
	if item == nil {
		t.Fatalf("expected map item")
	}
	if got := asString(item, "display-name"); got != "demo-result.json" {
		t.Fatalf("expected display-name=demo-result.json, got %q", got)
	}
	if got := asString(item, "source-label"); got != "flow end-user-install-demo • run wf-123" {
		t.Fatalf("expected source-label provenance, got %q", got)
	}
}
