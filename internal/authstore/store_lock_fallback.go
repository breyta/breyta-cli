//go:build aix || js || plan9 || wasip1

package authstore

import (
	"os"
	"sync"
)

// Breyta releases target macOS, Linux, and Windows. Keep source builds working
// on other Go platforms with process-local serialization where OS file-locking
// support is unavailable.
var fallbackStoreLock sync.Mutex

func lockStoreFile(*os.File) error {
	fallbackStoreLock.Lock()
	return nil
}

func unlockStoreFile(*os.File) error {
	fallbackStoreLock.Unlock()
	return nil
}
