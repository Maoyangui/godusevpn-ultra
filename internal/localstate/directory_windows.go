//go:build windows

// Package localstate bounds optional user recovery files to verified local
// directories. It does not authorize policy changes or migrate existing data.
package localstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

type directoryBackend interface {
	localDrive(string) error
	openDirectory(string, bool) (func(), error)
	createDirectory(string) error
}

// AcquireDirectory checks the namespace before any path I/O, walks from the
// volume root without following reparse points, and pins each existing parent.
// Read leases deny data-write handles; write leases must permit directory writes
// for atomic replacement and therefore do not prevent in-place reparse mutation
// by a concurrent same-account actor. Neither lease shares directory deletion.
// Read-only callers never create directories. A missing component returns
// os.ErrNotExist without probing descendants. Call release after all file I/O.
func AcquireDirectory(dir string, create bool) (func(), error) {
	return acquireDirectory(dir, create, nativeDirectory{})
}

func pathComponents(dir string) ([]string, error) {
	if len(dir) < 4 || len(dir) > 30000 || !filepath.IsAbs(dir) || len(filepath.VolumeName(dir)) != 2 || dir[1] != ':' || dir[2] != '\\' || filepath.Clean(dir) != dir {
		return nil, errors.New("恢复存储仅支持明确的本地磁盘绝对路径")
	}
	paths := []string{dir[:3]}
	current := dir[:3]
	for _, part := range strings.Split(dir[3:], `\`) {
		if !safeName(part) {
			return nil, errors.New("恢复存储不支持设备、数据流或歧义路径")
		}
		current = filepath.Join(current, part)
		paths = append(paths, current)
	}
	return paths, nil
}

func safeName(part string) bool {
	if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, `\/:*?"<>|`) {
		return false
	}
	for _, r := range part {
		if r < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
		return false
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		tail := base[3:]
		if (len(tail) == 1 && tail[0] >= '1' && tail[0] <= '9') || tail == "¹" || tail == "²" || tail == "³" {
			return false
		}
	}
	return true
}

func acquireDirectory(dir string, create bool, fs directoryBackend) (_ func(), err error) {
	components, err := pathComponents(dir)
	if err != nil {
		return nil, err
	}
	// This queries only the drive mapping/type, never a file on that drive.
	if err := fs.localDrive(components[0]); err != nil {
		return nil, err
	}
	var held []func()
	var once sync.Once
	release := func() {
		once.Do(func() {
			for i := len(held) - 1; i >= 0; i-- {
				held[i]()
			}
		})
	}
	defer func() {
		if err != nil {
			release()
		}
	}()
	for i, component := range components {
		var close func()
		close, err = fs.openDirectory(component, create)
		if errors.Is(err, os.ErrNotExist) && create && i > 0 {
			// All existing parents are pinned before creating this one child.
			if err = fs.createDirectory(component); err == nil {
				close, err = fs.openDirectory(component, create)
			}
		}
		if err != nil {
			return nil, err
		}
		held = append(held, close)
	}
	return release, nil
}

type nativeDirectory struct{}

func (nativeDirectory) localDrive(root string) error {
	device, err := windows.UTF16PtrFromString(root[:2])
	if err != nil {
		return errors.New("恢复存储盘符无效")
	}
	var target [32768]uint16
	if _, err := windows.QueryDosDevice(device, &target[0], uint32(len(target))); err != nil {
		return errors.New("恢复存储磁盘映射无法确认")
	}
	// Reject network providers and SUBST/other aliases before opening a root.
	// An unrecognized device namespace is not evidence of local storage.
	mapping := windows.UTF16ToString(target[:])
	if !localDeviceMapping(mapping) {
		return errors.New("恢复存储不支持网络盘或无法确认的磁盘别名")
	}
	p, err := windows.UTF16PtrFromString(root)
	if err != nil || windows.GetDriveType(p) != windows.DRIVE_FIXED {
		return errors.New("恢复存储仅支持可确认的本地固定磁盘")
	}
	return nil
}

func localDeviceMapping(mapping string) bool {
	const prefix = `\Device\HarddiskVolume`
	if !strings.HasPrefix(strings.ToLower(mapping), strings.ToLower(prefix)) || len(mapping) <= len(prefix) {
		return false
	}
	for _, r := range mapping[len(prefix):] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (nativeDirectory) openDirectory(path string, writable bool) (func(), error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, errors.New("恢复存储路径无效")
	}
	share := uint32(windows.FILE_SHARE_READ)
	if writable {
		// Ordinary atomic file replacement opens its parent for data-write.
		// Keep that operation working; this write lease does not claim to block
		// a malicious same-account actor changing reparse data in place.
		share |= windows.FILE_SHARE_WRITE
	}
	// FILE_READ_ATTRIBUTES alone does not participate in all Windows data-share
	// checks. LIST_DIRECTORY is required to make the write-denying lease real.
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES|windows.FILE_LIST_DIRECTORY, share, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fileError(err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(h)
		return nil, errors.New("恢复存储包含重解析点或无法确认的目录；未读取下级文件")
	}
	return func() { _ = windows.CloseHandle(h) }, nil
}

func (nativeDirectory) createDirectory(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("恢复存储路径无效")
	}
	err = windows.CreateDirectory(p, nil)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil // The caller reopens without following and validates it.
	}
	return fileError(err)
}

func fileError(err error) error {
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return os.ErrNotExist
	}
	return err
}

// OpenRegular opens a leaf without following reparse points. The caller must
// hold AcquireDirectory for its parent until this file is closed.
func OpenRegular(path string, createLock bool) (*os.File, error) {
	components, err := pathComponents(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if err := (nativeDirectory{}).localDrive(components[0]); err != nil {
		return nil, err
	}
	if !safeName(filepath.Base(path)) {
		return nil, errors.New("恢复文件名无效")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, errors.New("恢复文件路径无效")
	}
	access, disposition, share := uint32(windows.GENERIC_READ), uint32(windows.OPEN_EXISTING), uint32(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
	if createLock {
		access |= windows.GENERIC_WRITE
		disposition = windows.OPEN_ALWAYS
		share = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE
	}
	h, err := windows.CreateFile(p, access, share, nil, disposition, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fileError(err)
	}
	var info windows.ByHandleFileInformation
	typ, typeErr := windows.GetFileType(h)
	if err := windows.GetFileInformationByHandle(h, &info); err != nil || typeErr != nil || typ != windows.FILE_TYPE_DISK || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		windows.CloseHandle(h)
		return nil, errors.New("恢复文件不是可确认的普通本地文件")
	}
	return os.NewFile(uintptr(h), path), nil
}
