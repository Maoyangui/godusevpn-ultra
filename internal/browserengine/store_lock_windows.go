//go:build windows

package browserengine

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

// The caller holds verified fixed-cache parent pins. Lock files are permanent
// zero-byte regular leaves: removing them would split the lock namespace while
// another process still holds the old object. LockFileEx is cross-session and
// cross-process; a directory handle alone provides no such exclusion.
func lockStoreFile(directory, name string, exclusive bool) (func(), error) {
	if !safePath(name) || filepath.Base(name) != name {
		return nil, ErrStore
	}
	file, err := localstate.OpenRegular(filepath.Join(directory, name), true)
	if err != nil {
		return nil, lockError(err)
	}
	handle := windows.Handle(file.Fd())
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.NumberOfLinks != 1 || info.FileSizeHigh != 0 || info.FileSizeLow != 0 {
		file.Close()
		return nil, ErrStore
	}
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	overlapped := &windows.Overlapped{}
	if err := windows.LockFileEx(handle, flags, 0, 1, 0, overlapped); err != nil {
		file.Close()
		return nil, lockError(err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = windows.UnlockFileEx(handle, 0, 1, 0, overlapped)
			_ = file.Close()
		})
	}, nil
}

func lockError(err error) error {
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, os.ErrPermission) {
		return ErrBusy
	}
	return ErrStore
}
