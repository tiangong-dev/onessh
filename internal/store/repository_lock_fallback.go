//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris)

package store

import (
	"os"
	"sync"
)

var fallbackStoreLocks sync.Map

func lockFileExclusive(file *os.File) error {
	actual, _ := fallbackStoreLocks.LoadOrStore(file.Name(), &sync.Mutex{})
	lock := actual.(*sync.Mutex)
	if !lock.TryLock() {
		return ErrStoreLocked
	}
	return nil
}

func unlockFile(file *os.File) error {
	if actual, ok := fallbackStoreLocks.Load(file.Name()); ok {
		actual.(*sync.Mutex).Unlock()
	}
	return nil
}
