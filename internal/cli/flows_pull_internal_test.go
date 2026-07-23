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

func TestReconcilePulledDraftVisibilityTreatsMissingMetadataAsPrivate(t *testing.T) {
	source := `{:slug :example
 :discover {:public true}
 :marketplace {:visible true}
 :flow '(identity 1)}`
	data := map[string]any{"flow": map[string]any{}}

	got, err := reconcilePulledDraftVisibility(source, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ":discover {:public false}") ||
		!strings.Contains(got, ":marketplace {:visible false}") {
		t.Fatalf("missing canonical visibility should default to private:\n%s", got)
	}
}

func TestReconcilePulledDraftVisibilityPreservesReaderDiscards(t *testing.T) {
	source := `{:slug :example
 #_ #_ :discarded-key :discarded-value
 :discover {:public false}
 :flow '(identity 1)}`
	data := map[string]any{
		"flow": map[string]any{
			"discover": map[string]any{"public": true},
		},
	}

	got, err := reconcilePulledDraftVisibility(source, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "#_ #_ :discarded-key :discarded-value") ||
		!strings.Contains(got, ":discover {:public true}") {
		t.Fatalf("reader discard or visibility was changed incorrectly:\n%s", got)
	}
}

func TestReconcilePulledDraftVisibilityPreservesLeadingNestedReaderDiscards(t *testing.T) {
	source := `#_ #_ :discarded-inner :discarded-outer
{:slug :example
 :discover {:public false}
 :flow '(identity 1)}`
	data := map[string]any{
		"flow": map[string]any{
			"discover": map[string]any{"public": true},
		},
	}

	got, err := reconcilePulledDraftVisibility(source, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "#_ #_ :discarded-inner :discarded-outer\n") ||
		!strings.Contains(got, ":discover {:public true}") {
		t.Fatalf("leading reader discard or visibility was changed incorrectly:\n%s", got)
	}
}

func TestReconcilePulledDraftVisibilitySupportsStringKeys(t *testing.T) {
	source := `{:slug :example
 "discover" {"public" true}
 "marketplace" {"visible" true "app" {"app-id" "example"}}
 :flow '(identity 1)}`
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
	for _, want := range []string{
		`"discover" {"public" false}`,
		`"marketplace" {"visible" false`,
		`"app" {"app-id" "example"}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("string-key visibility source is missing %q:\n%s", want, got)
		}
	}
}

func TestReconcilePulledDraftVisibilityPreservesMetadataWrappedMaps(t *testing.T) {
	source := `{:slug :example
 :discover ^{:doc "keep"} {:public false :other true}
 :flow '(identity 1)}`
	data := map[string]any{
		"flow": map[string]any{
			"discover": map[string]any{"public": true},
		},
	}

	got, err := reconcilePulledDraftVisibility(source, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`:discover ^{:doc "keep"} {`,
		`:public true`,
		`:other true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("metadata-wrapped visibility source is missing %q:\n%s", want, got)
		}
	}
}

func TestReconcilePulledDraftVisibilityPreservesLegacyMetadataForms(t *testing.T) {
	source := `#^{:doc "flow"} {:slug :example
 :discover #^{:doc "discover"} {:public false :other true}
 :flow '(identity 1)}`
	data := map[string]any{
		"flow": map[string]any{
			"discover": map[string]any{"public": true},
		},
	}

	got, err := reconcilePulledDraftVisibility(source, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`#^{:doc "flow"} {`,
		`:discover #^{:doc "discover"} {`,
		`:public true`,
		`:other true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("legacy metadata source is missing %q:\n%s", want, got)
		}
	}
}
