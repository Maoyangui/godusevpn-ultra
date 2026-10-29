//go:build windows

package detection

import (
	"strings"

	"github.com/Maoyangui/godusevpn/internal/timeprivacy"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ReadFacts never opens devices, requests permissions or queries the network.
// ConsentStore is read as a bounded default record, never treated as the full
// Windows per-app permission decision or modified through undocumented keys.
func ReadFacts() Facts {
	zone, _ := timeprivacy.CurrentID()
	f := Facts{Platform: "windows", Elevated: windows.GetCurrentProcessToken().IsElevated(), TimeZone: zone}
	read := func(root registry.Key, path, name string) string {
		k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
		if err != nil {
			return ""
		}
		defer k.Close()
		n, typ, err := k.GetValue(name, nil)
		if err != nil || n < 2 || n > maxFactStringBytes || n%2 != 0 || (typ != registry.SZ && typ != registry.EXPAND_SZ) {
			return ""
		}
		data := make([]byte, n)
		read, typ, err := k.GetValue(name, data)
		if err != nil || read > len(data) {
			return ""
		}
		return boundedFactString(data[:read], typ)
	}
	f.Locale = normalizedLocale(read(registry.CURRENT_USER, `Control Panel\International`, "LocaleName"))
	consent := func(cap string) string {
		v := read(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\`+cap, "Value")
		if !strings.EqualFold(v, "Allow") && !strings.EqualFold(v, "Deny") {
			return ""
		}
		return v
	}
	f.Location = consent("location")
	f.Camera = consent("webcam")
	f.Microphone = consent("microphone")
	// Windows stores these capabilities separately from camera/microphone.
	// Missing keys remain unknown; do not infer a deny from an absent record.
	f.Notifications = consent("notifications")
	if f.Notifications == "" {
		f.Notifications = consent("userNotificationListener")
	}
	f.Clipboard = consent("clipboard")
	return f
}
