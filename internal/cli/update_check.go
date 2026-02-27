package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/breyta/breyta-cli/internal/updatecheck"
	"github.com/spf13/cobra"
)

func updateChecksEnabled() bool {
	v := strings.TrimSpace(os.Getenv("BREYTA_NO_UPDATE_CHECK"))
	return v == ""
}

func (a *App) startUpdateCheckNonBlocking(ctx context.Context, maxAge time.Duration) {
	if a == nil || !updateChecksEnabled() {
		return
	}
	if a.updateCh != nil {
		return
	}

	cur := buildinfo.DisplayVersion()
	a.updateNotice = updatecheck.CachedNotice(cur)
	if a.updateNotice != nil && a.updateNotice.Available && strings.TrimSpace(a.updateNotice.FixCommand) == "" {
		a.updateNotice.FixCommand = updatecheck.DefaultFixCommand
	}

	ch := make(chan *updatecheck.Notice, 1)
	a.updateCh = ch
	go func() {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		n, _ := updatecheck.CheckNow(cctx, cur, maxAge)
		ch <- n
	}()
}

func (a *App) consumeUpdateNoticeNonBlocking() {
	if a == nil || a.updateCh == nil {
		return
	}
	select {
	case n := <-a.updateCh:
		if n != nil && n.Available {
			a.updateNotice = n
		}
		if a.updateNotice != nil && a.updateNotice.Available && strings.TrimSpace(a.updateNotice.FixCommand) == "" {
			a.updateNotice.FixCommand = updatecheck.DefaultFixCommand
		}
		// Drain only once.
		a.updateCh = nil
	default:
	}
}

func (a *App) updateFixCommand() string {
	if a == nil || a.updateNotice == nil {
		return updatecheck.DefaultFixCommand
	}
	if cmd := strings.TrimSpace(a.updateNotice.FixCommand); cmd != "" {
		return cmd
	}
	return updatecheck.DefaultFixCommand
}

func (a *App) emitUpdateReminder(cmd *cobra.Command) {
	if a == nil || cmd == nil || a.updateReminderShown {
		return
	}
	a.consumeUpdateNoticeNonBlocking()
	if a.updateNotice == nil || !a.updateNotice.Available {
		return
	}
	current := strings.TrimSpace(a.updateNotice.CurrentVersion)
	latest := strings.TrimSpace(a.updateNotice.LatestVersion)
	if current == "" {
		current = buildinfo.DisplayVersion()
	}
	if latest == "" {
		return
	}
	_, _ = fmt.Fprintf(
		cmd.ErrOrStderr(),
		"Update available: %s -> %s. Run `%s`.\n",
		current,
		latest,
		a.updateFixCommand(),
	)
	a.updateReminderShown = true
}
