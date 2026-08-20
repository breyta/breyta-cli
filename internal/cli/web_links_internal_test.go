package cli

import "testing"

func TestParseRunResourceURIPreservesEncodedStepSlash(t *testing.T) {
	workflowID, stepID, kind := parseRunResourceURI("res://v1/ws/ws-acme/result/run/wf-123/step/tools%2Ffetch-order/output")
	if workflowID != "wf-123" || stepID != "tools/fetch-order" || kind != "output" {
		t.Fatalf("unexpected parsed resource: workflow=%q step=%q kind=%q", workflowID, stepID, kind)
	}
}
