//go:build windows

package ipc

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// These tests exercise the production identity reader and session validation
// using source contracts. They never open a peer process/token, service or pipe.
type peerSourceFixture struct {
	svc                                  serviceIdentity
	svcErr, imageErr, tokenErr, aliveErr error
	image                                string
	token                                processIdentity
	pipePID                              uint32
	serviceReads, imageReads, tokenReads int
	imageHandles                         []windows.Handle
}

func newPeerSourceFixture() *peerSourceFixture {
	const image = `C:\Product\godusevpn-svc.exe`
	return &peerSourceFixture{
		svc:   serviceIdentity{exists: true, state: windows.SERVICE_RUNNING, pid: 1234, kind: windows.SERVICE_WIN32_OWN_PROCESS, image: image, account: "LocalSystem"},
		image: image, pipePID: 1234,
		token: processIdentity{sid: "S-1-5-21-1-2-3-1001", elevated: true, administrator: true},
	}
}

func (f *peerSourceFixture) reader() peerIdentityReader {
	return peerIdentityReader{
		service: func() (serviceIdentity, error) { f.serviceReads++; return f.svc, f.svcErr },
		image: func(h windows.Handle) (string, error) {
			f.imageReads++
			f.imageHandles = append(f.imageHandles, h)
			return f.image, f.imageErr
		},
		token:     func(windows.Handle) (processIdentity, error) { f.tokenReads++; return f.token, f.tokenErr },
		alive:     func(windows.Handle) error { return f.aliveErr },
		serverPID: func(net.Conn) (uint32, error) { return f.pipePID, nil },
	}
}

func fixturePeerSession(t *testing.T, f *peerSourceFixture) *windowsPeerSession {
	t.Helper()
	r := f.reader()
	const heldProcess = windows.Handle(731)
	info, foreground, err := r.identity(f.pipePID, heldProcess, nil)
	if err != nil {
		t.Fatal("fixture peer was not authenticated:", err)
	}
	return &windowsPeerSession{pid: f.pipePID, process: heldProcess, sid: info.sid, image: info.image, foreground: foreground, reader: r}
}

func TestInstalledPeerDoesNotRequireTokenQueryAtAuthenticationOrSend(t *testing.T) {
	f := newPeerSourceFixture()
	f.tokenErr = windows.ERROR_ACCESS_DENIED
	p := fixturePeerSession(t, f)
	for range 2 {
		if err := p.Validate(); err != nil {
			t.Fatal("SCM-bound installed peer depended on token access:", err)
		}
	}
	if f.tokenReads != 0 || f.serviceReads < 3 || f.imageReads < 3 {
		t.Fatal("installed identity did not use fresh SCM/image evidence without token queries")
	}
	if p.sid != "" {
		t.Fatal("SCM evidence was represented as an observed token SID")
	}
	for _, h := range f.imageHandles {
		if h != p.process {
			t.Fatal("image was not read through the held process object")
		}
	}
}

func TestInstalledSessionRejectsChangedOrUnreadableSourcesWithoutForegroundFallback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*peerSourceFixture)
	}{
		{"SCM_denied", func(f *peerSourceFixture) { f.svcErr = windows.ERROR_ACCESS_DENIED }},
		{"service_absent", func(f *peerSourceFixture) { f.svc = serviceIdentity{} }},
		{"service_stopped", func(f *peerSourceFixture) { f.svc.state, f.svc.pid = windows.SERVICE_STOPPED, 0 }},
		{"starting", func(f *peerSourceFixture) { f.svc.state = windows.SERVICE_START_PENDING }},
		{"stopping", func(f *peerSourceFixture) { f.svc.state = windows.SERVICE_STOP_PENDING }},
		{"wrong_SCM_PID", func(f *peerSourceFixture) { f.svc.pid++ }},
		{"zero_SCM_PID", func(f *peerSourceFixture) { f.svc.pid = 0 }},
		{"shared_host", func(f *peerSourceFixture) { f.svc.kind = windows.SERVICE_WIN32_SHARE_PROCESS }},
		{"other_account", func(f *peerSourceFixture) { f.svc.account = `NT AUTHORITY\LocalService` }},
		{"local_lookalike", func(f *peerSourceFixture) { f.svc.account = `.\LocalSystem` }},
		{"missing_account", func(f *peerSourceFixture) { f.svc.account = "" }},
		{"SCM_image_changed", func(f *peerSourceFixture) { f.svc.image = `C:\Other\godusevpn-svc.exe` }},
		{"process_image_changed", func(f *peerSourceFixture) { f.image = `C:\Other\godusevpn-svc.exe` }},
		{"both_images_changed", func(f *peerSourceFixture) { f.image = `C:\Other\godusevpn-svc.exe`; f.svc.image = f.image }},
		{"image_unreadable", func(f *peerSourceFixture) { f.imageErr = windows.ERROR_ACCESS_DENIED }},
		{"pipe_PID_changed", func(f *peerSourceFixture) { f.pipePID++ }},
		{"process_exited", func(f *peerSourceFixture) { f.aliveErr = errors.New("fixture process exited") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPeerSourceFixture()
			p := fixturePeerSession(t, f)
			tc.change(f)
			if err := p.Validate(); err == nil {
				t.Fatal("changed/unreadable source was accepted")
			}
			if f.tokenReads != 0 {
				t.Fatal("installed session tried a foreground token fallback")
			}
		})
	}
}

