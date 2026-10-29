//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const sealedReadExecute = 0x1200a9
const sealedFullAccess = 0x1f01ff

// The owner is Administrators, not the originating ordinary user: an owner can
// otherwise rewrite a read-only DACL. Every object gets a protected, explicit
// ACL; no writable inherited ACE is retained and no inheritance is propagated.
func codeDescriptor(sid string) (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.StringToSid(sid)
	if err != nil || user == nil || !user.IsValid() || user.String() != sid || sid == "S-1-5-18" || sid == "S-1-5-32-544" {
		return nil, ErrRefused
	}
	return windows.SecurityDescriptorFromString("O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;" + sid + ")")
}

func verifyCodeDescriptor(sd *windows.SECURITY_DESCRIPTOR, sid string) error {
	if sd == nil || !sd.IsValid() {
		return ErrRefused
	}
	owner, defaulted, err := sd.Owner()
	if err != nil || defaulted || owner == nil || !owner.IsValid() || owner.String() != "S-1-5-32-544" {
		return ErrRefused
	}
	control, revision, err := sd.Control()
	if err != nil || revision != 1 || control&windows.SE_DACL_PROTECTED == 0 || control&windows.SE_DACL_PRESENT == 0 {
		return ErrRefused
	}
	acl, defaulted, err := sd.DACL()
	if err != nil || defaulted || acl == nil || acl.AceCount != 3 {
		return ErrRefused
	}
	expected := map[string]uint32{"S-1-5-18": sealedFullAccess, "S-1-5-32-544": sealedFullAccess, sid: sealedReadExecute}
	if len(expected) != 3 {
		return ErrRefused
	}
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, index, &ace) != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || ace.Header.AceSize < 20 {
			return ErrRefused
		}
		identity := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !identity.IsValid() || uint32(ace.Header.AceSize) != 8+uint32(identity.Len()) {
			return ErrRefused
		}
		name := identity.String()
		mask, exists := expected[name]
		if !exists || uint32(ace.Mask) != mask {
			return ErrRefused
		}
		delete(expected, name)
	}
	if len(expected) != 0 {
		return ErrRefused
	}
	return nil
}

type sealedObject struct {
	file      *os.File
	name      string
	directory bool
}

func validateObjectHandle(handle windows.Handle, path string, directory bool) error {
	var info windows.ByHandleFileInformation
	typ, err := windows.GetFileType(handle)
	if err != nil || typ != windows.FILE_TYPE_DISK || windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		return ErrRefused
	}
	var final, long [32768]uint16
	n, err := windows.GetFinalPathNameByHandle(handle, &final[0], uint32(len(final)), 0)
	if err != nil || n == 0 || n >= uint32(len(final)) {
		return ErrRefused
	}
	count, err := windows.GetLongPathName(windows.StringToUTF16Ptr(path), &long[0], uint32(len(long)))
	if err != nil || count == 0 || count >= uint32(len(long)) || !strings.EqualFold(strings.TrimPrefix(windows.UTF16ToString(final[:n]), `\\?\`), windows.UTF16ToString(long[:count])) {
		return ErrRefused
	}
	return nil
}

func pinSealDirectory(lease *nativeLease, path string) error {
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(h), "browser-guard-seal-directory")
	lease.files = append(lease.files, file)
	if validateObjectHandle(h, path, true) != nil {
		return ErrRefused
	}
	lease.seal = append(lease.seal, sealedObject{file, path, true})
	return nil
}

func verifyExactChildren(path string, names ...string) error {
	children, err := os.ReadDir(path)
	if err != nil || len(children) != len(names) {
		return ErrRefused
	}
	want := map[string]bool{}
	for _, name := range names {
		want[name] = true
	}
	for _, child := range children {
		if !want[child.Name()] || !child.IsDir() || child.Type()&os.ModeSymlink != 0 {
			return ErrRefused
		}
		delete(want, child.Name())
	}
	if len(want) != 0 {
		return ErrRefused
	}
	return nil
}

func sealCode(lease *nativeLease, userSID, session, engineID, version string, expected map[string]bool) error {
	descriptor, err := codeDescriptor(userSID)
	if err != nil {
		return ErrRefused
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return ErrRefused
	}
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil {
		return ErrRefused
	}
	if verifyExactChildren(session, "profile", engineID) != nil || verifyExactChildren(filepath.Join(session, engineID), version) != nil {
		return ErrRefused
	}
	for _, object := range lease.seal {
		h := windows.Handle(object.file.Fd())
		before, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			return err
		}
		if before == nil {
			return ErrRefused
		}
		current, _, err := before.Owner()
		if err != nil || current == nil || !current.IsValid() || current.String() != userSID && current.String() != "S-1-5-32-544" {
			return ErrRefused
		}
		// Changes and readback use this already verified, non-share-delete handle;
		// no pathname SetNamedSecurityInfo or owner-following reopen is used.
		if err := windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil); err != nil {
			return err
		}
		after, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			return err
		}
		if verifyCodeDescriptor(after, userSID) != nil {
			return ErrRefused
		}
	}
	// Directory pins and hashes protect known objects; the administrator-owned
	// read-only ACL also prevents new user-created optional DLLs. Re-enumeration
	// closes the interval before the last object's ACL was applied. Profile is
	// deliberately outside this enumeration and remains a writable fresh leaf.
	root := filepath.Join(session, engineID, version)
	seen := map[string]bool{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ErrRefused
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return ErrRefused
		}
		key := filepath.ToSlash(relative)
		if !expected[key] || seen[key] || entry.Type()&os.ModeSymlink != 0 {
			return ErrRefused
		}
		seen[key] = true
		return nil
	}); err != nil || len(seen) != len(expected) {
		return ErrRefused
	}
	for _, object := range lease.seal {
		h := windows.Handle(object.file.Fd())
		if validateObjectHandle(h, object.name, object.directory) != nil {
			return ErrRefused
		}
		sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil || verifyCodeDescriptor(sd, userSID) != nil {
			return ErrRefused
		}
	}
	if verifyExactChildren(session, "profile", engineID) != nil || verifyExactChildren(filepath.Join(session, engineID), version) != nil {
		return ErrRefused
	}
	return nil
}
