package cli

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/breyta/breyta-cli/internal/updatecheck"
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
		// Drain only once.
		a.updateCh = nil
	default:
	}
}
