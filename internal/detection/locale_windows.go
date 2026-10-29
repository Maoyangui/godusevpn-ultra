package detection

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var localeKernel = windows.NewLazySystemDLL("kernel32.dll")
var isValidLocaleName = localeKernel.NewProc("IsValidLocaleName")
var getLocaleInfoEx = localeKernel.NewProc("GetLocaleInfoEx")

func normalizedLocale(raw string) string { return checkedLocale(raw, knownWindowsLocale) }

// Both calls operate on the supplied synthetic/bounded identifier; this does
// not read a default/user locale, registry, devices, or network. ResolveLocaleName
// is deliberately not used: fallback must not turn an invalid value into fact.
func knownWindowsLocale(name string) string {
	if isValidLocaleName.Find() != nil || getLocaleInfoEx.Find() != nil {
		return ""
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return ""
	}
	valid, _, _ := isValidLocaleName.Call(uintptr(unsafe.Pointer(p)))
	if valid == 0 {
		return ""
	}
	var result [85]uint16 // LOCALE_NAME_MAX_LENGTH, including the terminator.
	const localeSNameNoUserOverride = 0x0000005c | 0x80000000
	n, _, _ := getLocaleInfoEx.Call(uintptr(unsafe.Pointer(p)), localeSNameNoUserOverride, uintptr(unsafe.Pointer(&result[0])), uintptr(len(result)))
	if n < 2 || n > uintptr(len(result)) || result[n-1] != 0 {
		return ""
	}
	return windows.UTF16ToString(result[:n-1])
}
