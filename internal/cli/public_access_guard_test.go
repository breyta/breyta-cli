package cli

import "testing"

func TestFlowSourceRequestsPublicAccess(t *testing.T) {
	for _, source := range []string{
		`{:slug :demo :discover {:public true} :flow '(identity 1)}`,
		`{:slug :demo :marketplace {:visible true} :flow '(identity 1)}`,
	} {
		if !flowSourceRequestsPublicAccess(source) {
			t.Fatalf("expected public access source to be detected: %s", source)
		}
	}

	for _, source := range []string{
		`{:slug :demo :discover {:public false} :marketplace {:visible false}}`,
		`{:slug :demo :flow '(identity 1)}`,
	} {
		if flowSourceRequestsPublicAccess(source) {
			t.Fatalf("expected non-public source to pass: %s", source)
		}
	}
}
