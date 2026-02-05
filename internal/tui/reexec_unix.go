//go:build !windows

package tui

import "syscall"

func reexec(exe string, argv []string, env []string) error {
	return syscall.Exec(exe, argv, env)
}
