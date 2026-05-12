//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package store

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockFileExclusive(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return ErrStoreLocked
		}
		return err
	}
	return nil
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
