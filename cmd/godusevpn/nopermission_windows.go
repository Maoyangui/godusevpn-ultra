//go:build windows

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Maoyangui/godusevpn/internal/svc"
)

// noPermissionHint 控制口拒绝这个账户时提示的种类,以及界面能不能替用户登记。
// Windows:控制管道有一份账户名单,界面可以提权跑 register-controller --restart 把本账户加进去。
func noPermissionHint() (string, bool) { return "register", true }

var (
	modShell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = modShell32.NewProc("ShellExecuteExW")
	modWTSAPI32         = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySession = modWTSAPI32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory   = modWTSAPI32.NewProc("WTSFreeMemory")
)

// shellExecuteInfo SHELLEXECUTEINFOW,字段顺序与对齐照 C 的来(64 位上共 112 字节)。
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	wtsUserName           = 5
)

// interactiveSessionSID resolves the account that owns the current interactive
// Windows session.  A compatibility shim or an installer-launched client can
// make the GUI elevated before this code runs; in that case the process token
// belongs to the administrator supplied to UAC, while WTS still identifies the
// standard user who owns the desktop session.
func interactiveSessionSID() (*windows.SID, error) {
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		return nil, err
	}
	var raw *uint16
	var bytes uint32
	r, _, callErr := procWTSQuerySession.Call(0, uintptr(session), uintptr(wtsUserName), uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&bytes)))
	if r == 0 || raw == nil {
		if callErr != nil {
			return nil, callErr
		}
		return nil, errors.New("WTSQuerySessionInformationW failed")
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(raw)))
	name := windows.UTF16PtrToString(raw)
	if name == "" {
		return nil, errors.New("interactive session has no user")
	}
	account, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	var sidLen, domainLen, use uint32
	err = windows.LookupAccountName(nil, account, nil, &sidLen, nil, &domainLen, &use)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || sidLen == 0 {
		if err == nil {
			return nil, errors.New("account SID size was not returned")
		}
		return nil, err
	}
	sidBytes := make([]byte, sidLen)
	domain := make([]uint16, domainLen)
	sid := (*windows.SID)(unsafe.Pointer(&sidBytes[0]))
	if err := windows.LookupAccountName(nil, account, sid, &sidLen, &domain[0], &domainLen, &use); err != nil {
		return nil, err
	}
	return sid, nil
}

func registrationSID() (*windows.SID, error) {
	if sid, err := interactiveSessionSID(); err == nil && sid != nil {
		return sid, nil
	}
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return nil, errors.New("无法读取当前账户 SID")
	}
	return user.User.Sid, nil
}

// registerController 提权跑 register-controller --restart 并**等它跑完**,按退出码报结果。
// 用户在 UAC 里点了"否"、提权进程失败、三分钟还没跑完,都如实报错,不再一律报成功。
func registerController(svcExe string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(svcExe)
	// The elevated helper runs with the administrator token supplied to UAC.
	// Passing no target would therefore register that administrator instead of
	// the ordinary user who pressed “Register this account”. Resolve the SID
	// before elevation and make the target explicit; SIDs contain only the
	// restricted characters accepted by the service command.
	sid, err := registrationSID()
	if err != nil || sid == nil {
		return errors.New("E_REGISTER_FAILED: 无法读取当前账户 SID")
	}
	paramsText := "register-controller " + sid.String() + " --restart"
	params, _ := windows.UTF16PtrFromString(paramsText)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(svcExe))
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess | seeMaskNoAsync, lpVerb: verb, lpFile: file, lpParameters: params, lpDirectory: dir, nShow: windows.SW_HIDE}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, e := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(e, windows.ERROR_CANCELLED) {
			return errors.New("E_UAC_CANCELLED: ")
		}
		return fmt.Errorf("E_REGISTER_FAILED: %v", e)
	}
	if info.hProcess == 0 {
		return errors.New("E_REGISTER_FAILED: ")
	}
	defer windows.CloseHandle(info.hProcess)
	ev, err := windows.WaitForSingleObject(info.hProcess, 180*1000)
	if err != nil {
		return fmt.Errorf("E_REGISTER_FAILED: %v", err)
	}
	if ev != windows.WAIT_OBJECT_0 {
		return errors.New("E_REGISTER_FAILED: ")
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return fmt.Errorf("E_REGISTER_FAILED: %v", err)
	}
	// 退出码的含义见 godusevpn-svc 的 register-controller
	switch code {
	case 0:
		return nil
	case 2:
		return errors.New("E_REGISTER_NOT_RESTARTED: ")
	case 3:
		// 登记成了、服务停着没拉起来:服务 ACL 允许普通用户启动,这里再试一次,免得严格全局下整机断网
		if err := svc.StartUser(); err == nil {
			return nil
		}
		return errors.New("E_REGISTER_SERVICE_DOWN: ")
	default:
		return fmt.Errorf("E_REGISTER_FAILED: %d", code)
	}
}
