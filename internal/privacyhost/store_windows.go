//go:build windows && (amd64 || arm64)

package privacyhost

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"github.com/Maoyangui/godusevpn/internal/privacy"
	"golang.org/x/sys/windows"
)

const privateSDDL = "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// Environment variables and RPC parameters cannot choose this directory.
func machineDirectory(lease *maintenance.Lease) (string, error) {
	if err := lease.Check(); err != nil {
		return "", err
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("ProgramData is not an absolute known folder")
	}
	for p := base; ; p = filepath.Dir(p) {
		if err = rejectReparseDirectory(p); err != nil {
			return "", err
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	root := filepath.Join(base, "godusevpn-ultra")
	if err = createPrivateDirectory(root); err != nil {
		return "", err
	}
	// Do not silently take over a folder planted by an ordinary user: its owner
	// could later replace the private child even after that child's ACL changed.
	if err = checkTrustedDirectory(root); err != nil {
		return "", fmt.Errorf("机器隐私目录的父目录未受管理员保护；请由管理员独立审查并明确迁移: %w", err)
	}
	dir := filepath.Join(root, "privacy-machine")
	if err = createPrivateDirectory(dir); err != nil {
		return "", err
	}
	// Existing objects are validated, never tightened/adopted implicitly. v1
	// account-specific ACLs require a separate reviewed migration, not a report.
	if err = privacy.ValidateMachineStoreDirectory(dir); err != nil {
		return "", fmt.Errorf("机器隐私备份权限需要管理员独立审查；未修改旧对象或备份: %w", err)
	}
	return dir, nil
}

func trustedSID(s string) bool { return s == "S-1-5-18" || s == "S-1-5-32-544" }

func rejectReparseDirectory(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("machine privacy directory contains a reparse point or non-directory")
	}
	return nil
}

func createPrivateDirectory(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(privateSDDL)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(p, &sa)
	runtime.KeepAlive(sd)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return rejectReparseDirectory(path)
	}
	return err
}

func checkTrustedDirectory(path string) error {
	if err := rejectReparseDirectory(path); err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	return trustedDescriptor(sd)
}

func trustedDescriptor(sd *windows.SECURITY_DESCRIPTOR) error {
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trustedSID(owner.String()) {
		return errors.New("untrusted directory owner")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("missing directory access control")
	}
	const dangerous = 0x10000000 | 0x40000000 | 0x00000002 | 0x00000004 | 0x00000010 | 0x00000040 | 0x00000100 | 0x00010000 | 0x00040000 | 0x00080000
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace == nil {
			return errors.New("invalid access control entry")
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if uint32(ace.Mask)&dangerous != 0 {
				sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
				if !trustedSID(sid.String()) {
					return errors.New("non-administrator can modify directory")
				}
			}
		default:
			return errors.New("unsupported directory access control entry")
		}
	}
	return nil
}
