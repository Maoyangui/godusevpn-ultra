//go:build windows && (amd64 || arm64)

package locationportalgate

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestInstalledAutoStartServiceRequiresFixedCodeIdentity(t *testing.T) {
	const image = `C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe`
	const command = `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" service`
	for _, accepted := range []string{command, `"c:\program files\GodUseVPN Ultra\godusevpn-svc.exe" "service"`} {
		if !fixedInstalledServiceConfig(windows.SERVICE_AUTO_START, windows.SERVICE_WIN32_OWN_PROCESS, "LocalSystem", accepted, image) {
			t.Fatal("fixed installed AutoStart command refused")
		}
	}
	for _, rejected := range []string{
		`C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe service`,
		`"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" Service`,
		command + " --anything", " " + command, command + " ",
		`"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe.bak" service`,
		`"C:\Users\qa\godusevpn-svc.exe" service`,
		`"C:\Program Files\GodUseVPN Ultra\godusevpn-ſvc.exe" service`,
		`"%ProgramFiles%\GodUseVPN Ultra\godusevpn-svc.exe" service`,
	} {
		if fixedInstalledServiceConfig(windows.SERVICE_AUTO_START, windows.SERVICE_WIN32_OWN_PROCESS, "LocalSystem", rejected, image) {
			t.Fatal("nonfixed service command accepted")
		}
	}
	for _, pair := range [][2]uint32{{windows.SERVICE_DEMAND_START, windows.SERVICE_WIN32_OWN_PROCESS}, {windows.SERVICE_DISABLED, windows.SERVICE_WIN32_OWN_PROCESS}, {windows.SERVICE_AUTO_START, windows.SERVICE_WIN32_SHARE_PROCESS}, {windows.SERVICE_AUTO_START, windows.SERVICE_WIN32_OWN_PROCESS | windows.SERVICE_INTERACTIVE_PROCESS}} {
		if fixedInstalledServiceConfig(pair[0], pair[1], "LocalSystem", command, image) {
			t.Fatal("wrong startup/type accepted", pair)
		}
	}
	for _, account := range []string{"", `NT AUTHORITY\SYSTEM`, `Machine\LocalSystem`, "LocalSyſtem", "qa"} {
		if fixedInstalledServiceConfig(windows.SERVICE_AUTO_START, windows.SERVICE_WIN32_OWN_PROCESS, account, command, image) {
			t.Fatal("noncanonical account accepted")
		}
	}
}

func TestProviderServiceAssociationIsExactAndBounded(t *testing.T) {
	if !fixedProviderService(windows.StringToUTF16Ptr(providerServiceName)) {
		t.Fatal("fixed service refused")
	}
	for _, name := range []string{"", "BFE", "WebManagement", providerServiceName + "-other", "GODUSEVPN-ULTRA"} {
		if fixedProviderService(windows.StringToUTF16Ptr(name)) {
			t.Fatal("unexpected association accepted")
		}
	}
	if fixedProviderService(nil) || sameASCIIPath("aé", "aê") || sameASCIIPath("svc", "ſvc") {
		t.Fatal("nil association or Unicode alias accepted")
	}
}

func TestServiceConfigStringsStayInsideOwnedSCMBuffer(t *testing.T) {
	units := []uint16{'x', 'y', 0, 'z'}
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&units[0])), len(units)*2)
	if text, err := configString(buffer, &units[0]); err != nil || text != "xy" {
		t.Fatal(text, err)
	}
	other := uint16('x')
	for _, pointer := range []*uint16{nil, &other, &units[3], (*uint16)(unsafe.Pointer(&buffer[1]))} {
		if _, err := configString(buffer, pointer); err == nil {
			t.Fatal("out-of-buffer, unaligned or unterminated value accepted")
		}
	}
	if _, err := configString(nil, &units[0]); err == nil {
		t.Fatal("empty buffer accepted")
	}
}

func TestInstalledCodeTrustDoesNotAcceptOrdinaryOwnerOrServicingSIDAtLeaf(t *testing.T) {
	const installer = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	if !trustedCodeSID("S-1-5-18", false) || !trustedCodeSID("S-1-5-32-544", false) || !trustedCodeSID(installer, true) {
		t.Fatal("trusted fixed owner refused")
	}
	if trustedCodeSID(installer, false) || trustedCodeSID("S-1-5-21-111-222-333-1001", true) || checkInstalledCodeDescriptor(nil, false, true) == nil {
		t.Fatal("untrusted code identity accepted")
	}
}
