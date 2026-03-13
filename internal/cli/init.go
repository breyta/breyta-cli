package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/breyta/breyta-cli/internal/skilldocs"
	"github.com/breyta/breyta-cli/skills"
	"github.com/spf13/cobra"
)

func newInitCmd(app *App) *cobra.Command {
	var provider string
	var dir string
	var force bool
	var noSkill bool
	var noWorkspace bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap local setup for agent-driven Breyta usage",
		Long: strings.TrimSpace(`
Sets up Breyta for agent-first usage:
- installs the Breyta agent skill bundle for Codex/Cursor/Claude Code/Gemini CLI
- creates a local "agent workspace" directory with an AGENTS.md file

It does not require authentication, but you'll typically run ` + "`breyta auth login`" + `
right after to connect the CLI to your Breyta account.
`),
		Example: strings.TrimSpace(`
# Install the skill bundle for your agent tool and create ./breyta-workspace
breyta init --provider codex

# Cursor, Claude Code, and Gemini CLI
breyta init --provider cursor
breyta init --provider claude
breyta init --provider gemini

# Use a specific directory and overwrite existing files
breyta init --dir ./my-breyta-workspace --force
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if noSkill && noWorkspace {
				return errors.New("nothing to do (both --no-skill and --no-workspace were set)")
			}

			p := skills.Provider(strings.TrimSpace(provider))
			if strings.TrimSpace(provider) == "" {
				p = skills.ProviderCodex
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			target, err := skills.Target(home, p)
			if err != nil {
				// In workspace-only mode, provider is irrelevant. Avoid failing init when users
				// pass an arbitrary --provider value along with --no-skill.
				if !noSkill {
					return err
				}
				// Best-effort: fall back to a known provider so AGENTS.md can still include
				// a concrete path example.
				target, _ = skills.Target(home, skills.ProviderCodex)
			}

			skillInstalled := false

			if !noSkill {
				ensureAPIURL(app)
				if strings.TrimSpace(app.APIURL) == "" {
					if noWorkspace {
						return errors.New("missing api base url (try `breyta auth login` first)")
					}
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: missing api base url; skipped skill install (try `breyta auth login`, then `breyta skills install`)")
				} else {
					_, files, err := skilldocs.FetchBundle(context.Background(), nil, app.APIURL, app.Token, skills.BreytaSkillSlug)
					if err != nil {
						if noWorkspace {
							return err
						}
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: skill bundle download failed (%v); continuing without skill install\n", err)
					} else if _, err := skills.InstallBreytaSkillFiles(home, p, skilldocs.ApplyCLIOverrides(skills.BreytaSkillSlug, files)); err != nil {
						if noWorkspace {
							return err
						}
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: skill install failed (%v); continuing without skill install\n", err)
					} else {
						skillInstalled = true

						fmt.Fprintf(cmd.OutOrStdout(), "Installed Breyta agent skill bundle for %s in %s\n", target.Provider, target.Dir)
					}
				}
			}

			if noWorkspace {
				return nil
			}

			wsDir := strings.TrimSpace(dir)
			if wsDir == "" {
				wsDir = "breyta-workspace"
			}
			absDir, err := filepath.Abs(wsDir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(absDir, 0o755); err != nil {
				return err
			}

			if err := os.MkdirAll(filepath.Join(absDir, "flows"), 0o755); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(absDir, "tmp", "flows"), 0o755); err != nil {
				return err
			}

			agentsPath := filepath.Join(absDir, "AGENTS.md")
			agentsContent := []byte(renderInitAgentsMD(target, !skillInstalled))
			wrote, err := writeInitFile(agentsPath, agentsContent, force)
			if err != nil {
				return err
			}
			if wrote {
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", agentsPath)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Kept existing %s (use --force to overwrite)\n", agentsPath)
			}

			gitignorePath := filepath.Join(absDir, ".gitignore")
			gitignoreContent := []byte(strings.TrimSpace(initGitignore) + "\n")
			wrote, err = writeInitFile(gitignorePath, gitignoreContent, force)
			if err != nil {
				return err
			}
			if wrote {
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", gitignorePath)
			}

			readmePath := filepath.Join(absDir, "README.md")
			readmeContent := []byte(renderInitReadmeMD())
			wrote, err = writeInitFile(readmePath, readmeContent, force)
			if err != nil {
				return err
			}
			if wrote {
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", readmePath)
			}

			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Next:")
			fmt.Fprintf(cmd.OutOrStdout(), "- Open this folder in your agent tool (or `cd %s` and start the agent) so it can read %s\n", absDir, agentsPath)
			fmt.Fprintln(cmd.OutOrStdout(), "- Authenticate: breyta auth login")
			fmt.Fprintln(cmd.OutOrStdout(), "- Verify: breyta workspaces list && breyta flows list")
			fmt.Fprintf(cmd.OutOrStdout(), "- Agent docs: breyta docs (or see %s)\n", agentsPath)
			return nil
		},
	}

	_ = app
	cmd.Flags().StringVar(&provider, "provider", string(skills.ProviderCodex), "Install location for the agent skill bundle (codex|cursor|claude|gemini)")
	cmd.Flags().StringVar(&dir, "dir", "breyta-workspace", "Workspace directory to create (contains AGENTS.md, flows/, tmp/flows/)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files in the workspace directory")
	cmd.Flags().BoolVar(&noSkill, "no-skill", false, "Skip installing the agent skill bundle")
	cmd.Flags().BoolVar(&noWorkspace, "no-workspace", false, "Skip creating the workspace directory and files")
	return cmd
}

func writeInitFile(path string, content []byte, force bool) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, errors.New("missing path")
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, content, 0o644)
}

const initGitignore = `
.DS_Store

# Scratch pulls / generated artifacts
tmp/
`

func renderInitAgentsMD(target skills.InstallTarget, skillNotInstalled bool) string {
	skillLine := "- Breyta skill bundle: " + target.File + "\n"
	if skillNotInstalled {
		skillLine = "- (Not installed) Breyta skill bundle would be at: " + target.File + "\n"
	}

	return strings.TrimSpace(`# Breyta agent workspace

This folder is meant to be used with a coding agent (Codex, Cursor, Claude Code, Gemini CLI, etc.) to build and operate Breyta workflows ("flows") through the ` + "`breyta`" + ` CLI.

## First-time setup
- Verify the CLI is installed: ` + "`breyta version`" + `
- Authenticate (hosted Breyta): ` + "`breyta auth login`" + `
- Choose a workspace: ` + "`breyta workspaces list`" + ` then ` + "`breyta workspaces use <workspace-id>`" + `
- Verify you can talk to the API: ` + "`breyta flows list`" + `

## Where to keep flow files
- ` + "`./flows/`" + `: recommended place for flow source files you want to keep (optionally in git)
- ` + "`./tmp/flows/`" + `: scratch pulls/edits

## Always-available agent context (recommended)
Many agent tools only read instructions from the active folder. This file (` + "`AGENTS.md`" + `) is the reliable source of truth.

If the agent needs more detail about Breyta, use:
` + "`breyta docs`" + ` (then ` + "`breyta docs find <query>`" + ` / ` + "`breyta docs show <slug>`" + `).

## Optional: installed skill bundle (nice-to-have)
Some agent tools can ingest a global skill bundle automatically, but not all do.

` + skillLine + `
- (Re)install / update it with: ` + "`breyta skills install --provider " + string(target.Provider) + "`" + `

If you want the agent to *always* use the skill when Breyta is involved, explicitly mention it in your project/root instructions (this file, or a root ` + "`AGENTS.md`" + ` equivalent).

Suggested line to paste into your agent's persistent project instructions:
- "When working with Breyta, read and follow: ` + "`" + target.File + "`" + ` (Breyta skill bundle), and use the ` + "`breyta`" + ` CLI."

## Release hygiene (required)
- Iterate in ` + "`draft`" + ` while editing and debugging.
- Do not repeatedly release to ` + "`live`" + ` during normal iteration.
- Release to ` + "`live`" + ` once after draft behavior is verified and you have explicit sign-off.

## Authoring standard (required before editing)
- Write the problem contract: trigger, inputs, outputs, side effects, failure behavior.
- Write the trigger map and path map: success path, fallback path, stop path.
- Define side effects and duplicate protection before building:
  - what must happen exactly once
  - idempotency key or dedupe strategy
- Define retry/timeout policy for each external boundary before draft runs.
- Choose concurrency mode intentionally before draft runs:
  - ` + "`sequential`" + ` for ordered work, shared state, large artifacts, or fragile APIs
  - ` + "`fanout`" + ` only for independent bounded items
  - ` + "`keyed`" + ` when work must serialize per entity
- For concurrent paths, write down what must never overlap and what timeout/partial-failure behavior is acceptable.
- Decide how large resources move through the flow:
  - inline small values
  - persist large artifacts
  - pass signed URLs/blob refs for large files
- Decide what run output proves success:
  - result fields
  - counts
  - child workflow ids
  - resource refs

## Reliability checklist (required)
- Exactly-once side effects have explicit duplicate protection.
- Retries are only used for transient failures and are bounded.
- Cursors/checkpoints do not advance past failed work.
- Concurrency is intentional and bounded.
- The chosen concurrency mode is justified in plain language.
- Shared state and side effects that must not overlap are named explicitly.
- Large payloads are passed by reference, not copied through many steps.
- Step ids/titles are operator-readable and make side effects obvious.
- Final run result contains proof of success, not just a ` + "`completed`" + ` status.

## Scale-aware defaults
- Prefer sequential handling for large artifact transfer unless fanout safety is proven.
- Prefer sequential mode when uncertain; concurrency is opt-in, not the default.
- Prefer child flows for heavyweight artifact creation or handoff.
- Use blob persistence + refs for large files instead of re-shaping raw bytes across many steps.

## Authoring loop (agent-friendly, draft-first)
1) Pull: ` + "`breyta flows pull <slug> --out ./flows/<slug>.clj`" + `
2) Edit ` + "`./flows/<slug>.clj`" + `
3) Push working copy to draft target: ` + "`breyta flows push --file ./flows/<slug>.clj`" + `
4) Check required draft config: ` + "`breyta flows configure check <slug>`" + `
5) Run draft target and wait for output: ` + "`breyta flows run <slug> --input '{\"n\":41}' --wait`" + `
6) Optional read-only draft check: ` + "`breyta flows validate <slug>`" + ` (useful for CI/troubleshooting)
7) Run at least one failure/no-op/replay check when feasible before release
8) If using concurrency, verify no skipped, duplicated, or overlapped work in draft output
9) Repeat steps 2-8 until behavior is correct and side effects are understood in draft
10) Release once (after explicit sign-off): ` + "`breyta flows release <slug>`" + `
11) Verify live install target: ` + "`breyta flows show <slug> --target live`" + `
12) Smoke-run live target and capture proof: ` + "`breyta flows run <slug> --target live --wait`" + `

