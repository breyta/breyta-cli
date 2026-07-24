package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/breyta/breyta-cli/internal/state"
)

func parseCSV(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range splitNonEmpty(s) {
		out[p] = true
	}
	return out
}

func splitNonEmpty(s string) []string {
	parts := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func findStep(f *state.Flow, id string) (state.FlowStep, bool) {
	for _, s := range f.Steps {
		if s.ID == id {
			return s, true
		}
	}
	return state.FlowStep{}, false
}

func maxVersion(f *state.Flow) int {
	m := 0
	for _, v := range f.Versions {
		if v.Version > m {
			m = v.Version
		}
	}
	if f.ActiveVersion > m {
		m = f.ActiveVersion
	}
	return m
}

func flowRecordForSource(f *state.Flow, source string) (state.FlowRecord, string, int, error) {
	switch source {
	case "draft":
		return flowRecordFromFlow(f), "draft", 0, nil
	case "current":
		fallthrough
	case "active":
		if f.ActiveVersion > 0 {
			if v, ok := findVersion(f, f.ActiveVersion); ok {
				if source == "current" {
					return v.Flow, "current", v.Version, nil
				}
				return v.Flow, "active", v.Version, nil
			}
		}
		if len(f.Versions) == 0 {
			return flowRecordFromFlow(f), "draft", 0, nil
		}
		return state.FlowRecord{}, "", 0, errors.New("current/active version not found; push then release first")
	case "latest":
		if len(f.Versions) == 0 {
			return flowRecordFromFlow(f), "draft", 0, nil
		}
		latest := f.Versions[0]
		for _, v := range f.Versions[1:] {
			if v.Version > latest.Version {
				latest = v
			}
		}
		return latest.Flow, "latest", latest.Version, nil
	default:
		return state.FlowRecord{}, "", 0, fmt.Errorf("invalid source %q (expected current or latest)", source)
	}
}

func flowRecordFromFlow(f *state.Flow) state.FlowRecord {
	return state.FlowRecord{
		Name:        f.Name,
		Description: f.Description,
		Tags:        f.Tags,
		Spine:       f.Spine,
		Steps:       f.Steps,
	}
}

func findVersion(f *state.Flow, version int) (state.FlowVersion, bool) {
	for _, v := range f.Versions {
		if v.Version == version {
			return v, true
		}
	}
	return state.FlowVersion{}, false
}

func diffSteps(a, b []state.FlowStep) map[string]any {
	am := map[string]state.FlowStep{}
	bm := map[string]state.FlowStep{}
	for _, s := range a {
		am[s.ID] = s
	}
	for _, s := range b {
		bm[s.ID] = s
	}
	added := []string{}
	removed := []string{}
	changed := []map[string]any{}
	for id, s := range bm {
		if _, ok := am[id]; !ok {
			added = append(added, id)
			continue
		}
		sa := am[id]
		if sa.Type != s.Type || sa.Title != s.Title {
			changed = append(changed, map[string]any{"id": id, "from": map[string]any{"type": sa.Type, "title": sa.Title}, "to": map[string]any{"type": s.Type, "title": s.Title}})
		}
	}
	for id := range am {
		if _, ok := bm[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return map[string]any{"added": added, "removed": removed, "changed": changed}
}

func buildSpine(f *state.Flow) []string {
	lines := make([]string, 0, len(f.Steps))
	for _, s := range f.Steps {
		lines = append(lines, fmt.Sprintf("%s (%s) %s", s.ID, s.Type, s.Title))
	}
	return lines
}
