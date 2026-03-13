package skilldocs

import (
	"strings"
	"testing"
)

func TestApplyCLIOverrides_BreytaSkillAddsRecoveryURLGuidance(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Non-Negotiables",
			"- Include web links from CLI JSON when available (`meta.webUrl` / `data.*.webUrl`) so users can inspect in Breyta web.",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "Prefer exact recovery URLs from failures when available: `error.actions[].url` first, then `meta.webUrl`.") {
		t.Fatalf("expected recovery URL priority guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "Only derive canonical recovery URLs when the needed ids are already known") {
		t.Fatalf("expected canonical recovery URL guardrail, got:\n%s", body)
	}
	if !strings.Contains(body, "include the exact recovery URL in `Runtime proof`") {
		t.Fatalf("expected runtime proof recovery URL guidance, got:\n%s", body)
	}
}
