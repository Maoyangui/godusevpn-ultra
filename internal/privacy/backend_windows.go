//go:build windows

package privacy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type windowsBackend struct{}

func newBackend() backend { return windowsBackend{} }

func currentUser() (string, bool) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", false
	}
	sid := u.User.Sid.String()
	// Never let an installed service alter HKCU for SYSTEM / service accounts.
	if sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20" || strings.HasPrefix(sid, "S-1-5-80-") || strings.HasPrefix(sid, "S-1-5-82-") || len(sid) < 10 {
		return "", false
	}
	groups, err := windows.GetCurrentProcessToken().GetTokenGroups()
	if err != nil {
		return "", false
	}
	interactive := false
	for _, g := range groups.AllGroups() {
		if g.Sid.String() == "S-1-5-4" {
			interactive = true
			break
		}
	}
	if !interactive {
		return "", false
	}
	return sid, true
}
func (windowsBackend) environment() environment {
	sid, safe := currentUser()
	sum := sha256.Sum256([]byte(sid))
	e := windowsEnvironment()
	e.SafeUser, e.UserKey = safe, hex.EncodeToString(sum[:])
	if !safe {
		e.Warnings = append(e.Warnings, "当前进程不是可确认的普通用户上下文，禁止自动修改用户隐私策略。")
	}
	return e
}

func (windowsBackend) machineEnvironment() environment {
	e := windowsEnvironment()
	e.Warnings = append(e.Warnings, "仅操作本产品固定 HKLM 机器策略；不访问当前或其他用户的 HKCU、浏览器或时区。")
	return e
}

func windowsEnvironment() environment {
	e := environment{Platform: "windows", Admin: windows.GetCurrentProcessToken().IsElevated(), Build: windows.RtlGetVersion().BuildNumber}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == nil {
		e.Edition, _, _ = k.GetStringValue("EditionID")
		k.Close()
	}
	return e
}
func registryRoot(hive string) (registry.Key, error) {
	switch hive {
	case "HKCU":
		return registry.CURRENT_USER, nil
	case "HKLM":
		return registry.LOCAL_MACHINE, nil
	}
	return 0, errors.New("unsupported hive")
}
func knownLocation(l location) bool {
	for _, c := range catalog() {
		for _, x := range c.Changes {
			if x.Location == l {
				return true
			}
			if x.Location.Hive == "HKCU" {
				higher := x.Location
				higher.Hive = "HKLM"
				if higher == l {
					return true
				}
			}
		}
	}
	return false
}
func writableLocation(l location) bool {
	for _, c := range catalog() {
		for _, x := range c.Changes {
			if x.Location == l {
				return true
			}
		}
	}
	return false
}
func (windowsBackend) read(l location) (value, error) {
	if !knownLocation(l) {
		return value{}, errors.New("location outside policy catalog")
	}
	root, err := registryRoot(l.Hive)
	if err != nil {
		return value{}, err
	}
	k, err := registry.OpenKey(root, l.Path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND) {
		return value{}, nil
	}
	if err != nil {
		return value{}, errors.New("policy read denied")
	}
	defer k.Close()
	n, typ, err := k.GetValue(l.Name, nil)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return value{}, nil
	}
	if err != nil {
		return value{}, errors.New("policy read failed")
	}
	if n > 65536 {
		return value{}, errors.New("policy exceeds backup limit")
	}
	b := make([]byte, n)
	n, typ, err = k.GetValue(l.Name, b)
	if err != nil {
		return value{}, errors.New("policy changed during read")
	}
	return value{true, typ, b[:n]}, nil
}

var advapi = windows.NewLazySystemDLL("advapi32.dll")
var regSetValue = advapi.NewProc("RegSetValueExW")
var regFlush = advapi.NewProc("RegFlushKey")

func (windowsBackend) write(l location, v value) error {
	_, safe := currentUser()
	if err := authorizeWindowsWrite(l, v, false, windows.GetCurrentProcessToken().IsElevated(), safe); err != nil {
		return err
	}
	return writeRegistryValue(l, v)
}

func (windowsBackend) writeMachine(l location, v value) error {
	// Do not route through the user writer or pretend SYSTEM is an interactive
	// user. The separate path checks the real token again and only four HKLM keys.
	if err := authorizeWindowsWrite(l, v, true, windows.GetCurrentProcessToken().IsElevated(), false); err != nil {
		return err
	}
	return writeRegistryValue(l, v)
}

func authorizeWindowsWrite(l location, v value, machine, elevated, safeUser bool) error {
	if !writableLocation(l) || !validValue(v) {
		return errors.New("policy outside writable catalog")
	}
	if machine && (!machineLocation(l) || !elevated) {
		return errors.New("elevated fixed machine-policy context required")
	}
	if !machine && !safeUser {
		return errors.New("unsafe user context")
	}
	if l.Hive == "HKLM" && !elevated {
		return errors.New("administrator required")
	}
	return nil
}

func writeRegistryValue(l location, v value) error {
	root, err := registryRoot(l.Hive)
	if err != nil {
		return err
	}
	var k registry.Key
	if v.Exists {
		k, _, err = registry.CreateKey(root, l.Path, registry.SET_VALUE|registry.QUERY_VALUE|registry.WOW64_64KEY)
	} else {
		k, err = registry.OpenKey(root, l.Path, registry.SET_VALUE|registry.QUERY_VALUE|registry.WOW64_64KEY)
	}
	if !v.Exists && (errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND)) {
		return nil
	}
	if err != nil {
		return errors.New("policy write denied")
	}
	defer k.Close()
	if !v.Exists {
		err = k.DeleteValue(l.Name)
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			err = nil
		}
		if err != nil {
			return errors.New("policy removal failed")
		}
	} else {
		name, err := windows.UTF16PtrFromString(l.Name)
		if err != nil {
			return err
		}
		var p *byte
		if len(v.Data) > 0 {
			p = &v.Data[0]
		}
		status, _, _ := regSetValue.Call(uintptr(k), uintptr(unsafe.Pointer(name)), 0, uintptr(v.Type), uintptr(unsafe.Pointer(p)), uintptr(len(v.Data)))
		runtime.KeepAlive(name)
		runtime.KeepAlive(v.Data)
		if status != 0 {
			return errors.New("policy write failed")
		}
	}
	status, _, _ := regFlush.Call(uintptr(k))
	if status != 0 {
		return errors.New("policy flush failed")
	}
	return nil
}
