package skilldocs

import (
	"strings"
	"testing"
)

func TestApplyCLIOverrides_BreytaSkillRewritesLegacyDiscoveryGuidance(t *testing.T) {
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
	if !strings.Contains(body, "Use `breyta flows list` and `breyta flows show <slug>` when you are modifying, debugging, reviewing, or explaining an existing workspace flow.") {
		t.Fatalf("expected existing-flow discovery guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "Use `breyta flows search <query>` when you are creating something new, exploring reusable patterns, or the relevant local flow is unknown.") {
		t.Fatalf("expected reusable-pattern discovery guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "Existing or local flow work: `breyta flows list` then `breyta flows show <slug>`") {
		t.Fatalf("expected preflight discovery guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "New or reusable-pattern work: `breyta flows search <query>`") {
		t.Fatalf("expected preflight search guidance, got:\n%s", body)
	}
	if string(got["references/x.md"]) != "ref" {
		t.Fatalf("expected non-skill files preserved")
	}
}

func TestApplyCLIOverrides_BreytaSkillLeavesCanonicalBodyAlone(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Discovery Rule (Required)",
			"- Use `breyta flows list` and `breyta flows show <slug>` when you are modifying an existing workspace flow.",
			"- Use `breyta flows search <query>` when you are creating something new.",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	if string(got["SKILL.md"]) != string(input["SKILL.md"]) {
		t.Fatalf("expected canonical body to remain unchanged, got:\n%s", string(got["SKILL.md"]))
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
