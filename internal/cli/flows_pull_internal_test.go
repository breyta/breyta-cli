package cli

import (
	"strings"
	"testing"
)

func TestReconcilePulledDraftVisibilityUpdatesOnlyCanonicalFlags(t *testing.T) {
	source := `{:slug :example
 :discover {:public true}
 :marketplace {:visible true :app {:app-id "example"}}
 :flow '(re-seq #"<loc>\s*([^<]+)\s*</loc>" xml)}`
	data := map[string]any{
		"flow": map[string]any{
			"discover":    map[string]any{"public": false},
			"marketplace": map[string]any{"visible": false},
		},
	}

	got, err := reconcilePulledDraftVisibility(source, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ":discover {:public false}") ||
		!strings.Contains(got, ":marketplace {:visible false") {
		t.Fatalf("visibility was not reconciled:\n%s", got)
	}
	if !strings.Contains(got, `:app {:app-id "example"}`) ||
		!strings.Contains(got, `#"<loc>\s*([^<]+)\s*</loc>"`) {
		t.Fatalf("unrelated source changed:\n%s", got)
	}
}

func TestReconcilePulledDraftVisibilityAddsMissingTrueFlags(t *testing.T) {
	source := "{:slug :example\n :flow '(identity 1)}"
	data := map[string]any{
		"flow": map[string]any{
			"discover":    map[string]any{"public": true},
			"marketplace": map[string]any{"visible": true},
		},
	}

	got, err := reconcilePulledDraftVisibility(source, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		":discover {:public true}",
		":marketplace {:visible true}",
		":flow '(identity 1)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("reconciled source is missing %q:\n%s", want, got)
		}
	}
	if _, err := extractTopLevelMapEntries(got); err != nil {
		t.Fatalf("reconciled source is invalid: %v\n%s", err, got)
	}
}

func TestReconcilePulledDraftVisibilityLeavesOmittedFalseFlagsAlone(t *testing.T) {
	source := "{:slug :example :flow '(identity 1)}"
	data := map[string]any{
		"flow": map[string]any{
			"discover":    map[string]any{"public": false},
			"marketplace": map[string]any{"visible": false},
		},
	}

	got, err := reconcilePulledDraftVisibility(source, data)
	if err != nil {
		t.Fatal(err)
	}
	if got != source {
		t.Fatalf("omitted false flags should not add source noise:\n%s", got)
	}
}
