//go:build windows

package ipc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func pinForegroundImage(expected, actual, adminSID string) ([]*os.File, error) {
	return pinForegroundImageWithCheck(expected, actual, adminSID, checkCodeDescriptor)
}

// Separate the handle-pinning mechanism from ACL policy for local fixture tests.
// Production always supplies checkCodeDescriptor through pinForegroundImage.
func pinForegroundImageWithCheck(expected, actual, adminSID string, checkDescriptor func(*windows.SECURITY_DESCRIPTOR, string, bool) error) ([]*os.File, error) {
	if !sameLocalImage(expected, actual) || !strings.EqualFold(filepath.Base(actual), "godusevpn-svc.exe") {
		return nil, errors.New("foreground server is not the fixed local service sibling")
	}
	path := filepath.Clean(expected)
	volume := filepath.VolumeName(path)
	parts := strings.Split(strings.TrimPrefix(path, volume+`\`), `\`)
	current := volume + `\`
	var held []*os.File
	ok := false
	defer func() {
		if !ok {
			for _, f := range held {
				_ = f.Close()
			}
		}
	}()
	for i := -1; i < len(parts); i++ {
		if i >= 0 {
			current = filepath.Join(current, parts[i])
		}
		isDir := i < len(parts)-1
		name, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return nil, errors.New("invalid local code path")
		}
		access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)
		share := uint32(windows.FILE_SHARE_READ)
		flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
		if isDir {
			// Permit unrelated file I/O, but hold every ancestor against rename
			// or deletion for this connection. Never follow a reparse point.
			share |= windows.FILE_SHARE_WRITE
			// Attributes-only handles do not enforce data sharing exclusions.
			access |= windows.FILE_LIST_DIRECTORY
			flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		} else {
			access |= windows.FILE_READ_DATA
		}
		h, err := windows.CreateFile(name, access, share, nil, windows.OPEN_EXISTING, flags, 0)
		if err != nil {
			return nil, errors.New("cannot pin foreground code path")
		}
		f := os.NewFile(uintptr(h), "ipc-service-image")
		if f == nil {
			_ = windows.CloseHandle(h)
			return nil, errors.New("invalid code handle")
		}
		held = append(held, f)
		var fileInfo windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(h, &fileInfo); err != nil || fileInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (fileInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != isDir {
			return nil, errors.New("foreground code path changed or contains a reparse point")
		}
		sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			return nil, errors.New("foreground code ACL cannot be read")
		}
		if err = checkDescriptor(sd, adminSID, isDir); err != nil {
			return nil, fmt.Errorf("foreground code security check failed at component %d: %w", i, err)
		}
	}
	ok = true
	return held, nil
}

func checkCodeDescriptor(sd *windows.SECURITY_DESCRIPTOR, adminSID string, directory bool) error {
	if sd == nil {
		return errors.New("missing code security descriptor")
	}
	trusted := func(sid string) bool {
		// Windows volume roots may be owned by its servicing account. This is
		// the fixed TrustedInstaller service SID, not an arbitrary service SID.
		const trustedInstaller = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
		return sid == "S-1-5-18" || sid == "S-1-5-32-544" || sid == trustedInstaller || (adminSID != "" && sid == adminSID)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trusted(owner.String()) {
		return errors.New("untrusted code owner")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("missing code DACL")
	}
	const fileWrites uint32 = 0x500D0156
	dangerous := fileWrites
	if directory {
		// Creating a distinct sibling directory is not the ability to replace
		// an existing pinned ancestor. Other write/delete/ACL rights are denied.
		dangerous &^= windows.FILE_APPEND_DATA
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, i, &ace); err != nil || ace == nil {
			return errors.New("invalid code ACE")
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if uint32(ace.Mask)&dangerous != 0 {
				sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
				if !trusted(sid.String()) {
					return errors.New("untrusted code writer")
				}
			}
		default:
			return errors.New("unsupported code ACE")
		}
	}
	runtime.KeepAlive(sd)
	return nil
}
