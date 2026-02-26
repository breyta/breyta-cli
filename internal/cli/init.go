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
					} else if _, err := skills.InstallBreytaSkillFiles(home, p, files); err != nil {
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
	if skillNotInstalled {
		skillLine = "- (Not installed) Breyta skill bundle would be at: " + target.File + "\n"
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

## Authoring loop (agent-friendly)
1) Pull: ` + "`breyta flows pull <slug> --out ./flows/<slug>.clj`" + `
2) Edit ` + "`./flows/<slug>.clj`" + `
3) Push working copy: ` + "`breyta flows push --file ./flows/<slug>.clj`" + `
4) Check required config: ` + "`breyta flows configure check <slug>`" + `
5) Optional read-only check: ` + "`breyta flows validate <slug>`" + ` (useful for CI/troubleshooting)
6) Release: ` + "`breyta flows release <slug>`" + `
7) Verify live install target: ` + "`breyta flows show <slug> --target live`" + `
8) Smoke-run live target: ` + "`breyta flows run <slug> --target live --wait`" + `
9) Optional draft run and wait for output: ` + "`breyta flows run <slug> --input '{\"n\":41}' --wait`" + `

## Docs for agents
- Product docs: ` + "`breyta docs`" + ` (search with ` + "`breyta docs find \"flows push\"`" + `)
- Command truth / flags: ` + "`breyta help <command...>`" + ` (for example: ` + "`breyta help flows push`" + `)
- Installed skill bundle: ` + "`breyta skills install --provider <codex|cursor|claude|gemini>`" + `

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
- Pull, edit, push, validate, release, run

Docs:
- CLI docs: ` + "`breyta docs`" + `
`)
}