func TestForegroundPeerRetainsStrictTokenAndSCMRequirements(t *testing.T) {
	for _, state := range []string{"absent", "stopped"} {
		t.Run(state, func(t *testing.T) {
			f := newPeerSourceFixture()
			f.svc = serviceIdentity{}
			if state == "stopped" {
				f.svc = serviceIdentity{exists: true, state: windows.SERVICE_STOPPED}
			}
			p := fixturePeerSession(t, f)
			if !p.foreground || f.tokenReads != 1 {
				t.Fatal("explicit foreground mode did not query its token")
			}
			if err := p.Validate(); err != nil {
				t.Fatal("unchanged foreground identity was rejected:", err)
			}
			f.tokenErr = windows.ERROR_ACCESS_DENIED
			if err := p.Validate(); err == nil {
				t.Fatal("foreground token query failure was bypassed")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*peerSourceFixture)
	}{
		{"token_denied", func(f *peerSourceFixture) { f.tokenErr = windows.ERROR_ACCESS_DENIED }},
		{"not_elevated", func(f *peerSourceFixture) { f.token.elevated = false }},
		{"deny_only_admin", func(f *peerSourceFixture) { f.token.administrator = false }},
		{"no_SID", func(f *peerSourceFixture) { f.token.sid = "" }},
		{"SCM_denied", func(f *peerSourceFixture) { f.svcErr = windows.ERROR_ACCESS_DENIED }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPeerSourceFixture()
			f.svc = serviceIdentity{}
			tc.change(f)
			if _, _, err := f.reader().identity(f.pipePID, windows.Handle(731), nil); err == nil {
				t.Fatal("untrusted foreground peer accepted")
			}
			if f.svcErr != nil && (f.tokenReads != 0 || f.imageReads != 0) {
				t.Fatal("SCM uncertainty fell back to foreground sources")
			}
		})
	}
}

func TestForegroundSessionRejectsAccountAndModeChanges(t *testing.T) {
	for _, changeMode := range []bool{false, true} {
		f := newPeerSourceFixture()
		running := f.svc
		f.svc = serviceIdentity{}
		p := fixturePeerSession(t, f)
		if changeMode {
			f.svc = running
		} else {
			f.token.sid = "S-1-5-21-1-2-3-1002"
		}
		if err := p.Validate(); err == nil {
			t.Fatal("foreground session accepted a different account or SCM mode")
		}
		if changeMode && f.tokenReads != 1 {
			t.Fatal("mode transition queried a new token instead of rejecting")
		}
	}
}

func TestSCMConfigurationRequiresCanonicalLocalSystemAndFixedServiceCommand(t *testing.T) {
	command := `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" service`
	for _, account := range []string{"LocalSystem", "localsystem", "LOCALSYSTEM"} {
		image, err := serviceConfigurationImage(command, account)
		if err != nil || image != `C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe` {
			t.Fatal("canonical configuration not read", err)
		}
	}
	for _, account := range []string{"", `.\LocalSystem`, `MACHINE\LocalSystem`, `NT AUTHORITY\SYSTEM`, "LocalService", "LocalSystem ", "LocalSyſtem"} {
		if _, err := serviceConfigurationImage(command, account); err == nil {
			t.Fatal("noncanonical service account accepted")
		}
	}
	for _, bad := range []string{`"C:\Product\godusevpn-svc.exe"`, `"C:\Product\godusevpn-svc.exe" run`, command + " extra", `"C:\Product\godusevpn-svc.exe" SERVICE`} {
		if _, err := serviceConfigurationImage(bad, "LocalSystem"); err == nil {
			t.Fatal("unexpected service command accepted")
		}
	}
}

func TestForegroundCodeDescriptorsRejectOtherAccountReplacement(t *testing.T) {
	const admin = "S-1-5-21-1-2-3-1001"
	for _, tc := range []struct {
		name, sddl string
		dir, allow bool
	}{
		{"private_admins", "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)", false, true},
		{"serviced_volume_owner", "O:S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464G:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)", true, true},
		{"readable_by_users", "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;BU)", false, true},
		{"users_write", "O:BAG:BAD:P(A;;FA;;;BA)(A;;FW;;;BU)", false, false},
		{"other_owner", "O:BUG:BUD:P(A;;FA;;;SY)(A;;FA;;;BA)", true, false},
		{"admin_personal_code", "O:" + admin + "G:BAD:P(A;;FA;;;BA)(A;;FA;;;" + admin + ")", false, true},
		{"other_person_write", "O:BAG:BAD:P(A;;FA;;;BA)(A;;FA;;;S-1-5-21-1-2-3-1002)", true, false},
		{"directory_delete_child", "O:BAG:BAD:P(A;;FA;;;BA)(A;;0x40;;;BU)", true, false},
		{"directory_create_sibling_only", "O:BAG:BAD:P(A;;FA;;;BA)(A;;0x4;;;BU)", true, true},
		{"file_append", "O:BAG:BAD:P(A;;FA;;;BA)(A;;0x4;;;BU)", false, false},
		{"inherit_only_template", "O:BAG:BAD:P(A;;FA;;;BA)(A;OICIIO;FA;;;CO)", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tc.sddl)
			if err != nil {
				t.Fatal(err)
			}
			if got := checkCodeDescriptor(sd, admin, tc.dir) == nil; got != tc.allow {
				t.Fatalf("trusted=%v expected=%v", got, tc.allow)
			}
		})
	}
}

