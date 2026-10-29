//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeOwnerDebug struct{}

var ownerAdjustTokenPrivileges = windows.NewLazySystemDLL("advapi32.dll").NewProc("AdjustTokenPrivileges")
var ownerUnretiredThreads = struct {
	sync.Mutex
	handles map[windows.Handle]struct{}
}{handles: make(map[windows.Handle]struct{})}

func (nativeOwnerDebug) lockThread()   { runtime.LockOSThread() }
func (nativeOwnerDebug) unlockThread() { runtime.UnlockOSThread() }
func (nativeOwnerDebug) threadHandle() (windows.Handle, error) {
	var h windows.Handle
	err := windows.DuplicateHandle(windows.CurrentProcess(), windows.CurrentThread(), windows.CurrentProcess(), &h, windows.SYNCHRONIZE, false, 0)
	return h, err
}
func (nativeOwnerDebug) threadToken() (windows.Token, error) {
	var token windows.Token
	err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token)
	return token, err
}
func (nativeOwnerDebug) ownToken() (windows.Token, error) {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &token)
	return token, err
}
func (nativeOwnerDebug) duplicateToken(source windows.Token) (windows.Token, error) {
	var token windows.Token
	err := windows.DuplicateTokenEx(source, windows.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_IMPERSONATE, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &token)
	return token, err
}
func (nativeOwnerDebug) tokenCore(t windows.Token) (ownerTokenCore, error) {
	return readOwnerTokenCore(t)
}
func (nativeOwnerDebug) privileges(t windows.Token) (map[windows.LUID]uint32, error) {
	return readOwnerPrivileges(t)
}
func (nativeOwnerDebug) debugPrivilege() (windows.LUID, error) {
	var id windows.LUID
	err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeDebugPrivilege"), &id)
	return id, err
}

// x/sys's BOOL wrapper discards ERROR_NOT_ALL_ASSIGNED when BOOL is true.
// Capture LastError from the very same syscall instead of a later GetLastError.
func (nativeOwnerDebug) enablePrivilege(t windows.Token, id windows.LUID) error {
	p := windows.Tokenprivileges{PrivilegeCount: 1}
	p.Privileges[0] = windows.LUIDAndAttributes{Luid: id, Attributes: windows.SE_PRIVILEGE_ENABLED}
	r1, _, code := ownerAdjustTokenPrivileges.Call(uintptr(t), 0, uintptr(unsafe.Pointer(&p)), 0, 0, 0)
	runtime.KeepAlive(&p)
	return ownerPrivilegeAdjustmentResult(r1, code)
}

func ownerPrivilegeAdjustmentResult(success uintptr, code error) error {
	if code != nil && code != syscall.Errno(0) {
		return code
	}
	if success == 0 {
		return ErrRefused
	}
	return nil
}
func (nativeOwnerDebug) attach(t windows.Token) error { return windows.SetThreadToken(nil, t) }
func (nativeOwnerDebug) revert() error                { return windows.RevertToSelf() }
func (nativeOwnerDebug) open(pid, mask uint32) (windows.Handle, error) {
	return windows.OpenProcess(mask, false, pid)
}
func (nativeOwnerDebug) close(h windows.Handle) error { return windows.CloseHandle(h) }
func (nativeOwnerDebug) waitThread(h windows.Handle) (uint32, error) {
	return windows.WaitForSingleObject(h, 5000)
}
func (nativeOwnerDebug) retainThread(h windows.Handle) {
	// An uncertain thread never becomes a usable parent capability. Keep its
	// synchronization HANDLE as evidence; do not close or reuse it on timeout.
	ownerUnretiredThreads.Lock()
	ownerUnretiredThreads.handles[h] = struct{}{}
	ownerUnretiredThreads.Unlock()
}
