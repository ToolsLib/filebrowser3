//go:build !windows

package diskcache

import "github.com/spf13/afero"

func cacheFilesystem(fs afero.Fs) afero.Fs { return fs }
