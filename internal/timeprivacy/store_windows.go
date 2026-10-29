//go:build windows

package timeprivacy

import (
	"errors"
	"path/filepath"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

func secureDirectory(dir string) error {
	release, err := localstate.AcquireDirectory(dir, true)
	if err != nil {
		return err
	}
	defer release()
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sid := u.User.Sid.String()
	if sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20" {
		return errors.New("interactive user required")
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func replace(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func lockStore(dir string) (func(), error) {
	release, err := localstate.AcquireDirectory(dir, true)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			release()
		}
	}()
	if err := secureDirectory(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "timezone.lock")
	f, err := localstate.OpenRegular(path, true)
	if err != nil {
		return nil, err
	}
	ov := &windows.Overlapped{}
	if err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ov); err != nil {
		f.Close()
		return nil, err
	}
	keep = true
	return func() { _ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ov); _ = f.Close(); release() }, nil
}
