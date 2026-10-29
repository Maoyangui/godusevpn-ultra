//go:build linux

package trial

import (
	"os"
	"strings"
)

func platformDeviceID() string {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(b)) != "" {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}
