package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	consultedFlowsStateVersion = 1
	maxConsultedFlows          = 25
)

var consultedFlowsStateRelativePath = filepath.Join("tmp", "consulted-flows.json")

type provenanceSourceRef struct {
	WorkspaceID string `json:"workspaceId"`
	FlowSlug    string `json:"flowSlug"`
	ConsultedAt string `json:"consultedAt,omitempty"`
}

type consultedFlowsState struct {
	Version     int                   `json:"version"`
	SourceFlows []provenanceSourceRef `json:"sourceFlows"`
}

func normalizeProvenanceSourceRef(ref provenanceSourceRef) (provenanceSourceRef, bool) {
	ref.WorkspaceID = strings.TrimSpace(ref.WorkspaceID)
	ref.FlowSlug = strings.TrimSpace(ref.FlowSlug)
	ref.ConsultedAt = strings.TrimSpace(ref.ConsultedAt)
	if ref.WorkspaceID == "" || ref.FlowSlug == "" {
		return provenanceSourceRef{}, false
	}
	return ref, true
}

func provenanceSourceRefKey(ref provenanceSourceRef) string {
	return ref.WorkspaceID + "\x00" + ref.FlowSlug
}

func dedupeProvenanceSourceRefs(refs []provenanceSourceRef) []provenanceSourceRef {
	if len(refs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(refs))
	out := make([]provenanceSourceRef, 0, len(refs))
	for _, ref := range refs {
		normalized, ok := normalizeProvenanceSourceRef(ref)
		if !ok {
			continue
		}
		key := provenanceSourceRefKey(normalized)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isBreytaAgentWorkspaceRoot(dir string) (bool, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false, nil
	}
	requiredPaths := []string{
		filepath.Join(dir, "AGENTS.md"),
		filepath.Join(dir, "flows"),
		filepath.Join(dir, "tmp", "flows"),
	}
	for _, path := range requiredPaths {
		st, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		if strings.HasSuffix(path, ".md") {
			if st.IsDir() {
				return false, nil
			}
			continue
		}
		if !st.IsDir() {
			return false, nil
		}
	}
	return true, nil
}

func findAgentWorkspaceRoot(start string) (string, bool, error) {
	start = strings.TrimSpace(start)
	if start == "" {
		return "", false, nil
	}
	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(absStart)
	if err != nil {
		return "", false, err
	}
	dir := absStart
	if !info.IsDir() {
		dir = filepath.Dir(absStart)
	}
	for {
		valid, err := isBreytaAgentWorkspaceRoot(dir)
		if err != nil {
			return "", false, err
		}
		if valid {
			return dir, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func consultedFlowsStatePathFromStart(start string) (string, bool, error) {
	root, found, err := findAgentWorkspaceRoot(start)
	if err != nil || !found {
		return "", found, err
	}
	return filepath.Join(root, consultedFlowsStateRelativePath), true, nil
}

func loadConsultedFlowRefsFromStart(start string) ([]provenanceSourceRef, error) {
	path, found, err := consultedFlowsStatePathFromStart(start)
	if err != nil || !found {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state consultedFlowsState
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, fmt.Errorf("read consulted flows state: %w", err)
	}
	return dedupeProvenanceSourceRefs(state.SourceFlows), nil
}

func saveConsultedFlowRefsFromStart(start string, refs []provenanceSourceRef) error {
	path, found, err := consultedFlowsStatePathFromStart(start)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	state := consultedFlowsState{
		Version:     consultedFlowsStateVersion,
		SourceFlows: dedupeProvenanceSourceRefs(refs),
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, append(b, '\n'), 0o644)
}

func recordConsultedFlowFromStart(start string, ref provenanceSourceRef) error {
	normalized, ok := normalizeProvenanceSourceRef(ref)
	if !ok {
		return nil
	}
	path, found, err := consultedFlowsStatePathFromStart(start)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	existing, err := loadConsultedFlowRefsFromStart(start)
	if err != nil {
		return err
	}
	normalized.ConsultedAt = time.Now().UTC().Format(time.RFC3339)
	key := provenanceSourceRefKey(normalized)
	updated := make([]provenanceSourceRef, 0, len(existing)+1)
	for _, candidate := range existing {
		if provenanceSourceRefKey(candidate) == key {
			continue
		}
		updated = append(updated, candidate)
	}
	updated = append(updated, normalized)
	if len(updated) > maxConsultedFlows {
		updated = updated[len(updated)-maxConsultedFlows:]
	}
	state := consultedFlowsState{
		Version:     consultedFlowsStateVersion,
		SourceFlows: updated,
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, append(b, '\n'), 0o644)
}

func recordConsultedFlow(ref provenanceSourceRef) error {
	start, err := os.Getwd()
	if err != nil {
		return err
	}
	return recordConsultedFlowFromStart(start, ref)
}

func currentConsultedFlowRefs() ([]provenanceSourceRef, error) {
	start, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return loadConsultedFlowRefsFromStart(start)
}

func currentProvenanceCandidates(targetWorkspaceID, targetFlowSlug string) ([]provenanceSourceRef, error) {
	refs, err := currentConsultedFlowRefs()
	if err != nil {
		return nil, err
	}
	targetWorkspaceID = strings.TrimSpace(targetWorkspaceID)
	targetFlowSlug = strings.TrimSpace(targetFlowSlug)
	if len(refs) == 0 {
		return nil, nil
	}
	filtered := make([]provenanceSourceRef, 0, len(refs))
	for _, ref := range refs {
		if targetFlowSlug != "" && ref.FlowSlug == targetFlowSlug &&
			(targetWorkspaceID == "" || ref.WorkspaceID == targetWorkspaceID) {
			continue
		}
		filtered = append(filtered, ref)
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	return filtered, nil
}

func provenanceCandidatesMetaItems(refs []provenanceSourceRef) []map[string]any {
	if len(refs) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		normalized, ok := normalizeProvenanceSourceRef(ref)
		if !ok {
			continue
		}
		item := map[string]any{
			"workspaceId": normalized.WorkspaceID,
			"flowSlug":    normalized.FlowSlug,
		}
		if normalized.ConsultedAt != "" {
			item["consultedAt"] = normalized.ConsultedAt
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func workspaceIDFromEnvelope(out map[string]any, fallback string) string {
	if out != nil {
		if workspaceID, _ := out["workspaceId"].(string); strings.TrimSpace(workspaceID) != "" {
			return strings.TrimSpace(workspaceID)
		}
	}
	return strings.TrimSpace(fallback)
}

func appendEnvelopeHints(out map[string]any, hints ...string) {
	if out == nil || len(hints) == 0 {
		return
	}
	seen := map[string]struct{}{}
	merged := make([]any, 0, len(hints))
	if raw, ok := out["_hints"].([]any); ok {
		for _, hintAny := range raw {
			hint, ok := hintAny.(string)
			if !ok {
				continue
			}
			hint = strings.TrimSpace(hint)
			if hint == "" {
				continue
			}
			if _, exists := seen[hint]; exists {
				continue
			}
			seen[hint] = struct{}{}
			merged = append(merged, hint)
		}
	}
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		if _, exists := seen[hint]; exists {
			continue
		}
		seen[hint] = struct{}{}
		merged = append(merged, hint)
	}
	if len(merged) > 0 {
		out["_hints"] = merged
	}
}

func appendProvenanceHints(out map[string]any, targetWorkspaceID, targetFlowSlug string) error {
	targetFlowSlug = strings.TrimSpace(targetFlowSlug)
	if out == nil || targetFlowSlug == "" {
		return nil
	}
	candidates, err := currentProvenanceCandidates(targetWorkspaceID, targetFlowSlug)
	if err != nil || len(candidates) == 0 {
		return err
	}
	meta := ensureMeta(out)
	if meta != nil {
		meta["provenanceCandidates"] = provenanceCandidatesMetaItems(candidates)
	}
	appendEnvelopeHints(
		out,
		"Persist consulted flow provenance: breyta flows provenance set "+targetFlowSlug+" --from-consulted",
		"Clear flow provenance intentionally: breyta flows provenance set "+targetFlowSlug+" --clear",
	)
	return nil
}

func parseProvenanceSourceRef(raw string, defaultWorkspaceID string) (provenanceSourceRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return provenanceSourceRef{}, fmt.Errorf("empty source flow reference")
	}
	workspaceID := strings.TrimSpace(defaultWorkspaceID)
	flowSlug := raw
	if strings.Contains(raw, "/") {
		parts := strings.SplitN(raw, "/", 2)
		workspaceID = strings.TrimSpace(parts[0])
		flowSlug = strings.TrimSpace(parts[1])
	}
	if workspaceID == "" {
		return provenanceSourceRef{}, fmt.Errorf("source flow %q is missing workspace id; use <workspace-id>/<flow-slug> or set --workspace", raw)
	}
	if !isAPIValidFlowSlug(flowSlug) {
		return provenanceSourceRef{}, fmt.Errorf("invalid source flow slug %q", flowSlug)
	}
	return provenanceSourceRef{WorkspaceID: workspaceID, FlowSlug: flowSlug}, nil
}

func provenanceSourceFlowPayloadItems(refs []provenanceSourceRef) []map[string]any {
	if len(refs) == 0 {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		normalized, ok := normalizeProvenanceSourceRef(ref)
		if !ok {
			continue
		}
		items = append(items, map[string]any{
			"workspaceId": normalized.WorkspaceID,
			"flowSlug":    normalized.FlowSlug,
		})
	}
	return items
}
