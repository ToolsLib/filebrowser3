package fbhttp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/afero"
)

// The OS releases this shared-filesystem lock on process death. Redis leases
// are not used for mutual exclusion. Storage must support advisory file locks.
func lockUploadStage(ctx context.Context, fs afero.Fs, stage string) (func(), error) {
	file, err := fs.OpenFile(stage+".lock", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	var underlying afero.File = file
	for {
		base, ok := underlying.(*afero.BasePathFile)
		if !ok {
			break
		}
		underlying = base.File
	}
	fd, ok := underlying.(interface{ Fd() uintptr })
	if !ok {
		_ = file.Close()
		return nil, fmt.Errorf("upload filesystem does not support advisory locks")
	}
	for {
		locked, err := tryUploadLock(fd.Fd())
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if locked {
			return func() { _ = unlockUploadLock(fd.Fd()); _ = file.Close() }, nil
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
