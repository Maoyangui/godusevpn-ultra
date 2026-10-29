//go:build windows

package regionprivacy

import (
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeBackend struct{}

var geoKernel = windows.NewLazySystemDLL("kernel32.dll")
var geoGet = geoKernel.NewProc("GetUserDefaultGeoName")
var geoSet = geoKernel.NewProc("SetUserGeoName")
var geoEnum = geoKernel.NewProc("EnumSystemGeoNames")
var geoInfo = geoKernel.NewProc("GetGeoInfoEx")
var enumCallbackOnce sync.Once
var enumCallback uintptr
var enumSequence atomic.Uintptr
var enumStates sync.Map

type enumState struct {
	codes []string
	bad   bool
}

func ordinaryIdentity(sid, integrity string, elevated, impersonating bool) bool {
	return sid != "" && sid != "S-1-5-18" && sid != "S-1-5-19" && sid != "S-1-5-20" && !elevated && !impersonating && (integrity == "S-1-16-8192" || integrity == "S-1-16-8448")
}

func (nativeBackend) identity() (userIdentity, error) {
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
	return userIdentity{sid: user.User.Sid.String(), directory: filepath.Join(base, "GodUseVPN Ultra", "PrivacyRegion")}, nil
}

func decodeGeoBuffer(buffer []uint16, result uintptr) (string, error) {
	if result == 0 || result > uintptr(len(buffer)) {
		return "", ErrReadback
	}
	end := -1
	for i, value := range buffer {
		if value == 0 {
			end = i
			break
		}
	}
	if end < 0 || uintptr(end+1) != result {
		return "", ErrReadback
	}
	code := windows.UTF16ToString(buffer[:end])
	if !codePattern.MatchString(code) {
		return "", ErrReadback
	}
	return code, nil
}

func decodeGeoLabel(buffer []uint16, result uintptr) (string, error) {
	end := -1
	for index, unit := range buffer {
		if unit == 0 {
			end = index
			break
		}
	}
	// The documentation describes a byte count despite its WCHAR/cch buffer.
	// Accept either count only for the same complete terminated result.
	if end <= 0 || result != uintptr(end+1) && result != uintptr((end+1)*2) {
		return "", ErrUnsupported
	}
	value := windows.UTF16ToString(buffer[:end])
	if stringsHaveControls(value) {
		return "", ErrUnsupported
	}
	return value, nil
}

func (b nativeBackend) current() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := b.identity(); err != nil || geoGet.Find() != nil {
		return "", ErrUnsupported
	}
	var code [8]uint16
	result, _, _ := geoGet.Call(uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)))
	return decodeGeoBuffer(code[:], result)
}

func (b nativeBackend) set(code string) error {
	if !codePattern.MatchString(code) {
		return ErrInput
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := b.identity(); err != nil || geoSet.Find() != nil {
		return ErrUnsupported
	}
	text, err := windows.UTF16PtrFromString(code)
	if err != nil {
		return ErrInput
	}
	result, _, _ := geoSet.Call(uintptr(unsafe.Pointer(text)))
	if result == 0 {
		return ErrReadback
	}
	return nil
}

func geoEnumerationCallback(name *uint16, id uintptr) uintptr {
	value, ok := enumStates.Load(id)
	if !ok || name == nil {
		return 0
	}
	state := value.(*enumState)
	if len(state.codes) >= 1024 {
		state.bad = true
		return 0
	}
	var code [4]uint16
	for index := range code {
		code[index] = *(*uint16)(unsafe.Add(unsafe.Pointer(name), index*2))
		if code[index] == 0 {
			decoded, err := decodeGeoBuffer(code[:], uintptr(index+1))
			if err != nil {
				state.bad = true
				return 0
			}
			state.codes = append(state.codes, decoded)
			return 1
		}
	}
	state.bad = true
	return 0
}

func (nativeBackend) choices() ([]Choice, error) {
	if geoEnum.Find() != nil || geoInfo.Find() != nil || geoGet.Find() != nil || geoSet.Find() != nil {
		return nil, ErrUnsupported
	}
	enumCallbackOnce.Do(func() { enumCallback = windows.NewCallback(geoEnumerationCallback) })
	id := enumSequence.Add(1)
	if id == 0 {
		return nil, ErrUnsupported
	}
	state := &enumState{}
	enumStates.Store(id, state)
	defer enumStates.Delete(id)
	// GEOCLASS_ALL (0) includes genuine ISO and M.49 regions. Never synthesize
	// an initial country list or drop a numeric original that must be restorable.
	result, _, _ := geoEnum.Call(0, enumCallback, id)
	if result == 0 || state.bad || len(state.codes) == 0 {
		return nil, ErrUnsupported
	}
	choices := make([]Choice, 0, len(state.codes))
	for _, code := range state.codes {
		name, _ := windows.UTF16PtrFromString(code)
		var label [256]uint16
		result, _, _ := geoInfo.Call(uintptr(unsafe.Pointer(name)), 8, uintptr(unsafe.Pointer(&label[0])), uintptr(len(label))) // GEO_FRIENDLYNAME only; no coordinates.
		friendly, err := decodeGeoLabel(label[:], result)
		if err != nil {
			return nil, ErrUnsupported
		}
		choices = append(choices, Choice{Code: code, Label: friendly + " (" + code + ")"})
	}
	return checkedChoices(choices)
}
