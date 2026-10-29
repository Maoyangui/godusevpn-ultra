package maintenance

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const mutexName = `Global\GodUseVPN-Ultra-Privacy-Maintenance-v1`
const gateAccess = windows.READ_CONTROL | windows.SYNCHRONIZE | windows.MUTEX_MODIFY_STATE
const gateSDDL = "O:BAG:BAD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x00120001;;;IU)"

var pinMu sync.Mutex
var pinned windows.Handle
var activeMu sync.Mutex
var activeThreads = map[uint32]bool{}

func trustedSID(s string) bool { return s == "S-1-5-18" || s == "S-1-5-32-544" }

func checkDescriptor(sd *windows.SECURITY_DESCRIPTOR) error {
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trustedSID(owner.String()) {
		return ErrUnavailable
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return ErrUnavailable
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return ErrUnavailable
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil {
			return ErrUnavailable
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrUnavailable
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if !trustedSID(sid) && (sid != "S-1-5-4" || uint32(ace.Mask)&^uint32(gateAccess) != 0) {
			return ErrUnavailable
		}
	}
	return nil
}

func openGate(create bool) (windows.Handle, error) {
	name, _ := windows.UTF16PtrFromString(mutexName)
	h, err := windows.OpenMutex(gateAccess, false, name)
	if err != nil && errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && create && windows.GetCurrentProcessToken().IsElevated() {
		sd, e := windows.SecurityDescriptorFromString(gateSDDL)
		if e != nil {
			return 0, ErrUnavailable
		}
		sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
		h, err = windows.CreateMutexEx(&sa, name, 0, gateAccess)
		runtime.KeepAlive(sd)
	}
	if err != nil {
		return 0, ErrUnavailable
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		windows.CloseHandle(h)
		return 0, ErrUnavailable
	}
	if err = checkDescriptor(sd); err != nil {
		windows.CloseHandle(h)
		return 0, err
	}
	return h, nil
}

// Prepare pins an admin-created synchronization object for this helper's life.
// Ordinary callers neither create nor take ownership of absent/foreign objects.
func Prepare() error {
	pinMu.Lock()
	defer pinMu.Unlock()
	if pinned != 0 {
		return nil
	}
	h, err := openGate(true)
	if err == nil {
		pinned = h
	}
	return err
}
func Ready() (bool, error) {
	h, err := openGate(false)
	if err != nil {
		return false, err
	}
	windows.CloseHandle(h)
	return true, nil
}

func Acquire() (_ *Lease, err error) {
	runtime.LockOSThread()
	tid := windows.GetCurrentThreadId()
	activeMu.Lock()
	recursive := activeThreads[tid]
	activeMu.Unlock()
	if recursive {
		runtime.UnlockOSThread()
		return nil, errors.New("禁止递归获取维护门禁；内部操作必须显式传递当前许可")
	}
	keep := false
	defer func() {
		if !keep {
			runtime.UnlockOSThread()
		}
	}()
	h, err := openGate(true)
	if err != nil {
		return nil, err
	}
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		windows.CloseHandle(h)
		return nil, ErrUnavailable
	}
	if status == uint32(windows.WAIT_ABANDONED) {
		windows.ReleaseMutex(h)
		windows.CloseHandle(h)
		return nil, ErrAbandoned
	}
	if status != windows.WAIT_OBJECT_0 {
		windows.CloseHandle(h)
		return nil, ErrBusy
	}
	keep = true
	activeMu.Lock()
	activeThreads[tid] = true
	activeMu.Unlock()
	return newLease(tid, windows.GetCurrentThreadId, func() error {
		defer runtime.UnlockOSThread()
		activeMu.Lock()
		delete(activeThreads, tid)
		activeMu.Unlock()
		return errors.Join(windows.ReleaseMutex(h), windows.CloseHandle(h))
	}), nil
}
