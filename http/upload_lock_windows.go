package fbhttp

import (
	"errors"
	"golang.org/x/sys/windows"
)

func tryUploadLock(fd uintptr) (bool, error) {
	err := windows.LockFileEx(windows.Handle(fd), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

func unlockUploadLock(fd uintptr) error {
	return windows.UnlockFileEx(windows.Handle(fd), 0, 1, 0, &windows.Overlapped{})
}
