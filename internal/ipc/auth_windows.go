//go:build windows

package ipc

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/identity"
	"golang.org/x/sys/windows"
)

type serviceIdentity struct {
	exists           bool
	state, pid, kind uint32
	image, account   string
}

type processIdentity struct {
	sid                     string
	elevated, administrator bool
	image                   string
}

// SCM identifies an installed service without requiring an ordinary client to
// open another account's primary token. Token queries remain mandatory for the
// separately authenticated, explicitly elevated foreground mode.
type peerIdentityReader struct {
	service   func() (serviceIdentity, error)
	image     func(windows.Handle) (string, error)
	token     func(windows.Handle) (processIdentity, error)
	alive     func(windows.Handle) error
	serverPID func(net.Conn) (uint32, error)
}

func windowsPeerReader() peerIdentityReader {
	return peerIdentityReader{
		service: readPeerService, image: readPeerImage, token: readPeerToken,
		alive: peerAlive, serverPID: pipeServerPID,
	}
}

func (r peerIdentityReader) identity(pid uint32, h windows.Handle, expectedForeground *bool) (processIdentity, bool, error) {
	var out processIdentity
	svc, err := r.service()
	if err != nil {
		return out, false, err
	}
	foreground, err := servicePeerMode(pid, svc)
	if err != nil {
		return out, false, err
	}
	if expectedForeground != nil && foreground != *expectedForeground {
		return out, false, errors.New("service instance mode changed")
	}
	image, err := r.image(h)
	if err != nil {
		return out, false, err
	}
	if !foreground {
		if !sameLocalImage(image, svc.image) {
			return out, false, errors.New("SCM process image differs from service configuration")
		}
		// This is SCM account/configuration evidence, not a queried token SID.
		return processIdentity{image: image}, false, nil
	}
	out, err = r.token(h)
	if err != nil {
		return processIdentity{}, false, err
	}
	if out.sid == "" || !out.elevated || (out.sid != "S-1-5-18" && !out.administrator) {
		return processIdentity{}, false, errors.New("server does not have an elevated administrator token")
	}
	out.image = image
	return out, true, nil
}

// Holding the process object prevents PID reuse while this connection exists.
// Every Validate also rechecks the pipe and the current SCM instance; access
// denied is an authentication failure, never an automatic foreground fallback.
type windowsPeerSession struct {
	mu         sync.Mutex
	conn       net.Conn
	pid        uint32
	sid, image string
	process    windows.Handle
	files      []*os.File
	foreground bool
	closed     bool
	reader     peerIdentityReader
}

func authenticateWindowsPeer(conn net.Conn) (peerSession, error) {
	pid, err := pipeServerPID(conn)
	if err != nil || pid == 0 {
		return nil, errors.New("pipe identity unavailable")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, errors.New("process identity unavailable")
	}
	p := &windowsPeerSession{conn: conn, pid: pid, process: h, reader: windowsPeerReader()}
	ok := false
	defer func() {
		if !ok {
			_ = p.Close()
		}
	}()
	info, foreground, err := p.reader.identity(pid, h, nil)
	if err != nil {
		return nil, err
	}
	p.sid, p.image = info.sid, info.image
	p.foreground = foreground
	if foreground {
		self, err := os.Executable()
		if err != nil {
			return nil, errors.New("client image unavailable")
		}
		expected := filepath.Join(filepath.Dir(self), "godusevpn-svc.exe")
		// A foreground process must be the fixed service sibling, on a local
		// drive, with no other account able to replace its code or ancestors.
		p.files, err = pinForegroundImage(expected, info.image, info.sid)
		if err != nil {
			return nil, err
		}
	}
	ok = true
	return p, nil
}

func (p *windowsPeerSession) Validate() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return net.ErrClosed
	}
	if err := p.reader.alive(p.process); err != nil {
		return err
	}
	pid, err := p.reader.serverPID(p.conn)
	if err != nil || pid != p.pid {
		return errors.New("pipe process changed")
	}
	info, foreground, err := p.reader.identity(p.pid, p.process, &p.foreground)
	if err != nil {
		return err
	}
	if info.sid != p.sid || !sameLocalImage(info.image, p.image) {
		return errors.New("held process identity changed")
	}
	if foreground != p.foreground {
		return errors.New("service instance changed")
	}
	// A successful process query may race with exit; no bytes have been sent.
	return p.reader.alive(p.process)
}

func (p *windowsPeerSession) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	for _, f := range p.files {
		_ = f.Close()
	}
	return windows.CloseHandle(p.process)
}

func peerAlive(h windows.Handle) error {
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("service process exited or is unreadable")
	}
	return nil
}

func pipeServerPID(conn net.Conn) (uint32, error) {
	f, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return 0, errors.New("pipe handle unavailable")
	}
	var pid uint32
	err := windows.GetNamedPipeServerProcessId(windows.Handle(f.Fd()), &pid)
	return pid, err
}

