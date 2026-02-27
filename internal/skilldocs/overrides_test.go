package skilldocs

import (
	"strings"
	"testing"
)

func TestApplyCLIOverrides_BreytaSkillRewritesSearchGuidance(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Non-Negotiables",
			"- Before creating a new flow, search existing definitions: `breyta flows search <query>`.",
			"",
			"## Preflight",
			"3. Confirm reusable resources:",
			"   - `breyta connections list`",
			"   - `breyta flows search <query>`",
		}, "\n")),
		"references/x.md": []byte("ref"),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "inspect workspace flows first: `breyta flows list` then `breyta flows show <slug>`") {
		t.Fatalf("expected workspace listing guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "Approved template discovery: `breyta flows search <query>`") {
		t.Fatalf("expected template discovery guidance in override, got:\n%s", body)
	}
	if string(got["references/x.md"]) != "ref" {
		t.Fatalf("expected non-skill files preserved")
	}
}

func TestApplyCLIOverrides_NonBreytaNoop(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte("hello"),
	}
	got := ApplyCLIOverrides("other-skill", input)
	if string(got["SKILL.md"]) != "hello" {
		t.Fatalf("expected no change for non-breyta skill")
	}
}
