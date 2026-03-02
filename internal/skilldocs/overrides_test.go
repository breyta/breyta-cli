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

func TestApplyCLIOverrides_BreytaSkillAddsKeyedScheduleGuard(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Core learnings",
			"- Schedule triggers require a prod profile before deploy and activate",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "For keyed concurrency (`:type :keyed`), schedule `:config :input` must include the key-field") {
		t.Fatalf("expected keyed schedule guard in override, got:\n%s", body)
	}
	if !strings.Contains(body, "activation will fail") {
		t.Fatalf("expected explicit activation failure warning in override, got:\n%s", body)
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

func TestApplyCLIOverrides_BreytaSkillInjectsNamingConventions(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Non-Negotiables",
			"- Keep :flow orchestration-focused.",
			"",
			"## Capability Discovery",
			"- breyta docs",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "## Readability + Searchability Naming Conventions (Required)") {
		t.Fatalf("expected naming conventions section, got:\n%s", body)
	}
	if !strings.Contains(body, "default to Should we ...? framing when possible") {
		t.Fatalf("expected Should we framing guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "search tokens appear in :name, :description, and :tags") {
		t.Fatalf("expected search token guidance, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_DoesNotDuplicateNamingConventions(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Readability + Searchability Naming Conventions (Required)",
			"- existing content",
			"",
			"## Capability Discovery",
			"- breyta docs",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	count := strings.Count(body, "## Readability + Searchability Naming Conventions (Required)")
	if count != 1 {
		t.Fatalf("expected naming conventions header exactly once, got %d\n%s", count, body)
	}
}

func TestApplyCLIOverrides_DoesNotMatchSubHeadingCapabilityDiscovery(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Non-Negotiables",
			"- Keep :flow orchestration-focused.",
			"",
			"### Capability Discovery",
			"- legacy subsection",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "### Capability Discovery") {
		t.Fatalf("expected original H3 heading to remain, got:\n%s", body)
	}
	if strings.Contains(body, "## Readability + Searchability Naming Conventions (Required)\n\n### Capability Discovery") {
		t.Fatalf("unexpected insertion before H3 Capability Discovery heading:\n%s", body)
	}
	sectionPos := strings.Index(body, "## Readability + Searchability Naming Conventions (Required)")
	h3Pos := strings.Index(body, "### Capability Discovery")
	if sectionPos == -1 || h3Pos == -1 || sectionPos < h3Pos {
		t.Fatalf("expected conventions section appended after existing H3 subsection when H2 is absent, got:\n%s", body)
	}
}