// Only a definitely absent/stopped SCM service may use the explicit elevated
// foreground run path. A starting, stopping, unreadable or different live SCM
// instance cannot be bypassed by a foreground process.
func servicePeerMode(pid uint32, s serviceIdentity) (bool, error) {
	if pid == 0 {
		return false, errors.New("pipe process identity unavailable")
	}
	if !s.exists || (s.state == windows.SERVICE_STOPPED && s.pid == 0) {
		return true, nil
	}
	if s.state != windows.SERVICE_RUNNING || s.pid == 0 || s.pid != pid || s.kind != windows.SERVICE_WIN32_OWN_PROCESS || !canonicalLocalSystem(s.account) {
		return false, errors.New("server is not the current SCM LocalSystem service instance")
	}
	if s.image == "" {
		return false, errors.New("service image unavailable")
	}
	return false, nil
}

func readPeerToken(h windows.Handle) (processIdentity, error) {
	var p processIdentity
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return p, errors.New("server token cannot be queried")
	}
	defer token.Close()
	u, err := token.GetTokenUser()
	if err != nil {
		return p, errors.New("server token user unavailable")
	}
	p.sid = u.User.Sid.String()
	var elevated uint32
	var used uint32
	if err = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), uint32(unsafe.Sizeof(elevated)), &used); err != nil || used != 4 {
		return p, errors.New("server elevation unavailable")
	}
	p.elevated = elevated != 0
	groups, err := token.GetTokenGroups()
	if err != nil {
		return p, errors.New("server groups unavailable")
	}
	for _, group := range groups.AllGroups() {
		if group.Sid.String() == "S-1-5-32-544" && group.Attributes&windows.SE_GROUP_ENABLED != 0 && group.Attributes&windows.SE_GROUP_USE_FOR_DENY_ONLY == 0 {
			p.administrator = true
		}
	}
	return p, nil
}

func readPeerImage(h windows.Handle) (string, error) {
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil || n == 0 {
		return "", errors.New("server image unavailable")
	}
	return windows.UTF16ToString(buf[:n]), nil
}

func serviceConfigurationImage(command, account string) (string, error) {
	// Only SCM's canonical name is accepted. Do not guess whether a local or
	// domain-qualified lookalike account name denotes the SYSTEM SID.
	if !canonicalLocalSystem(account) {
		return "", errors.New("service configuration is not canonical LocalSystem")
	}
	args, err := windows.DecomposeCommandLine(command)
	if err != nil || len(args) != 2 || args[1] != "service" {
		return "", errors.New("service command is not the fixed product service command")
	}
	return args[0], nil
}

func canonicalLocalSystem(account string) bool {
	const canonical = "LocalSystem"
	// SCM's canonical name is ASCII. The length check also excludes Unicode
	// case-fold lookalikes rather than treating them as account aliases.
	return len(account) == len(canonical) && strings.EqualFold(account, canonical)
}

func readPeerService() (serviceIdentity, error) {
	var out serviceIdentity
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return out, errors.New("SCM identity unavailable")
	}
	defer windows.CloseServiceHandle(m)
	name, _ := windows.UTF16PtrFromString(identity.AppName)
	s, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return out, nil
	}
	if err != nil {
		return out, errors.New("service identity cannot be queried")
	}
	defer windows.CloseServiceHandle(s)
	var status windows.SERVICE_STATUS_PROCESS
	var used uint32
	if err = windows.QueryServiceStatusEx(s, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &used); err != nil {
		return out, errors.New("service instance unavailable")
	}
	out.exists, out.state, out.pid, out.kind = true, status.CurrentState, status.ProcessId, status.ServiceType
	// A stopped service need not have readable/valid code to authenticate an
	// independently validated foreground instance. Query failures above still
	// fail closed instead of being mistaken for an absent service.
	if out.state == windows.SERVICE_STOPPED && out.pid == 0 {
		return out, nil
	}
	buf := make([]byte, 4096)
	for {
		cfg := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
		err = windows.QueryServiceConfig(s, cfg, uint32(len(buf)), &used)
		if err == nil {
			out.account = windows.UTF16PtrToString(cfg.ServiceStartName)
			out.image, err = serviceConfigurationImage(windows.UTF16PtrToString(cfg.BinaryPathName), out.account)
			if err != nil {
				return out, err
			}
			runtime.KeepAlive(buf)
			return out, nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || used <= uint32(len(buf)) || used > 65536 {
			return out, errors.New("service configuration unavailable")
		}
		buf = make([]byte, used)
	}
}

func localImagePath(path string) bool {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if !filepath.IsAbs(clean) || len(volume) != 2 || volume[1] != ':' || strings.Contains(clean[2:], ":") {
		return false
	}
	root, err := windows.UTF16PtrFromString(volume + `\`)
	if err != nil {
		return false
	}
	kind := windows.GetDriveType(root)
	return kind == windows.DRIVE_FIXED || kind == windows.DRIVE_REMOVABLE
}

// SCM is the installed-service trust authority. Matching textual paths only
// corroborates its current process binding; a basename alone never authenticates
// it. Installation-tree ownership/ACL validation belongs to the installer, not
// to a new claim that this branch pins or verifies the installed code's bytes.
func sameLocalImage(a, b string) bool {
	return localImagePath(a) && localImagePath(b) && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
