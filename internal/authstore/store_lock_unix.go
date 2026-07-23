//go:build darwin || linux

package authstore

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockStoreFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

func unlockStoreFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
