package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/breyta/breyta-cli/internal/updatecheck"

	"github.com/spf13/cobra"
)

const skipTUIUpgradeEnv = "BREYTA_SKIP_TUI_UPGRADE"

const (
	testNoBrewRunEnv      = "BREYTA_UPDATE_TEST_NO_BREW_RUN"
	testSimulateReexecEnv = "BREYTA_UPDATE_TEST_SIMULATE_REEXEC"
)

func maybeUpgradeBeforeTUI(cmd *cobra.Command) error {
	if cmd == nil || !updateChecksEnabled() {
		return nil
	}
	if strings.TrimSpace(os.Getenv(skipTUIUpgradeEnv)) != "" {
		return nil
	}
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return nil
	}

	cur := buildinfo.DisplayVersion()
	if cur == "dev" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	notice, err := updatecheck.CheckNow(ctx, cur, 6*time.Hour)
	if err != nil || notice == nil || !notice.Available {
		return nil
	}
	if notice.InstallMethod != updatecheck.InstallMethodBrew || !updatecheck.BrewAvailable() {
		// We only block for Homebrew installs (as requested).
		fmt.Fprintf(cmd.ErrOrStderr(), "Update available: %s → %s (run `brew upgrade breyta` if installed via brew)\n", notice.CurrentVersion, notice.LatestVersion)
		return nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Update available: %s → %s\n", notice.CurrentVersion, notice.LatestVersion)
	fmt.Fprint(cmd.ErrOrStderr(), "Update now via Homebrew? [Y/n] ")
	choice := strings.TrimSpace(readLine())
	if choice != "" && strings.ToLower(choice) != "y" && strings.ToLower(choice) != "yes" {
		return nil
	}

	if strings.TrimSpace(os.Getenv(testNoBrewRunEnv)) != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "Test mode: skipping `brew upgrade breyta`")
		if strings.TrimSpace(os.Getenv(testSimulateReexecEnv)) != "" {
			exe, err := os.Executable()
			if err != nil {
				return nil
			}
			env := append([]string{}, os.Environ()...)
			env = append(env, skipTUIUpgradeEnv+"=1")
			return syscall.Exec(exe, os.Args, env)
		}
		return nil
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "Running: brew upgrade breyta")
	c := exec.Command("brew", "upgrade", "breyta")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "brew upgrade failed: %v\n", err)
		fmt.Fprint(cmd.ErrOrStderr(), "Continue with current version? [Y/n] ")
		choice := strings.TrimSpace(readLine())
		if choice == "" || strings.ToLower(choice) == "y" || strings.ToLower(choice) == "yes" {
			return nil
		}
		return err
	}

	// Re-exec to pick up the newly installed binary.
	// Prefer resolving `breyta` via PATH for Homebrew installs (the Cellar path may
	// have changed).
	exe := ""
	if notice.InstallMethod == updatecheck.InstallMethodBrew {
		if p, err := exec.LookPath("breyta"); err == nil {
			exe = p
		}
	}
	if strings.TrimSpace(exe) == "" {
		p, err := os.Executable()
		if err != nil {
			return nil
		}
		exe = p
	}
	env := append([]string{}, os.Environ()...)
	env = append(env, skipTUIUpgradeEnv+"=1")
	return syscall.Exec(exe, os.Args, env)
}

func readLine() string {
	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
