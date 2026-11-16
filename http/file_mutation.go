package fbhttp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const internalWritePrefix = ".filebrowser-write-"

type mutationLock struct {
	gate  chan struct{}
	users int
}

var fileMutations = struct {
	sync.Mutex
	paths map[string]*mutationLock
}{paths: make(map[string]*mutationLock)}

// lockFileMutation serializes checks and writes for one destination. Entries
// disappear after the last waiter, so arbitrary request paths cannot leak locks.
func lockFileMutation(ctx context.Context, name string) (func(), error) {
	name = physicalPathKey(name)
	fileMutations.Lock()
	lock := fileMutations.paths[name]
	if lock == nil {
		lock = &mutationLock{gate: make(chan struct{}, 1)}
		fileMutations.paths[name] = lock
	}
	lock.users++
	fileMutations.Unlock()
	releaseRef := func() {
		fileMutations.Lock()
		lock.users--
		if lock.users == 0 {
			delete(fileMutations.paths, name)
		}
		fileMutations.Unlock()
	}
	select {
	case lock.gate <- struct{}{}:
		return func() { <-lock.gate; releaseRef() }, nil
	case <-ctx.Done():
		releaseRef()
		return nil, ctx.Err()
	}
}

func physicalPathKey(name string) string {
	name = filepath.Clean(name)
	target, suffix := name, ""
	for {
		resolved, err := filepath.EvalSymlinks(target)
		if err == nil {
			name = filepath.Join(resolved, suffix)
			break
		}
		if !os.IsNotExist(err) || filepath.Dir(target) == target {
			break
		}
		suffix = filepath.Join(filepath.Base(target), suffix)
		target = filepath.Dir(target)
	}
	if runtime.GOOS == "windows" {
		name = strings.ToLower(name)
	}
	return name
}

func internalWritePath(name string) bool {
	for _, component := range strings.Split(slashClean(name), "/") {
		if strings.HasPrefix(strings.ToLower(component), internalWritePrefix) {
			return true
		}
	}
	return false
}
