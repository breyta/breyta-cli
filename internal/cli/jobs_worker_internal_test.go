package cli

import (
	"os"
	"testing"
	"time"
)

func TestPrepareJobsWorkerFiles_PersistsPrivateStateFiles(t *testing.T) {
	job := map[string]any{
		"jobId":   "job-1",
		"jobType": "demo.echo",
		"payload": map[string]any{
			"secret": "top-secret",
		},
	}

	jobDir, jobFile, payloadFile, resultFile, contextFile, err := prepareJobsWorkerFiles(job)
	if err != nil {
		t.Fatalf("prepareJobsWorkerFiles: %v", err)
	}
	defer os.RemoveAll(jobDir)

	if err := writeJobsWorkerJSONFile(resultFile, map[string]any{"status": "succeeded"}); err != nil {
		t.Fatalf("writeJobsWorkerJSONFile(result): %v", err)
	}

	dirInfo, err := os.Stat(jobDir)
	if err != nil {
		t.Fatalf("stat jobDir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected jobDir perms 0700, got %o", got)
	}

	for _, path := range []string{jobFile, payloadFile, resultFile, contextFile} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("expected %s perms 0600, got %o", path, got)
		}
	}
}

func TestJobsWorkerHeartbeatInterval_BoundsToLeaseDuration(t *testing.T) {
	tests := []struct {
		name          string
		leaseDuration time.Duration
		want          time.Duration
	}{
		{name: "zero", leaseDuration: 0, want: 0},
		{name: "short lease", leaseDuration: 3 * time.Second, want: 1500 * time.Millisecond},
		{name: "medium lease", leaseDuration: 6 * time.Second, want: 3 * time.Second},
		{name: "ten second lease", leaseDuration: 10 * time.Second, want: 5 * time.Second},
		{name: "long lease", leaseDuration: 12 * time.Second, want: 6 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := jobsWorkerHeartbeatInterval(tc.leaseDuration)
			if got != tc.want {
				t.Fatalf("jobsWorkerHeartbeatInterval(%s) = %s, want %s", tc.leaseDuration, got, tc.want)
			}
			if tc.leaseDuration > 0 && got > 0 && got >= tc.leaseDuration {
				t.Fatalf("heartbeat interval %s must be < lease duration %s", got, tc.leaseDuration)
			}
		})
	}
}
