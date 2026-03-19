package cli

import "testing"

func TestParseRunsListQuery(t *testing.T) {
	filters, err := parseRunsListQuery("status:failed flow::my-flow installation:prof-1 version:7")
	if err != nil {
		t.Fatalf("parseRunsListQuery failed: %v", err)
	}
	if filters.Status != "failed" {
		t.Fatalf("expected status failed, got %q", filters.Status)
	}
	if filters.Flow != "my-flow" {
		t.Fatalf("expected flow my-flow, got %q", filters.Flow)
	}
	if filters.InstallationID != "prof-1" {
		t.Fatalf("expected installation prof-1, got %q", filters.InstallationID)
	}
	if !filters.HasVersion || filters.Version != 7 {
		t.Fatalf("expected version 7, got %+v", filters)
	}
	if got := buildRunsListQuery(filters); got != "status:failed flow:my-flow installation:prof-1 version:7" {
		t.Fatalf("unexpected rebuilt query: %q", got)
	}
}

func TestParseRunsListQuery_MultipleStatusesRejected(t *testing.T) {
	if _, err := parseRunsListQuery("status:failed status:completed"); err == nil {
		t.Fatalf("expected multiple statuses to be rejected")
	}
}

func TestParseRunsListQuery_AllowsTerminalStatuses(t *testing.T) {
	for _, status := range []string{"cancelled", "canceled", "terminated", "timed-out", "timed_out"} {
		filters, err := parseRunsListQuery("status:" + status)
		if err != nil {
			t.Fatalf("expected status %q to parse, got error: %v", status, err)
		}
		if filters.Status != status {
			t.Fatalf("expected status %q, got %q", status, filters.Status)
		}
	}
}
