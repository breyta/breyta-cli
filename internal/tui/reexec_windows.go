//go:build windows

package tui

import (
	"os"
	"os/exec"
)

func reexec(exe string, argv []string, env []string) error {
	args := []string{}
	if len(argv) > 1 {
		args = argv[1:]
	}
	c := exec.Command(exe, args...)
	c.Env = env
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Start()
}
