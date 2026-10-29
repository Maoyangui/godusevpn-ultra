//go:build windows

package browserprofiles

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

type nativeStorage struct{ path, userSID string }
type nativeDirectory struct {
	file    *os.File
	release func()
	userSID string
}
type nativeFile struct {
	*os.File
	parent *nativeDirectory
}

// Resolve the current ordinary user's known folder without consulting a
// caller-supplied path, environment-variable override or redirected network
// folder. Namespace validation precedes all file I/O in begin.
func platformStorage() (storage, error) {
	token := windows.GetCurrentProcessToken()
	user, e := token.GetTokenUser()
	if e != nil || user.User.Sid == nil {
		return nil, ErrUnsupported
	}
	sid := user.User.Sid.String()
	if sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20" {
		return nil, ErrUnsupported
	}
	var elevated, used uint32
	if windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &used) != nil || used != 4 || elevated != 0 {
		return nil, ErrUnsupported
	}
	var buffer [512]byte
	if windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buffer[0], uint32(len(buffer)), &used) != nil {
		return nil, ErrUnsupported
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	if label.Label.Sid == nil {
		return nil, ErrUnsupported
	}
	level := label.Label.Sid.String()
	if level != "S-1-16-8192" && level != "S-1-16-8448" {
		return nil, ErrUnsupported
	}
	base, e := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DONT_VERIFY)
	if e != nil {
		return nil, ErrUnsupported
	}
	return nativeStorage{filepath.Join(base, "GodUseVPN Ultra", "BrowserProfiles"), sid}, nil
}

func (s nativeStorage) begin(create bool) (_ directory, result error) {
	release, e := localstate.AcquireDirectory(s.path, create)
	if e != nil {
		return nil, nativeError(e)
	}
	defer func() {
		if result != nil {
			release()
		}
	}()
	pointer, e := windows.UTF16PtrFromString(s.path)
	if e != nil {
		return nil, ErrStore
	}
	h, e := windows.CreateFile(pointer, windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.SYNCHRONIZE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, nativeError(e)
	}
	accepted := false
	defer func() {
		if !accepted {
			windows.CloseHandle(h)
		}
	}()
	if verifyHandle(h, true, s.userSID) != nil || verifyDirectoryPath(h, s.path) != nil {
		return nil, ErrStore
	}
	accepted = true
	return &nativeDirectory{os.NewFile(uintptr(h), "browser-profile-directory"), release, s.userSID}, nil
}
func (d *nativeDirectory) close() { _ = d.file.Close(); d.release() }
func (d *nativeDirectory) names() ([]string, error) {
	// Go's Windows Readdirnames enumerates this already-pinned handle through
	// GetFileInformationByHandleEx. No child path is followed to obtain names.
	names, e := d.file.Readdirnames(maxRecords + 1)
	if e != nil && !errors.Is(e, io.EOF) {
		return nil, ErrStore
	}
	if len(names) > maxRecords {
		return nil, ErrFull
	}
	return names, nil
}

type openPolicy struct{ access, share, disposition, options, attributes uint32 }

