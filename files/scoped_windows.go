package files

import (
	"os"
	"time"
)

const scopedNonblock = 0

func scopedChmod(root *os.Root, name string, mode os.FileMode) error { return root.Chmod(name, mode) }
func scopedChtimes(root *os.Root, name string, atime, mtime time.Time) error {
	return root.Chtimes(name, atime, mtime)
}