func TestCodePathDoesNotProbeUNCDeviceOrRelativePaths(t *testing.T) {
	for _, path := range []string{`\\example.invalid\share\godusevpn-svc.exe`, `\\?\C:\Product\godusevpn-svc.exe`, `C:godusevpn-svc.exe`, `godusevpn-svc.exe`, `C:\Product\godusevpn-svc.exe:stream`} {
		if localImagePath(path) {
			t.Fatal("unsafe code path accepted")
		}
	}
}

func TestForegroundImagePinsOnlyTemporaryCodeAndAncestors(t *testing.T) {
	// This is fixture data, never executed. Only the test's temporary path is
	// written; no service, token privileges, ACLs or product state is changed.
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal("cannot identify test fixture owner")
	}
	dir := t.TempDir()
	image := filepath.Join(dir, "godusevpn-svc.exe")
	if err = os.WriteFile(image, []byte("fixture, not an executable"), 0600); err != nil {
		t.Fatal(err)
	}
	// The host's ancestor ACLs are not a fixture. Test the real handle sharing
	// mechanism independently of ACL policy, which is tested with synthetic
	// security descriptors above. This does not certify foreground acceptance.
	held, err := pinForegroundImageWithCheck(image, image, u.User.Sid.String(), func(*windows.SECURITY_DESCRIPTOR, string, bool) error { return nil })
	if err != nil {
		t.Fatal("private temporary code could not be pinned:", err)
	}
	closed := false
	defer func() {
		if !closed {
			for _, f := range held {
				_ = f.Close()
			}
		}
	}()
	if err = os.WriteFile(image, []byte("replacement"), 0600); err == nil {
		t.Fatal("pinned code remained writable")
	}
	name, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h, e := windows.CreateFile(name, windows.DELETE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0); e == nil {
		windows.CloseHandle(h)
		t.Fatal("pinned directory still grants a delete handle")
	} else if !errors.Is(e, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("fixture could not establish a sharing exclusion")
	}
	if err = os.Rename(dir, dir+"-renamed"); err == nil {
		_ = os.Rename(dir+"-renamed", dir)
		t.Fatal("pinned ancestor could be renamed")
	}
	for _, f := range held {
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	closed = true
	if err = os.WriteFile(image, []byte("fixture cleanup after close"), 0600); err != nil {
		t.Fatal("code handle not released")
	}
}
