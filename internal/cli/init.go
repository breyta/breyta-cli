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
			fmt.Fprintln(cmd.OutOrStdout(), "- Optional: open the TUI with: breyta")
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
	skillGuidance := "- Read and follow the installed skill bundle first.\n"
	if skillNotInstalled {
		skillLine = "- (Not installed) Breyta skill bundle would be at: " + target.File + "\n"
		skillGuidance = "- Install the Breyta skill bundle first when possible, or fall back to `breyta docs find \"CLI Workflow\"` and `breyta docs find \"CLI Essentials\"`.\n"
	}

	return strings.TrimSpace(`# Breyta agent workspace

This folder is meant to be used with a coding agent (Codex, Cursor, Claude Code, Gemini CLI, etc.) to build and operate Breyta workflows ("flows") through the ` + "`breyta`" + ` CLI.

## First-time setup
- Verify the CLI is installed: ` + "`breyta --version`" + `
- Authenticate (hosted Breyta): ` + "`breyta auth login`" + `
- Verify you can talk to the API: ` + "`breyta workspaces list`" + ` and ` + "`breyta flows list`" + `

## Where to keep flow files
- ` + "`./flows/`" + `: recommended place for flow source files you want to keep (optionally in git)
- ` + "`./tmp/flows/`" + `: scratch pulls/edits

## Canonical guidance
Many agent tools only read instructions from the active folder. This file (` + "`AGENTS.md`" + `) is the reliable local pointer.

` + skillLine + `
- (Re)install / update it with: ` + "`breyta skills install --provider " + string(target.Provider) + "`" + `

` + skillGuidance + `
- Open the workflow doctrine with: ` + "`breyta docs find \"CLI Workflow\"`" + `
- Use the condensed loop with: ` + "`breyta docs find \"CLI Essentials\"`" + `
- Use ` + "`breyta help <command...>`" + ` for flag truth.

Suggested line to paste into persistent project instructions:
- "When working with Breyta, read and follow: ` + "`" + target.File + "`" + ` and the CLI Workflow guide, then use the ` + "`breyta`" + ` CLI."

## Required authoring loop
1) Discover the right flow source:
   - existing/local flow work: ` + "`breyta flows list`" + ` / ` + "`breyta flows show <slug>`" + `
   - new or reusable-pattern work: ` + "`breyta flows search <query> --full`" + `
2) Pull and edit: ` + "`breyta flows pull <slug> --out ./flows/<slug>.clj`" + `
3) Structure-check the flow file: ` + "`breyta flows paren-check ./flows/<slug>.clj`" + `, then ` + "`breyta flows paren-repair ./flows/<slug>.clj`" + ` if needed
4) Declare ` + "`:requires`" + ` and add ` + "`:persist`" + ` for growing outputs
5) Push and check config: ` + "`breyta flows push --file ./flows/<slug>.clj`" + ` then ` + "`breyta flows configure check <slug>`" + `
6) If the check reports missing bindings or inputs, run ` + "`breyta flows configure <slug> --set ...`" + ` and re-run the check
7) Run draft and inspect proof: ` + "`breyta flows run <slug> --wait`" + `, ` + "`breyta runs show <workflow-id>`" + `, ` + "`breyta resources workflow list <workflow-id>`" + `
8) Release once after draft proof and explicit approval: ` + "`breyta flows release <slug>`" + `
9) Verify live explicitly: ` + "`breyta flows show <slug> --target live`" + ` and ` + "`breyta flows run <slug> --target live --wait`" + `

## Guardrails
- Fail closed on sensitive routing and hidden behavior inputs.
- Persist growing outputs early and inspect refs instead of trusting ` + "`completed`" + `.
- Put validation and duplicate protection in front of side effects.
- Final outputs should summarize status and proof fields, not raw provider payloads.

## Local development (optional)
For local ` + "`flows-api`" + ` development you typically use dev mode + env vars:

` + "```bash\n" + `export BREYTA_API_URL="http://localhost:8090"
export BREYTA_WORKSPACE="ws-acme"
export BREYTA_TOKEN="dev-user-123"
breyta --dev flows list
` + "```\n" + `
`)
}

func renderInitReadmeMD() string {
	return strings.TrimSpace(`# Breyta workspace

This directory was created by ` + "`breyta init`" + `.

Quick start:

` + "```bash\n" + `breyta auth login
breyta flows list
` + "```\n" + `

Suggested workflow:
- Keep editable flow source files in ` + "`./flows/`" + `
- Follow the installed Breyta skill bundle and the CLI Workflow guide for the full authoring doctrine
- Use the condensed loop from ` + "`AGENTS.md`" + ` for local iteration

Docs:
- CLI docs: ` + "`breyta docs`" + `
- Workflow guide: ` + "`breyta docs find \"CLI Workflow\"`" + `
`)
}
