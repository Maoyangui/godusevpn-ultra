//go:build windows

// Package identity keeps the Ultra fork's machine resources separate from the
// original product. Changing display text alone would not provide isolation.
package identity

const (
	Ultra            = true
	AppName          = "godusevpn-ultra"
	DisplayName      = "GodUseVPN Ultra"
	PipeName         = `\\.\pipe\godusevpn-ultra`
	ProtocolScheme   = "godusevpn-ultra"
	SingleInstanceID = "godusevpn-ultra-7fa3c840-b9fd-489e-ae19-1c932fe6a85d"
	TunName          = "godusevpn-ultra"
	TunAddr4         = "172.30.215.1/30"
	TunAddr6         = "fdcb:ef12:9387::1/126"
	MixedPort        = 12080
	ClashPort        = 19090
	DataEnvironment  = "GODUSEVPN_ULTRA_DATA"
)
