//go:build unix

package sqlite

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func jobLockBusy(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}

func lockJobFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func unlockJobFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
