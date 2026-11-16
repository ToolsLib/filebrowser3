package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/afero"
)

// ScopedFs resolves aliases for authorization compatibility and executes every
// OS operation through a traversal-resistant directory handle. Resolving a
// path is not relied on to protect the subsequent operation from symlink races.
type ScopedFs struct {
	base      *afero.BasePathFs
	native    bool
	sourceErr error
}

type scopedFile struct {
	*os.File
	root string
}

func (f *scopedFile) Name() string {
	relative, err := filepath.Rel(f.root, f.File.Name())
	if err != nil {
		return f.File.Name()
	}
	return string(filepath.Separator) + relative
}

var (
	_ afero.Fs      = (*ScopedFs)(nil)
	_ afero.Lstater = (*ScopedFs)(nil)
)

const maxSymlinkHops = 255

func NewScopedFs(source afero.Fs, path string) *ScopedFs {
	native := false
	var sourceErr error
	if scoped, ok := source.(*ScopedFs); ok {
		native, sourceErr = scoped.native, scoped.sourceErr
		source = scoped.base
	} else {
		switch source.(type) {
		case *afero.OsFs:
			native = true
		case *afero.MemMapFs:
		default:
			sourceErr = fmt.Errorf("unsupported scoped filesystem source %T", source)
		}
	}
	return &ScopedFs{base: afero.NewBasePathFs(source, path).(*afero.BasePathFs), native: native, sourceErr: sourceErr}
}

func NewFs(source afero.Fs, path string, followExternal bool) afero.Fs {
	if followExternal {
		return afero.NewBasePathFs(source, path)
	}
	return NewScopedFs(source, path)
}

func BasePath(fs afero.Fs) *afero.BasePathFs {
	switch f := fs.(type) {
	case *ScopedFs:
		return f.base
	case *afero.BasePathFs:
		return f
	}
	return nil
}

func (s *ScopedFs) BasePathFs() *afero.BasePathFs { return s.base }
func (s *ScopedFs) RealPath(name string) (string, error) {
	if err := s.virtualGuard(name); err != nil {
		return "", err
	}
	return s.base.RealPath(name)
}
func (s *ScopedFs) Name() string { return "ScopedFs" }

// resolveMissing follows dangling links too, and retains the missing suffix.
// This permits new files while refusing writes through out-of-scope links.
func resolveMissing(target string) (string, error) {
	suffix := ""
	resolved, err := filepath.EvalSymlinks(target)
	for hops := 0; errors.Is(err, fs.ErrNotExist); {
		info, statErr := os.Lstat(target)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			hops++
			if hops > maxSymlinkHops {
				return "", os.ErrPermission
			}
			dest, readErr := os.Readlink(target)
			if readErr != nil {
				return "", readErr
			}
			if !filepath.IsAbs(dest) {
				parent, parentErr := filepath.EvalSymlinks(filepath.Dir(target))
				if parentErr != nil {
					return "", parentErr
				}
				dest = filepath.Join(parent, dest)
			}
			target = filepath.Clean(dest)
		} else {
			if statErr != nil && !os.IsNotExist(statErr) {
				return "", statErr
			}
			parent := filepath.Dir(target)
			if parent == target {
				return "", err
			}
			suffix = filepath.Join(filepath.Base(target), suffix)
			target = parent
		}
		resolved, err = filepath.EvalSymlinks(target)
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, suffix), nil
}

func (s *ScopedFs) resolve(name string, followFinal bool) (string, string, error) {
	if err := s.virtualGuard(name); err != nil {
		return "", "", err
	}
	if _, err := s.base.RealPath(name); err != nil {
		return "", "", err
	}
	rawRoot, err := filepath.Abs(afero.FullBaseFsPath(s.base, "/"))
	if err != nil {
		return "", "", err
	}
	rawTarget, err := filepath.Abs(afero.FullBaseFsPath(s.base, name))
	if err != nil {
		return "", "", err
	}
	lexical, err := filepath.Rel(rawRoot, rawTarget)
	if err != nil || !filepath.IsLocal(lexical) {
		return "", "", os.ErrPermission
	}
	root, err := filepath.EvalSymlinks(rawRoot)
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(root, lexical)
	resolved, err := resolveMissing(target)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(rel) {
		return "", "", os.ErrPermission
	}
	if !followFinal && lexical != "." {
		parent, err := resolveMissing(filepath.Dir(target))
		if err != nil {
			return "", "", err
		}
		rel, err = filepath.Rel(root, filepath.Join(parent, filepath.Base(target)))
		if err != nil || !filepath.IsLocal(rel) {
			return "", "", os.ErrPermission
		}
	}
	return root, rel, nil
}

func (s *ScopedFs) virtualGuard(name string) error {
	if s.sourceErr != nil {
		return s.sourceErr
	}
	if _, err := s.base.RealPath(name); err != nil {
		return err
	}
	relative, err := filepath.Rel(afero.FullBaseFsPath(s.base, "/"), afero.FullBaseFsPath(s.base, name))
	if err != nil || !filepath.IsLocal(relative) {
		return os.ErrPermission
	}
	return nil
}

