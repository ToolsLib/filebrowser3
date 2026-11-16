//go:build !windows

package files

import (
	"golang.org/x/sys/unix"
	"os"
	"time"
)

const scopedNonblock = unix.O_NONBLOCK

func scopedChmod(root *os.Root, name string, mode os.FileMode) error {
	file, err := root.OpenFile(name, os.O_RDONLY|scopedNonblock, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Chmod(mode)
}
func scopedChtimes(root *os.Root, name string, atime, mtime time.Time) error {
	file, err := root.OpenFile(name, os.O_RDONLY|scopedNonblock, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return unix.Futimes(int(file.Fd()), []unix.Timeval{unix.NsecToTimeval(atime.UnixNano()), unix.NsecToTimeval(mtime.UnixNano())})
}
