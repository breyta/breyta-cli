package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeBreytaAgentWorkspace(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "flows"), 0o755); err != nil {
		t.Fatalf("mkdir flows: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tmp", "flows"), 0o755); err != nil {
		t.Fatalf("mkdir tmp/flows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func TestFindAgentWorkspaceRoot_FindsNearestAgentsFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "flows", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	writeBreytaAgentWorkspace(t, root)

	got, found, err := findAgentWorkspaceRoot(nested)
	if err != nil {
		t.Fatalf("find root: %v", err)
	}
	if !found {
		t.Fatalf("expected workspace root to be found")
	}
	if got != root {
		t.Fatalf("expected root %q, got %q", root, got)
	}
}

func TestFindAgentWorkspaceRoot_IgnoresGenericAgentsFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# generic\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	got, found, err := findAgentWorkspaceRoot(nested)
	if err != nil {
		t.Fatalf("find root: %v", err)
	}
	if found {
		t.Fatalf("expected no workspace root, got %q", got)
	}
}

func TestFindAgentWorkspaceRoot_PrefersNearestValidWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# generic\n"), 0o644); err != nil {
		t.Fatalf("write parent AGENTS.md: %v", err)
	}
	workspaceRoot := filepath.Join(root, "breyta-workspace")
	writeBreytaAgentWorkspace(t, workspaceRoot)
	nested := filepath.Join(workspaceRoot, "flows", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, found, err := findAgentWorkspaceRoot(nested)
	if err != nil {
		t.Fatalf("find root: %v", err)
	}
	if !found {
		t.Fatalf("expected workspace root to be found")
	}
	if got != workspaceRoot {
		t.Fatalf("expected root %q, got %q", workspaceRoot, got)
	}
}

func TestRecordConsultedFlowFromStart_PersistsRecencyWithoutDuplicates(t *testing.T) {
	root := t.TempDir()
	writeBreytaAgentWorkspace(t, root)

	if err := recordConsultedFlowFromStart(root, provenanceSourceRef{WorkspaceID: "ws-1", FlowSlug: "alpha"}); err != nil {
		t.Fatalf("record alpha: %v", err)
	}
	if err := recordConsultedFlowFromStart(root, provenanceSourceRef{WorkspaceID: "ws-1", FlowSlug: "beta"}); err != nil {
		t.Fatalf("record beta: %v", err)
	}
	if err := recordConsultedFlowFromStart(root, provenanceSourceRef{WorkspaceID: "ws-1", FlowSlug: "alpha"}); err != nil {
		t.Fatalf("record alpha again: %v", err)
	}

	refs, err := loadConsultedFlowRefsFromStart(root)
	if err != nil {
		t.Fatalf("load refs: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].FlowSlug != "beta" || refs[1].FlowSlug != "alpha" {
		t.Fatalf("expected recency order [beta alpha], got %#v", refs)
	}
	if refs[1].ConsultedAt == "" {
		t.Fatalf("expected consultedAt to be populated")
	}
}

func TestCurrentProvenanceCandidates_FiltersCurrentFlow(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "flows")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	writeBreytaAgentWorkspace(t, root)
	if err := saveConsultedFlowRefsFromStart(root, []provenanceSourceRef{
		{WorkspaceID: "ws-1", FlowSlug: "source-a"},
		{WorkspaceID: "ws-1", FlowSlug: "target-flow"},
		{WorkspaceID: "ws-2", FlowSlug: "source-b"},
	}); err != nil {
		t.Fatalf("save refs: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	refs, err := currentProvenanceCandidates("ws-1", "target-flow")
	if err != nil {
		t.Fatalf("current candidates: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 filtered refs, got %d", len(refs))
	}
	if refs[0].FlowSlug != "source-a" || refs[1].FlowSlug != "source-b" {
		t.Fatalf("unexpected filtered refs: %#v", refs)
	}
}

func TestParseProvenanceSourceRef_UsesCurrentWorkspaceWhenOmitted(t *testing.T) {
	ref, err := parseProvenanceSourceRef("demo-flow", "ws-1")
	if err != nil {
		t.Fatalf("parse source ref: %v", err)
	}
	if ref.WorkspaceID != "ws-1" || ref.FlowSlug != "demo-flow" {
		t.Fatalf("unexpected ref: %#v", ref)
	}
}
