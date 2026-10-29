//go:build !windows

package identity

// The first Ultra release targets Windows. Other platform builds retain the
// upstream identifiers and must not be distributed as an Ultra release.
const (
	Ultra            = false
	AppName          = "godusevpn"
	DisplayName      = "佛跳墙"
	PipeName         = ""
	ProtocolScheme   = "godusevpn"
	SingleInstanceID = "godusevpn-3c1f8d2e-7b4a-4e0c-9a6d-fotiaoqiang"
	TunName          = "godusevpn"
	TunAddr4         = "172.19.0.1/30"
	TunAddr6         = "fdfe:dcba:9876::1/126"
	MixedPort        = 2080
	ClashPort        = 9090
	DataEnvironment  = "GODUSEVPN_DATA"
)
