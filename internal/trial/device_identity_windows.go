//go:build windows

package trial

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// platformDeviceID reads the Windows machine identity locally. Only its
// client-side SHA-256 digest is sent to the trial backend.
func platformDeviceID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\\Microsoft\\Cryptography`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}
