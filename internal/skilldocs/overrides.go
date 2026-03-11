package skilldocs

import "strings"

// ApplyCLIOverrides keeps older downloaded Breyta skill bundles compatible with
// current CLI guidance. New bundles should already contain the canonical
// doctrine and typically require no changes here.
func ApplyCLIOverrides(skillSlug string, files map[string][]byte) map[string][]byte {
	if strings.TrimSpace(skillSlug) != "breyta" {
		return files
	}
	raw, ok := files["SKILL.md"]
	if !ok || len(raw) == 0 {
		return files
	}

	original := string(raw)
	updated := original
	replacements := [][2]string{
		{
			"- Before creating a new flow, search existing definitions: `breyta flows search <query>`.",
			"- Use `breyta flows list` and `breyta flows show <slug>` when you are modifying, debugging, reviewing, or explaining an existing workspace flow.\n- Use `breyta flows search <query>` when you are creating something new, exploring reusable patterns, or the relevant local flow is unknown.",
		},
		{
			"- Before creating a new flow, inspect workspace flows first: `breyta flows list` then `breyta flows show <slug>`.\n- Use `breyta flows search <query>` only for approved template discovery/reuse.",
			"- Use `breyta flows list` and `breyta flows show <slug>` when you are modifying, debugging, reviewing, or explaining an existing workspace flow.\n- Use `breyta flows search <query>` when you are creating something new, exploring reusable patterns, or the relevant local flow is unknown.",
		},
		{
			"3. Confirm reusable resources:\n   - `breyta connections list`\n   - `breyta flows search <query>`",
			"3. Confirm reusable resources:\n   - `breyta connections list`\n   - Existing or local flow work: `breyta flows list` then `breyta flows show <slug>`\n   - New or reusable-pattern work: `breyta flows search <query>`",
		},
	}

	for _, pair := range replacements {
		updated = strings.ReplaceAll(updated, pair[0], pair[1])
	}
	if updated == original {
		return files
	}

	cloned := make(map[string][]byte, len(files))
	for k, v := range files {
		cloned[k] = v
	}
	cloned["SKILL.md"] = []byte(updated)
	return cloned
}
