//go:build windows

package locationportal

import (
	"golang.org/x/sys/windows"
	"path/filepath"
	"runtime"
	"unsafe"
)

func ordinaryIdentity(sid, integrity string, elevated, impersonating bool) bool {
	return sid != "" && sid != "S-1-5-18" && sid != "S-1-5-19" && sid != "S-1-5-20" && !elevated && !impersonating && (integrity == "S-1-16-8192" || integrity == "S-1-16-8448")
}

func nativeIdentity() (userIdentity, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var thread windows.Token
	err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &thread)
	if err == nil {
		thread.Close()
		return userIdentity{}, ErrUnsupported
	}
	if err != windows.ERROR_NO_TOKEN {
		return userIdentity{}, ErrUnsupported
	}
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return userIdentity{}, ErrUnsupported
	}
	var elevated, used uint32
	if windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &used) != nil || used != 4 {
		return userIdentity{}, ErrUnsupported
	}
	var buffer [512]byte
	if windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buffer[0], uint32(len(buffer)), &used) != nil {
		return userIdentity{}, ErrUnsupported
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	if label.Label.Sid == nil || !ordinaryIdentity(user.User.Sid.String(), label.Label.Sid.String(), elevated != 0, false) {
		return userIdentity{}, ErrUnsupported
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DONT_VERIFY)
	if err != nil {
		return userIdentity{}, ErrUnsupported
	}
	return userIdentity{sid: user.User.Sid.String(), directory: filepath.Join(base, "GodUseVPN Ultra", "LocationPortal")}, nil
}
