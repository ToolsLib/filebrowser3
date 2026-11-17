package diskcache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/spf13/afero"
	"golang.org/x/sys/windows"
)

func cacheFilesystem(fs afero.Fs) afero.Fs {
	switch fs.(type) {
	case afero.OsFs, *afero.OsFs:
		return deleteSharingFs{}
	default:
		return fs
	}
}

// Cached previews stream from an open handle. Windows readers must share delete
// access so replacement and eviction can succeed while the old bytes are served.
// BasePathFs still resolves and confines every name before this Open is called.
type deleteSharingFs struct{ afero.OsFs }

func cachePath(name string) (*uint16, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	// Preserve support for long local and UNC cache paths.
	if !strings.HasPrefix(absolute, `\\?\`) {
		if strings.HasPrefix(absolute, `\\`) {
			absolute = `\\?\UNC\` + absolute[2:]
		} else {
			absolute = `\\?\` + absolute
		}
	}
	return windows.UTF16PtrFromString(absolute)
}

func (deleteSharingFs) Open(name string) (afero.File, error) {
	encoded, err := cachePath(name)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	handle, err := windows.CreateFile(encoded, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	return os.NewFile(uintptr(handle), name), nil
}

func (fs deleteSharingFs) Rename(oldname, newname string) error {
	err := fs.OsFs.Rename(oldname, newname)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return err
	}
	// MoveFileEx cannot replace an open destination even with delete sharing.
	// FileRenameInfoEx provides replacement while readers retain the old handle.
	// https://learn.microsoft.com/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information
	source, pathErr := cachePath(oldname)
	if pathErr != nil {
		return err
	}
	handle, openErr := windows.CreateFile(source, windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if openErr != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	absolute, pathErr := filepath.Abs(newname)
	if pathErr != nil {
		return err
	}
	name, pathErr := windows.UTF16FromString(absolute)
	if pathErr != nil {
		return err
	}
	type renameInformation struct {
		Flags          uint32
		RootDirectory  windows.Handle
		FileNameLength uint32
		FileName       [1]uint16
	}
	var layout renameInformation
	offset := unsafe.Offsetof(layout.FileName)
	length := len(name) - 1
	buffer := make([]byte, int(offset)+length*2)
	info := (*renameInformation)(unsafe.Pointer(&buffer[0]))
	info.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.FileNameLength = uint32(length * 2)
	copy(unsafe.Slice(&info.FileName[0], length), name[:length])
	if renameErr := windows.SetFileInformationByHandle(handle, windows.FileRenameInfoEx, &buffer[0], uint32(len(buffer))); renameErr != nil {
		return &os.LinkError{Op: "rename", Old: oldname, New: newname, Err: renameErr}
	}
	return nil
}
