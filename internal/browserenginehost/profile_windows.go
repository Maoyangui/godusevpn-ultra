//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

type profileLease struct {
	directory, sessionRoot string
	files                  []*os.File
	release                func()
}

func (p *profileLease) Close() {
	for i := len(p.files) - 1; i >= 0; i-- {
		p.files[i].Close()
	}
	if p.release != nil {
		p.release()
	}
}

type unicodeString struct {
	length, max uint16
	buffer      *uint16
}
type objectAttributes struct {
	length     uint32
	root       windows.Handle
	name       *unicodeString
	attributes uint32
	sd         *windows.SECURITY_DESCRIPTOR
	qos        uintptr
}
type ioStatus struct{ status, information uintptr }

var ntCreateFile = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtCreateFile")

func atomicDirectory(parent windows.Handle, name string, sd *windows.SECURITY_DESCRIPTOR) (*os.File, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:*?"<>|`) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return nil, ErrRefused
	}
	units, err := windows.UTF16FromString(name)
	if err != nil || len(units) > 256 {
		return nil, ErrRefused
	}
	text := unicodeString{uint16((len(units) - 1) * 2), uint16(len(units) * 2), &units[0]}
	attributes := objectAttributes{length: uint32(unsafe.Sizeof(objectAttributes{})), root: parent, name: &text, attributes: 0x40 | 0x1000, sd: sd}
	var status ioStatus
	var handle windows.Handle
	// FILE_CREATE + DIRECTORY_FILE + OPEN_REPARSE_POINT is one creation/handle
	// acquisition. No create-then-reopen gap and no reuse on a name collision.
	result, _, _ := ntCreateFile.Call(uintptr(unsafe.Pointer(&handle)), windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.SYNCHRONIZE, uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(&status)), 0, windows.FILE_ATTRIBUTE_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, 2, 1|0x20|0x200000, 0, 0)
	runtime.KeepAlive(units)
	runtime.KeepAlive(sd)
	if result != 0 || handle == 0 || handle == windows.InvalidHandle || status.information != 2 {
		if handle != 0 && handle != windows.InvalidHandle {
			windows.CloseHandle(handle)
		}
		return nil, ErrRefused
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(handle)
		return nil, ErrRefused
	}
	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || !sameProfileDescriptor(actual, sd) {
		windows.CloseHandle(handle)
		return nil, ErrRefused
	}
	return os.NewFile(uintptr(handle), "ultra-fresh-browser-directory"), nil
}
func sameProfileDescriptor(actual, expected *windows.SECURITY_DESCRIPTOR) bool {
	if actual == nil || expected == nil || !actual.IsValid() || !expected.IsValid() {
		return false
	}
	owner, defaulted, err := actual.Owner()
	want, _, wantErr := expected.Owner()
	if err != nil || wantErr != nil || defaulted || owner == nil || want == nil || !owner.Equals(want) {
		return false
	}
	control, _, err := actual.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	acl, defaulted, err := actual.DACL()
	wantACL, _, wantErr := expected.DACL()
	if err != nil || wantErr != nil || defaulted || acl == nil || wantACL == nil || acl.AceCount != wantACL.AceCount || acl.AceCount != 3 {
		return false
	}
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace, wantACE *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, index, &ace) != nil || windows.GetAce(wantACL, index, &wantACE) != nil || ace == nil || wantACE == nil || ace.Header != wantACE.Header || ace.Mask != wantACE.Mask {
			return false
		}
		identity := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		target := (*windows.SID)(unsafe.Pointer(&wantACE.SidStart))
		if !identity.IsValid() || !target.IsValid() || !identity.Equals(target) || uint32(ace.Header.AceSize) != 8+uint32(identity.Len()) {
			return false
		}
	}
	return true
}
func createProfile(manager *browserengine.Manager, sessionID string, c Config, relayPort int) (_ *profileLease, result error) {
	sessionRoot, err := manager.InstanceDirectory(sessionID)
	if err != nil {
		return nil, ErrRefused
	}
	container := filepath.Dir(sessionRoot)
	release, err := localstate.AcquireDirectory(container, true)
	if err != nil {
		return nil, ErrRefused
	}
	lease := &profileLease{release: release}
	complete := false
	defer func() {
		if !complete {
			lease.Close()
		}
	}()
	parent, err := os.OpenRoot(container)
	if err != nil {
		return nil, ErrRefused
	}
	defer parent.Close()
	parentHandle, err := parent.Open(".")
	if err != nil {
		return nil, ErrRefused
	}
	defer parentHandle.Close()
	account, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || account.User.Sid == nil {
		return nil, ErrRefused
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + account.User.Sid.String() + "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + account.User.Sid.String() + ")")
	if err != nil {
		return nil, ErrRefused
	}
	session, err := atomicDirectory(windows.Handle(parentHandle.Fd()), sessionID, sd)
	if err != nil {
		return nil, ErrRefused
	}
	lease.files = append(lease.files, session)
	profile, err := atomicDirectory(windows.Handle(session.Fd()), "profile", sd)
	if err != nil {
		return nil, ErrRefused
	}
	lease.files = append(lease.files, profile)
	lease.sessionRoot = sessionRoot
	lease.directory = filepath.Join(lease.sessionRoot, "profile")
	root, err := os.OpenRoot(lease.directory)
	if err != nil {
		return nil, ErrRefused
	}
	defer root.Close()
	write := func(name string, data []byte) error {
		file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return ErrRefused
		}
		defer file.Close()
		if n, err := file.Write(data); err != nil || n != len(data) || file.Sync() != nil {
			return ErrRefused
		}
		return nil
	}
	if c.EngineID == "firefox" {
		if write("user.js", firefoxPreferences(c, relayPort)) != nil {
			return nil, ErrRefused
		}
	} else {
		if root.Mkdir("Default", 0700) != nil || write("Default/Preferences", chromePreferences(c)) != nil {
			return nil, ErrRefused
		}
	}
	complete = true
	return lease, nil
}