func filePolicy(write, create bool) (openPolicy, error) {
	if create && !write {
		return openPolicy{}, ErrInput
	}
	p := openPolicy{access: windows.FILE_GENERIC_READ | windows.READ_CONTROL | windows.SYNCHRONIZE, share: windows.FILE_SHARE_READ, disposition: windows.FILE_OPEN, options: windows.FILE_NON_DIRECTORY_FILE | windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT, attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	if write {
		p.access |= windows.FILE_GENERIC_WRITE | windows.DELETE
	}
	if create {
		p.disposition = windows.FILE_CREATE
		p.options |= windows.FILE_WRITE_THROUGH
	}
	return p, nil
}
func (d *nativeDirectory) open(name string, write, create bool) (recordFile, error) {
	if !strings.HasSuffix(name, extension) || !idPattern.MatchString(strings.TrimSuffix(name, extension)) {
		return nil, ErrInput
	}
	p, e := filePolicy(write, create)
	if e != nil {
		return nil, e
	}
	relative, e := windows.NewNTUnicodeString(name)
	if e != nil {
		return nil, ErrInput
	}
	oa := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(d.file.Fd()), ObjectName: relative, Attributes: p.attributes}
	if create {
		oa.SecurityDescriptor, e = windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + d.userSID + ")")
		if e != nil {
			return nil, ErrStore
		}
	}
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	e = windows.NtCreateFile(&h, p.access, &oa, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, p.share, p.disposition, p.options, 0, 0)
	if e != nil {
		return nil, nativeError(e)
	}
	accepted := false
	defer func() {
		if !accepted {
			if create && status.Information == 2 {
				// The kernel confirmed this handle is our fresh FILE_CREATE,
				// so cleanup needs no pathname and cannot target an older file.
				flag := byte(1)
				_ = windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, &flag, 1)
			}
			windows.CloseHandle(h)
		}
	}()
	// FILE_CREATE is required to return FILE_CREATED, never a reused object.
	if create && status.Information != 2 {
		return nil, ErrStore
	}
	if verifyHandle(h, false, d.userSID) != nil {
		return nil, ErrStore
	}
	accepted = true
	return &nativeFile{os.NewFile(uintptr(h), "browser-profile-record"), d}, nil
}
func (d *nativeDirectory) remove(file recordFile) error {
	f, ok := file.(*nativeFile)
	if !ok || f.parent != d {
		return ErrStore
	}
	// Disposition is set on the same verified, exclusively write-held object.
	// A renamed/replaced path is never reopened and no directory is traversed.
	flag := byte(1)
	if e := windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileDispositionInfo, &flag, 1); e != nil {
		return nativeError(e)
	}
	return nil
}
func verifyHandle(h windows.Handle, directory bool, user string) error {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		return ErrStore
	}
	kind, e := windows.GetFileType(h)
	if e != nil || kind != windows.FILE_TYPE_DISK {
		return ErrStore
	}
	sd, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		return ErrStore
	}
	return privateDescriptor(sd, user)
}
func privateDescriptor(sd *windows.SECURITY_DESCRIPTOR, user string) error {
	trusted := func(s string) bool { return s == user || s == "S-1-5-18" || s == "S-1-5-32-544" }
	if sd == nil {
		return ErrStore
	}
	owner, _, e := sd.Owner()
	if e != nil || owner == nil || !trusted(owner.String()) {
		return ErrStore
	}
	acl, _, e := sd.DACL()
	if e != nil || acl == nil {
		return ErrStore
	}
	// All generic/data/attribute/EA/delete/ACL/ownership writes are privileged.
	const writes = uint32(0x500d0156)
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil {
			return ErrStore
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrStore
		}
		if uint32(ace.Mask)&writes != 0 && !trusted((*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()) {
			return ErrStore
		}
	}
	return nil
}
func verifyDirectoryPath(h windows.Handle, expected string) error {
	var actual, long [32768]uint16
	n, e := windows.GetFinalPathNameByHandle(h, &actual[0], uint32(len(actual)), 0)
	if e != nil || n == 0 || n >= uint32(len(actual)) {
		return ErrStore
	}
	p, e := windows.UTF16PtrFromString(expected)
	if e != nil {
		return ErrStore
	}
	length, e := windows.GetLongPathName(p, &long[0], uint32(len(long)))
	if e != nil || length == 0 || length >= uint32(len(long)) {
		return ErrStore
	}
	if !strings.EqualFold(strings.TrimPrefix(windows.UTF16ToString(actual[:n]), `\\?\`), windows.UTF16ToString(long[:length])) {
		return ErrStore
	}
	return nil
}
func nativeError(e error) error {
	switch {
	case errors.Is(e, os.ErrNotExist), errors.Is(e, windows.ERROR_FILE_NOT_FOUND), errors.Is(e, windows.ERROR_PATH_NOT_FOUND), errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND), errors.Is(e, windows.STATUS_OBJECT_PATH_NOT_FOUND):
		return os.ErrNotExist
	case errors.Is(e, windows.ERROR_ALREADY_EXISTS), errors.Is(e, windows.STATUS_OBJECT_NAME_COLLISION):
		return ErrConflict
	case errors.Is(e, windows.ERROR_SHARING_VIOLATION), errors.Is(e, windows.ERROR_LOCK_VIOLATION), errors.Is(e, windows.STATUS_SHARING_VIOLATION):
		return ErrBusy
	}
	return ErrStore
}