func scopedMode(mode os.FileMode) os.FileMode {
	return mode & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
}

func (s *ScopedFs) rooted(name string, followFinal bool) (*os.Root, string, error) {
	root, name, err := s.resolve(name, followFinal)
	if err != nil {
		return nil, "", err
	}
	handle, err := os.OpenRoot(root)
	return handle, name, err
}

// ResolvedPath is a virtual path suitable for applying scope-relative rules to
// the target of an alias. Missing file names are resolved against their parent.
func (s *ScopedFs) ResolvedPath(name string) (string, error) {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return "", err
		}
		return "/" + strings.TrimLeft(filepath.ToSlash(filepath.Clean(name)), "/"), nil
	}
	_, relative, err := s.resolve(name, true)
	if err != nil {
		return "", err
	}
	if relative == "." {
		return "/", nil
	}
	return "/" + filepath.ToSlash(relative), nil
}

func ResolveVirtualPath(fs afero.Fs, name string) (string, error) {
	if scoped, ok := fs.(*ScopedFs); ok {
		return scoped.ResolvedPath(name)
	}
	if BasePath(fs) != nil {
		return (&ScopedFs{base: BasePath(fs), native: true}).ResolvedPath(name)
	}
	return name, nil
}

func (s *ScopedFs) Create(name string) (afero.File, error) {
	return s.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}
func (s *ScopedFs) Open(name string) (afero.File, error) { return s.OpenFile(name, os.O_RDONLY, 0) }

func (s *ScopedFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return nil, err
		}
		return s.base.OpenFile(name, flag, scopedMode(perm))
	}
	root, relative, err := s.rooted(name, flag&os.O_EXCL == 0)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.OpenFile(relative, flag|scopedNonblock, scopedMode(perm))
	if err != nil {
		return nil, err
	}
	return &scopedFile{File: file, root: root.Name()}, nil
}

func (s *ScopedFs) Mkdir(name string, perm os.FileMode) error {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return err
		}
		return s.base.Mkdir(name, scopedMode(perm))
	}
	root, relative, err := s.rooted(name, false)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Mkdir(relative, scopedMode(perm))
}
func (s *ScopedFs) MkdirAll(name string, perm os.FileMode) error {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return err
		}
		return s.base.MkdirAll(name, scopedMode(perm))
	}
	root, relative, err := s.rooted(name, true)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.MkdirAll(relative, scopedMode(perm))
}
func (s *ScopedFs) Remove(name string) error {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return err
		}
		return s.base.Remove(name)
	}
	root, relative, err := s.rooted(name, false)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(relative)
}
func (s *ScopedFs) RemoveAll(name string) error {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return err
		}
		return s.base.RemoveAll(name)
	}
	root, relative, err := s.rooted(name, false)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(relative)
}
func (s *ScopedFs) Rename(oldname, newname string) error {
	if !s.native {
		if err := s.virtualGuard(oldname); err != nil {
			return err
		}
		if err := s.virtualGuard(newname); err != nil {
			return err
		}
		return s.base.Rename(oldname, newname)
	}
	root, old, err := s.rooted(oldname, false)
	if err != nil {
		return err
	}
	defer root.Close()
	newRoot, new, err := s.resolve(newname, false)
	if err != nil {
		return err
	}
	sameRoot := newRoot == root.Name()
	if runtime.GOOS == "windows" {
		sameRoot = strings.EqualFold(newRoot, root.Name())
	}
	if !sameRoot {
		return os.ErrPermission
	}
	return root.Rename(old, new)
}
func (s *ScopedFs) Stat(name string) (os.FileInfo, error) {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return nil, err
		}
		return s.base.Stat(name)
	}
	root, relative, err := s.rooted(name, true)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Stat(relative)
}
func (s *ScopedFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return nil, false, err
		}
		return s.base.LstatIfPossible(name)
	}
	root, relative, err := s.rooted(name, false)
	if err != nil {
		return nil, true, err
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	return info, true, err
}

func (s *ScopedFs) Chmod(name string, mode os.FileMode) error {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return err
		}
		return s.base.Chmod(name, scopedMode(mode))
	}
	root, relative, err := s.rooted(name, true)
	if err != nil {
		return err
	}
	defer root.Close()
	return scopedChmod(root, relative, scopedMode(mode))
}
func (s *ScopedFs) Chown(name string, uid, gid int) error {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return err
		}
		return s.base.Chown(name, uid, gid)
	}
	file, err := s.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.(*scopedFile).File.Chown(uid, gid)
}
func (s *ScopedFs) Chtimes(name string, atime, mtime time.Time) error {
	if !s.native {
		if err := s.virtualGuard(name); err != nil {
			return err
		}
		return s.base.Chtimes(name, atime, mtime)
	}
	root, relative, err := s.rooted(name, true)
	if err != nil {
		return err
	}
	defer root.Close()
	return scopedChtimes(root, relative, atime, mtime)
}
