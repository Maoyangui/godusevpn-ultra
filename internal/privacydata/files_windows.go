//go:build windows

package privacydata

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func validateLocalVolume(path string) error {
	root := filepath.VolumeName(path) + `\`
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return errors.New("路径不合法")
	}
	typ := windows.GetDriveType(p)
	if typ == windows.DRIVE_REMOTE || typ == windows.DRIVE_UNKNOWN || typ == windows.DRIVE_NO_ROOT_DIR {
		return errors.New("只支持可确认的本地磁盘，不支持映射网络驱动器")
	}
	return nil
}
func rejectReparse(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("路径不合法")
	}
	a, err := windows.GetFileAttributes(p)
	if err != nil || a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("不支持不可确认的路径或 Windows 重解析点")
	}
	return nil
}

func verifyHandle(h windows.Handle, path string) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return errors.New("只支持普通文件，不支持重解析点或目录")
	}
	if typ, err := windows.GetFileType(h); err != nil || typ != windows.FILE_TYPE_DISK {
		return errors.New("不支持设备文件")
	}
	b := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &b[0], uint32(len(b)), 0)
	if err != nil || n >= uint32(len(b)) {
		return errors.New("无法验证实际文件路径")
	}
	actual := windows.UTF16ToString(b[:n])
	actual = strings.TrimPrefix(actual, `\\?\`)
	// Temp directories and user-selected files can use an ordinary 8.3 alias.
	// Expand only that spelling; GetLongPathName does not authorize redirection.
	expected := path
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("路径不合法")
	}
	n, err = windows.GetLongPathName(p, &b[0], uint32(len(b)))
	if err != nil || n >= uint32(len(b)) {
		return errors.New("无法核对文件路径名称")
	}
	expected = windows.UTF16ToString(b[:n])
	if !strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected)) {
		return errors.New("文件路径经过重定向或名称映射，拒绝处理")
	}
	return nil
}

func openSource(path string) (*os.File, error) {
	release, err := pinValidatedPath(path, false)
	if err != nil {
		return nil, err
	}
	defer release()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, errors.New("路径不合法")
	}
	// Deny concurrent writes/deletes while reading; OPEN_REPARSE_POINT rejects
	// leaf redirection and final-path verification detects redirected parents.
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		return nil, errors.New("源文件无法独占读取，可能正被其他应用修改")
	}
	if err = verifyHandle(h, path); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

func openDestination(path string) (*os.File, func() error, error) {
	return openDestinationWithSecurity(path, nil)
}

func openPrivateDestination(path string) (*os.File, func() error, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, errors.New("无法确认报告保存账户")
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + u.User.Sid.String() + ")")
	if err != nil {
		return nil, nil, errors.New("无法建立报告私有权限")
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	return openDestinationWithSecurity(path, sa)
}

func openDestinationWithSecurity(path string, sa *windows.SecurityAttributes) (*os.File, func() error, error) {
	release, err := pinValidatedPath(path, true)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, nil, errors.New("路径不合法")
	}
	// CREATE_NEW is Windows' O_EXCL equivalent. DELETE access is held solely so a
	// failed operation can delete its own opened file by handle, without a path race.
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE|windows.GENERIC_READ|windows.DELETE, windows.FILE_SHARE_READ, sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, nil, errors.New("无法新建输出文件；已存在的文件不会覆盖")
	}
	f := os.NewFile(uintptr(h), path)
	discard := func() error {
		flag := byte(1)
		return windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, &flag, 1)
	}
	if err = verifyHandle(h, path); err != nil {
		_ = discard()
		_ = f.Close()
		return nil, nil, err
	}
	return f, discard, nil
}
