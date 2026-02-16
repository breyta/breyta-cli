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

func newUpgradeCmd(app *App) *cobra.Command {
	var apply bool
	var open bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for new releases and upgrade",
		RunE: func(cmd *cobra.Command, args []string) error {
			cur := buildinfo.DisplayVersion()
			ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Second)
			defer cancel()

			notice, checkErr := updatecheck.CheckNow(ctx, cur, 0)
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

			meta := map[string]any{"checked": true}
			if checkErr != nil {
				meta["checkError"] = checkErr.Error()
			}

			data := map[string]any{
				"update": notice,
			}

			if apply {
				if !notice.Available {
					return writeErr(cmd, errors.New("no newer version available"))
				}
				if len(notice.Upgrade) == 0 {
					return writeErr(cmd, errors.New("no automatic upgrade command available; use --open to view release artifacts"))
				}
				if err := runUpgradeCommand(ctx, notice.Upgrade, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
					return writeErr(cmd, err)
				}
				data["applied"] = true
				data["appliedCommand"] = notice.Upgrade
			}

			if open {
				if err := openReleasePage(notice.ReleaseURL); err != nil {
					return writeErr(cmd, err)
				}
				data["opened"] = true
				data["openedUrl"] = notice.ReleaseURL
			}

			return writeData(cmd, app, meta, data)
		},
	}

	cmd.Flags().BoolVar(&apply, "apply", false, "Run the detected upgrade command (currently Homebrew installs)")
	cmd.Flags().BoolVar(&open, "open", false, "Open the latest GitHub release page in your browser")
	return cmd
}
