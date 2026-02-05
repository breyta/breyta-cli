package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/breyta/breyta-cli/internal/updatecheck"
)

const skipTUIUpdatePromptEnv = "BREYTA_SKIP_TUI_UPDATE_PROMPT"

const (
	testNoBrewRunEnv      = "BREYTA_UPDATE_TEST_NO_BREW_RUN"
	testSimulateReexecEnv = "BREYTA_UPDATE_TEST_SIMULATE_REEXEC"
)

func runUpgradeAndReexec(n *updatecheck.Notice) error {
	if !updateChecksEnabled() {
		return nil
	}
	if n == nil || !n.Available {
		return nil
	}
	if n.InstallMethod != updatecheck.InstallMethodBrew || !updatecheck.BrewAvailable() {
		return nil
	}

	if strings.TrimSpace(os.Getenv(testNoBrewRunEnv)) == "" {
		fmt.Fprintln(os.Stderr, "Running: brew upgrade breyta")
		c := exec.Command("brew", "upgrade", "breyta")
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		if err := c.Run(); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(os.Stderr, "Test mode: skipping `brew upgrade breyta`")
	}

	if strings.TrimSpace(os.Getenv(testNoBrewRunEnv)) != "" && strings.TrimSpace(os.Getenv(testSimulateReexecEnv)) == "" {
		return nil
	}

	exe := ""
	if p, err := exec.LookPath("breyta"); err == nil {
		exe = p
	}
	if strings.TrimSpace(exe) == "" {
		p, err := os.Executable()
		if err != nil {
			return nil
		}
		exe = p
	}

	env := append([]string{}, os.Environ()...)
	env = append(env, skipTUIUpdatePromptEnv+"=1")
	return reexec(exe, os.Args, env)
}
