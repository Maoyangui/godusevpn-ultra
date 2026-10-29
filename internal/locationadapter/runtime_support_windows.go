//go:build windows

package locationadapter

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type rtlOSVersionInfoEx struct {
	OSVersionInfoSize uint32
	MajorVersion      uint32
	MinorVersion      uint32
	BuildNumber       uint32
	PlatformID        uint32
	CSDVersion        [128]uint16
	ServicePackMajor  uint16
	ServicePackMinor  uint16
	SuiteMask         uint16
	ProductType       byte
	_                 byte
}

var rtlGetVersion = windows.NewLazySystemDLL("ntdll.dll").NewProc("RtlGetVersion")

func runtimeSupportStatus() RuntimeSupport {
	r := RuntimeSupport{
		Platform:             "windows",
		ProviderBuildFloor:   providerBuildFloor,
		RequiresLAF:          true,
		RequiresRunFullTrust: true,
		Status:               "unknown",
	}
	info := rtlOSVersionInfoEx{OSVersionInfoSize: uint32(unsafe.Sizeof(rtlOSVersionInfoEx{}))}
	if err := rtlGetVersion.Find(); err != nil {
		r.Reason = "无法读取 Windows 版本；不会启用 GeolocationProvider。"
		return r
	}
	code, _, _ := rtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	if code != 0 {
		r.Reason = "Windows 版本读取失败；不会启用 GeolocationProvider。"
		return r
	}
	r.Build = info.BuildNumber
	r.ProviderAPIAvailable = providerBuildSupported(r.Build)
	if !r.ProviderAPIAvailable {
		r.Status = "build_too_old"
		r.Reason = fmt.Sprintf("Windows build %d 低于 GeolocationProvider 文档门槛 %d；仅保留独立读取或 Device Portal 路径。", r.Build, providerBuildFloor)
		return r
	}
	r.Status = "build_supported"
	r.Reason = fmt.Sprintf("Windows build %d 达到 GeolocationProvider 文档门槛；仍需运行时类型、Microsoft LAF、runFullTrust、会话所有权和原子写入检查。", r.Build)
	return r
}