## Provenance for derived flows
- Keep ` + "`created-by`" + ` as the creator of the current flow record.
- When a flow is derived from existing flows, store source lineage separately as provenance metadata.
- Only flows actually opened with ` + "`breyta flows show`" + ` or ` + "`breyta flows pull`" + ` become consulted provenance candidates. Search hits alone do not.
- After creating or updating a derived flow, persist curated provenance with:
  - ` + "`breyta flows provenance set <slug> --from-consulted`" + `
  - ` + "`breyta flows provenance set <slug> --source <workspace-id>/<flow-slug>`" + `
- Clear provenance intentionally with ` + "`breyta flows provenance set <slug> --clear`" + `.

## Docs for agents
- Product docs: ` + "`breyta docs`" + ` (search with ` + "`breyta docs find \"flows push\"`" + `)
- Command truth / flags: ` + "`breyta help <command...>`" + ` (for example: ` + "`breyta help flows push`" + `)
- Installed skill bundle: ` + "`breyta skills install --provider <codex|cursor|claude|gemini>`" + `
`)
}

func renderInitReadmeMD() string {
	return strings.TrimSpace(`# Breyta workspace

This directory was created by ` + "`breyta init`" + ` for coding-agent-driven Breyta work.

Quick start:

` + "```bash\n" + `breyta auth login
breyta workspaces list
breyta workspaces use <workspace-id>
breyta flows list
` + "```\n" + `

Suggested workflow:
- Keep editable flow source files in ` + "`./flows/`" + `
- Iterate in draft: pull, edit, push, configure check, run/validate
- If the flow was derived from other flows, persist curated lineage with ` + "`breyta flows provenance set <slug> --from-consulted`" + ` or ` + "`--source`" + `
- Release once to live after draft is verified and approved

Docs:
- Product docs: ` + "`breyta docs`" + `
- Command help: ` + "`breyta help <command...>`" + `
`)
}
