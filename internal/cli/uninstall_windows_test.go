package cli

import (
	"errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
	"testing"
)

func TestInstalledUIProcessNamesRemainScoped(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"godusevpn.exe", "godusevpn.exe"},
		{"GODUSEVPN.EXE", "godusevpn.exe"},
		{"GodUseVPN.Ultra.BrowserHost.exe", `browser-host\GodUseVPN.Ultra.BrowserHost.exe`},
		{"godusevpn.ultra.browserhost.exe", `browser-host\GodUseVPN.Ultra.BrowserHost.exe`},
		{"GodUseVPN.Ultra.BrowserHost.exe.bak", ""},
		{"msedgewebview2.exe", ""},
		{"godusevpn-svc.exe", ""},
		{"other.exe", ""},
	} {
		if got := installedUIRelative(tc.name); got != tc.want {
			t.Fatalf("%q: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Synthetic configuration strings only: no SCM connection or service changes.
func TestInstalledServiceConfigRequiresExactInstallationIdentity(t *testing.T) {
	const servicePath = `C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe`
	const fixedCommand = `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" service`
	for _, tc := range []struct {
		name, command, account string
		serviceType            uint32
		want                   bool
	}{
		{"fixed", fixedCommand, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, true},
		{"case_insensitive_path_and_account", `"c:\PROGRAM FILES\godusevpn ultra\GODUSEVPN-SVC.EXE" service`, "localsystem", windows.SERVICE_WIN32_OWN_PROCESS, true},
		{"quoted_service_argument", `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" "service"`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, true},
		{"no_command", "", "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"no_service_argument", `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe"`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"empty_service_argument", `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" ""`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"extra_argument", fixedCommand + " --extra", "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"extra_empty_argument", fixedCommand + ` ""`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"wrong_argument", `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" uninstall`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"argument_case_changed", `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" Service`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"unquoted_path", `C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe service`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"relative_path", `godusevpn-svc.exe service`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"different_drive", `"D:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" service`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"sibling_product", `"C:\Program Files\GodUseVPN\godusevpn-svc.exe" service`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"similar_directory", `"C:\Program Files\GodUseVPN Ultra backup\godusevpn-svc.exe" service`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"similar_binary", `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe.bak" service`, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"nul_command", fixedCommand + "\x00", "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"empty_account", fixedCommand, "", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"local_service_account", fixedCommand, `NT AUTHORITY\LocalService`, windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"network_service_account", fixedCommand, `NT AUTHORITY\NetworkService`, windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"named_admin_account", fixedCommand, `.\Administrator`, windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"similar_account", fixedCommand, "LocalSystem ", windows.SERVICE_WIN32_OWN_PROCESS, false},
		{"missing_type", fixedCommand, "LocalSystem", 0, false},
		{"shared_process", fixedCommand, "LocalSystem", windows.SERVICE_WIN32_SHARE_PROCESS, false},
		{"interactive_process", fixedCommand, "LocalSystem", windows.SERVICE_WIN32_OWN_PROCESS | windows.SERVICE_INTERACTIVE_PROCESS, false},
		{"driver", fixedCommand, "LocalSystem", windows.SERVICE_KERNEL_DRIVER, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mgr.Config{BinaryPathName: tc.command, ServiceStartName: tc.account, ServiceType: tc.serviceType}
			if got := matchesInstalledServiceConfig(cfg, servicePath); got != tc.want {
				t.Fatalf("identity match = %v, want %v", got, tc.want)
			}
		})
	}
}

// Opens only a temporary directory. No ACL, installed path or service changes.
func TestPinnedDirectoryRejectsDeleteHandleUntilClose(t *testing.T) {
	path := t.TempDir()
	h, err := openPinnedCode(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	name, _ := windows.UTF16PtrFromString(path)
	openDelete := func() (windows.Handle, error) {
		return windows.CreateFile(name, windows.DELETE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	}
	other, err := openDelete()
	if err == nil {
		windows.CloseHandle(other)
		t.Fatal("directory pin did not block DELETE access")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal(err)
	}
	if err = windows.CloseHandle(h); err != nil {
		t.Fatal(err)
	}
	h = 0
	other, err = openDelete()
	if err != nil {
		t.Fatal("temporary directory DELETE open failed after release", err)
	}
	windows.CloseHandle(other)
}

func TestUninstallCodeTrustUsesSyntheticDescriptors(t *testing.T) {
	for _, tc := range []struct {
		s                       string
		ancestor, protected, ok bool
	}{{"O:BAG:BAD:P(A;;GA;;;BA)(A;;GA;;;SY)(A;;GRGX;;;BU)", false, true, true}, {"O:BUG:BUD:P(A;;GA;;;BA)", false, true, false}, {"O:BAG:BAD:P(A;;GA;;;BA)(A;;GW;;;BU)", false, true, false}, {"O:BAG:BAD:P(A;;GA;;;BA)(A;OIIO;GW;;;BU)", false, true, false}, {"O:BAG:BAD:P(A;;GA;;;BA)(A;OICIIO;GA;;;CO)", true, false, true}} {
		sd, e := windows.SecurityDescriptorFromString(tc.s)
		if e != nil {
			t.Fatal(e)
		}
		if (verifyCodeDescriptor(sd, tc.ancestor, tc.protected) == nil) != tc.ok {
			t.Fatal("unexpected code trust")
		}
	}
}
