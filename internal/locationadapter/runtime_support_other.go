//go:build !windows

package locationadapter

import "runtime"

func runtimeSupportStatus() RuntimeSupport {
	return RuntimeSupport{
		Platform:           runtime.GOOS,
		ProviderBuildFloor: providerBuildFloor,
		Status:             "unsupported",
		Reason:             "GeolocationProvider 适配器仅面向 Windows；未修改其他平台。",
	}
}
