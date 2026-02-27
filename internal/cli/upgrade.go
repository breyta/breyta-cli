package cli

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/browseropen"
	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/breyta/breyta-cli/internal/skillsync"
	"github.com/breyta/breyta-cli/internal/updatecheck"
	"github.com/spf13/cobra"
)

var openReleasePage = func(u string) error {
	return browseropen.Open(u)
}

var runUpgradeCommand = func(ctx context.Context, argv []string, out io.Writer, errOut io.Writer) error {
	if len(argv) == 0 {
		return errors.New("missing upgrade command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout = out
	cmd.Stderr = errOut
	return cmd.Run()
}

var syncInstalledSkills = func(ctx context.Context, apiURL, token string) (skillsync.SyncResult, error) {
	return skillsync.SyncInstalledNow(ctx, apiURL, token)
}

func newUpgradeCmd(app *App) *cobra.Command {
	var apply bool
	var all bool
	var cliOnly bool
	var skillsOnly bool
	var yes bool
	var open bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for updates and optionally upgrade CLI + installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			cur := buildinfo.DisplayVersion()
			checkCtx, cancel := context.WithTimeout(cmd.Context(), 6*time.Second)
			defer cancel()

			notice, checkErr := updatecheck.CheckNow(checkCtx, cur, 0)
			if notice == nil {
				notice = updatecheck.CachedNotice(cur)
			}

			if notice == nil {
				notice = &updatecheck.Notice{
					Available:      false,
					CurrentVersion: strings.TrimSpace(cur),
					InstallMethod:  updatecheck.DetectInstallMethod(),
				}
			}
			if strings.TrimSpace(notice.ReleaseURL) == "" {
				notice.ReleaseURL = updatecheck.ReleasePageURL
			}
			if strings.TrimSpace(notice.FixCommand) == "" {
				notice.FixCommand = updatecheck.DefaultFixCommand
			}

			meta := map[string]any{"checked": true}
			if checkErr != nil {
				meta["checkError"] = checkErr.Error()
			}

			data := map[string]any{
				"update": notice,
			}

			if apply && (all || cliOnly || skillsOnly) {
				return writeErr(cmd, errors.New("--apply cannot be combined with --all, --cli-only, or --skills-only"))
			}
			if all && (cliOnly || skillsOnly) {
				return writeErr(cmd, errors.New("--all cannot be combined with --cli-only or --skills-only"))
			}
			if cliOnly && skillsOnly {
				return writeErr(cmd, errors.New("--cli-only and --skills-only are mutually exclusive"))
			}
			if (all || cliOnly || skillsOnly) && !yes {
				return writeErr(cmd, errors.New("add --yes to execute actions (example: breyta upgrade --all --yes)"))
			}

			runCLI := apply || all || cliOnly
			runSkills := all || skillsOnly

			if runSkills {
				skillsCtx, skillsCancel := context.WithTimeout(cmd.Context(), 30*time.Second)
				res, err := syncInstalledSkills(skillsCtx, app.APIURL, app.Token)
				skillsCancel()
				if err != nil {
					return writeErr(cmd, err)
				}
				data["skills"] = map[string]any{
					"requested":          true,
					"installedProviders": res.InstalledProviders,
					"syncedProviders":    res.SyncedProviders,
				}
			}

			if runCLI {
				if !notice.Available {
					if apply {
						return writeErr(cmd, errors.New("no newer version available"))
					}
					data["cli"] = map[string]any{
						"requested": true,
						"applied":   false,
						"reason":    "already_up_to_date",
					}
				} else {
					if len(notice.Upgrade) == 0 {
						return writeErr(cmd, errors.New("no automatic upgrade command available; use --open to view release artifacts"))
					}
					if err := runUpgradeCommand(cmd.Context(), notice.Upgrade, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
						return writeErr(cmd, err)
					}
					data["cli"] = map[string]any{
						"requested": true,
						"applied":   true,
						"command":   notice.Upgrade,
					}
				}
			}

			if open {
				if err := openReleasePage(notice.ReleaseURL); err != nil {
					return writeErr(cmd, err)
				}
				data["opened"] = true
				data["openedUrl"] = notice.ReleaseURL
			}

			data["fixCommand"] = notice.FixCommand

			return writeData(cmd, app, meta, data)
		},
	}

	cmd.Flags().BoolVar(&apply, "apply", false, "Run the detected CLI upgrade command (legacy)")
	cmd.Flags().BoolVar(&all, "all", false, "Upgrade CLI and refresh installed skills")
	cmd.Flags().BoolVar(&cliOnly, "cli-only", false, "Upgrade CLI only")
	cmd.Flags().BoolVar(&skillsOnly, "skills-only", false, "Refresh installed skills only")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm and execute --all/--cli-only/--skills-only actions")
	cmd.Flags().BoolVar(&open, "open", false, "Open the latest GitHub release page in your browser")
	return cmd
}
