package cli

import (
	"os"
	"testing"
)

func TestPrepareJobsWorkerFiles_PersistsPrivateStateFiles(t *testing.T) {
	job := map[string]any{
		"jobId":   "job-1",
		"jobType": "demo.echo",
		"payload": map[string]any{
			"secret": "top-secret",
		},
	}

	jobDir, jobFile, payloadFile, resultFile, err := prepareJobsWorkerFiles(job)
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

	for _, path := range []string{jobFile, payloadFile, resultFile} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("expected %s perms 0600, got %o", path, got)
		}
	}
}
