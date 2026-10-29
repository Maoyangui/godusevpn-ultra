//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeOwner struct {
	parent    windows.Handle
	root, sid string
}

// New accepts only the already authenticated GUI process HANDLE held by the
// fixed helper. Paths and user context are derived from its token, including
// UAC elevation using a different administrator account.
func New(parent windows.Handle) (*Manager, error) {
	pid, err := windows.GetProcessId(parent)
	if err != nil || pid == 0 {
		return nil, refusal("owner_process_id", err)
	}
	var before, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(parent, &before, &exit, &kernel, &user); err != nil {
		return nil, refusal("owner_process_times", err)
	}
	duplicate, err := openAuthenticatedOwner(parent, nativeOwnerAccess{})
	if err != nil {
		return nil, err
	}
	complete := false
	defer func() {
		if !complete {
			windows.CloseHandle(duplicate)
		}
	}()
	var after windows.Filetime
	if err = windows.GetProcessTimes(duplicate, &after, &exit, &kernel, &user); err != nil || before != after {
		return nil, refusal("owner_process_identity", err)
	}
	var token windows.Token
	// SHGetKnownFolderPath requires QUERY|IMPERSONATE for a non-null user
	// token. This obtains the authenticated GUI user's folder when UAC used
	// another administrator account; TOKEN_DUPLICATE is not needed here.
	if err = windows.OpenProcessToken(duplicate, windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE, &token); err != nil {
		return nil, refusal("owner_token_open", err)
	}
	defer token.Close()
	account, err := token.GetTokenUser()
	if err != nil || account.User.Sid == nil {
		return nil, refusal("owner_token_identity", err)
	}
	var elevated, used uint32
	if err = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &used); err != nil || used != 4 || elevated != 0 {
		return nil, refusal("owner_token_elevation", err)
	}
	// Do not let Shell verify a user-redirected KnownFolder before its namespace
	// has been checked. In alternate-admin UAC this lookup still uses the GUI
	// token, and no UNC/device/ADS path is opened with administrator credentials.
	base, err := token.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DONT_VERIFY)
	if err != nil {
		return nil, refusal("owner_known_folder", err)
	}
	if err = verifyLocalDirectory(base, checkFixedLocalDrive); err != nil {
		return nil, refusal("owner_local_drive", err)
	}
	owner := &nativeOwner{parent: duplicate, root: filepath.Join(base, "GodUseVPN Ultra", "BrowserInstances"), sid: account.User.Sid.String()}
	m := &Manager{records: map[string]*record{}, ownerAlive: owner.alive, closeOwner: func() { windows.CloseHandle(duplicate) }}
	m.open = owner.open
	// Native filter installation is wired in native_wfp_windows.go. A missing
	// backend is a refusal in Manager.Install, never a successful guard.
	m.backend = newNativeFirewall()
	complete = true
	return m, nil
}

func checkFixedLocalDrive(root string) error {
	device, err := windows.UTF16PtrFromString(root[:2])
	if err != nil {
		return ErrRefused
	}
	var target [32768]uint16
	if _, err := windows.QueryDosDevice(device, &target[0], uint32(len(target))); err != nil {
		return err
	}
	if !fixedVolumeMapping(windows.UTF16ToString(target[:])) {
		return ErrRefused
	}
	// QueryDosDevice above refuses network providers/SUBST aliases first; only
	// a confirmed HarddiskVolume mapping reaches the drive-type query.
	if windows.GetDriveType(windows.StringToUTF16Ptr(root)) != windows.DRIVE_FIXED {
		return ErrRefused
	}
	return nil
}
func (o *nativeOwner) alive() bool {
	var code uint32
	return windows.GetExitCodeProcess(o.parent, &code) == nil && code == 259
}
func (o *nativeOwner) open(r InstallRequest) (sessionLease, error) { return openNativeLease(o, r) }
