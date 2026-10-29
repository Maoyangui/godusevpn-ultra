//go:build windows

package identity

import "testing"

func TestUltraDoesNotReuseOriginalMachineResources(t *testing.T){
	if !Ultra||AppName=="godusevpn"||PipeName==`\\.\pipe\godusevpn`||ProtocolScheme=="godusevpn"||TunName=="godusevpn"||DataEnvironment=="GODUSEVPN_DATA"{t.Fatal("original product namespace reused")}
	if MixedPort==2080||ClashPort==9090||TunAddr4=="172.19.0.1/30"||TunAddr6=="fdfe:dcba:9876::1/126"{t.Fatal("default socket or TUN address reused")}
}
