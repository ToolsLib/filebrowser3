//go:build !windows

package fbhttp

import (
	"errors"
	"golang.org/x/sys/unix"
)

func tryUploadLock(fd uintptr) (bool, error) {
	err := unix.Flock(int(fd), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

func unlockUploadLock(fd uintptr) error { return unix.Flock(int(fd), unix.LOCK_UN) }
