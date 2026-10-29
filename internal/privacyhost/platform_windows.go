//go:build windows && (amd64 || arm64)

package privacyhost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	prep "github.com/Maoyangui/godusevpn/internal/locationportalprepare"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"github.com/Maoyangui/godusevpn/internal/privacy"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const pipePrefix = `\\.\pipe\GodUseVPNUltra-Privacy-`

// IsElevated checks the actual process token without acquiring privileges or
// launching a helper. It is a capability check, not an authorization claim.
func IsElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// ExecuteElevated is for an explicitly elevated CLI/uninstaller. It never
// requests elevation itself, never runs a browser, and accepts no storage path.
// Ordinary UI and ordinary CLI callers must use the authenticated helper pipe.
func ExecuteElevated(method string, params, out any) error {
	return ExecuteElevatedWithLease(nil, method, params, out)
}
func ExecuteElevatedWithLease(existing *maintenance.Lease, method string, params, out any) error {
	if !allowedMethods[method] || method == "MaintenanceReady" || browserGuardMethod(method) || !IsElevated() {
		return errors.New("需要已提升令牌和固定机器隐私操作")
	}
	l, release, err := maintenance.Use(existing)
	if err != nil {
		return err
	}
	defer release()
	return executeElevated(method, params, out, IsElevated, func() (dispatcher, error) {
		dir, err := machineDirectory(l)
		if err != nil {
			return dispatcher{}, err
		}
		return dispatcher{lease: l, gate: maintenance.Acquire, policy: privacy.NewMachine(dir), firewall: appfirewall.New(dir)}, nil
	})
}

type helperConfig struct {
	maintenanceOnly    bool
	pipe, session, sid string
	parent             uint32
}

func randomHex() (string, error) {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(b[:]), nil
}
func tokenSID(token windows.Token) (string, error) {
	u, e := token.GetTokenUser()
	if e != nil {
		return "", e
	}
	s := u.User.Sid.String()
	if s == "S-1-5-18" || s == "S-1-5-19" || s == "S-1-5-20" {
		return "", errors.New("privacy helper requires an interactive UI user")
	}
	return s, nil
}
func validConfig(c helperConfig) bool {
	if c.parent == 0 || len(c.session) != 64 || !strings.HasPrefix(c.pipe, pipePrefix) || len(c.pipe) != len(pipePrefix)+64 {
		return false
	}
	if _, e := hex.DecodeString(c.session); e != nil {
		return false
	}
	if _, e := hex.DecodeString(strings.TrimPrefix(c.pipe, pipePrefix)); e != nil {
		return false
	}
	sid, e := windows.StringToSid(c.sid)
	return e == nil && sid.String() == c.sid && c.sid != "S-1-5-18" && c.sid != "S-1-5-19" && c.sid != "S-1-5-20"
}

func parseConfig(args []string) (helperConfig, error) {
	var c helperConfig
	if (len(args) != 9 && len(args) != 11) || args[0] != helperFlag {
		return c, errors.New("invalid helper arguments")
	}
	seen := map[string]bool{}
	for i := 1; i < len(args); i += 2 {
		if seen[args[i]] {
			return c, errors.New("duplicate helper argument")
		}
		seen[args[i]] = true
		switch args[i] {
		case "--mode":
			if args[i+1] != "maintenance" {
				return c, errors.New("unknown fixed helper mode")
			}
			c.maintenanceOnly = true
		case "--pipe":
			c.pipe = args[i+1]
		case "--session":
			c.session = args[i+1]
		case "--sid":
			c.sid = args[i+1]
		case "--parent":
			n, e := strconv.ParseUint(args[i+1], 10, 32)
			if e != nil {
				return c, e
			}
			c.parent = uint32(n)
		default:
			return c, errors.New("unknown helper argument")
		}
	}
	if !validConfig(c) {
		return c, errors.New("invalid helper identity")
	}
	return c, nil
}

// Start creates the pipe as the ordinary UI, then asks Windows to elevate only
// a second copy of the current executable in the headless fixed helper mode.
// Call on a UI worker goroutine: the UAC consent dialog is controlled by Windows.
func Start(ctx context.Context, exe string) (*Client, error) {
	return start(ctx, exe, false)
}

// StartMaintenance authenticates an elevated gate-only child. It cannot open a
// machine store, invoke WFP, or read/modify a policy. The original CLI stays ordinary.
func StartMaintenance(ctx context.Context, exe string) (*Client, error) { return start(ctx, exe, true) }
func start(ctx context.Context, exe string, maintenanceOnly bool) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if !sameProductExecutable(self, exe) {
		return nil, errors.New("helper must use this product executable or its fixed GUI/CLI sibling")
	}
	sid, err := tokenSID(windows.GetCurrentProcessToken())
	if err != nil {
		return nil, err
	}
	pipeID, err := randomHex()
	if err != nil {
		return nil, err
	}
	session, err := randomHex()
	if err != nil {
		return nil, err
	}
	cfg := helperConfig{maintenanceOnly: maintenanceOnly, pipe: pipePrefix + pipeID, session: session, parent: uint32(os.Getpid()), sid: sid}
	listener, err := winio.ListenPipe(cfg.pipe, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + sid + ")", InputBufferSize: 4096, OutputBufferSize: 4096})
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	h, err := launchElevated(exe, cfg)
	if err != nil {
		return nil, fmt.Errorf("仅隐私 helper 未获授权；VPN 和普通界面可继续使用: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			windows.CloseHandle(h)
		}
	}()
	pid, err := windows.GetProcessId(h)
	if err != nil {
		return nil, err
	}
	acceptCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	stop := context.AfterFunc(acceptCtx, func() { listener.Close() })
	defer stop()
	var conn net.Conn
	for {
		conn, err = listener.Accept()
		if err != nil {
			return nil, fmt.Errorf("helper did not connect; no privacy operation was sent: %w", err)
		}
		actual, e := clientPID(conn)
		if e != nil || actual != pid {
			conn.Close()
			continue
		}
		break
	}
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}
	var greeting hello
	if err = readFrame(conn, &greeting, 4096); err != nil {
		conn.Close()
		return nil, err
	}
	if greeting.Version != protocolVersion || greeting.PID != pid || greeting.Parent != cfg.parent || !sameSession(greeting.Session, cfg.session) {
		conn.Close()
		return nil, errors.New("helper handshake identity mismatch")
	}
	if err = writeFrame(conn, hello{Version: protocolVersion, Session: session, PID: pid, Parent: cfg.parent}, 4096); err != nil {
		conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	c := newClient(conn, session, func() { windows.CloseHandle(h) })
	if maintenanceOnly {
		c.heartbeat = "MaintenanceReady"
	}
	keep = true
	go c.watch(ctx)
	return c, nil
}

// Handle must run before Wails/WebView, services, protocol handlers or browser
// startup. It never interprets ordinary application arguments or launches URLs.
func Handle(args []string) (bool, int) {
	if len(args) == 0 || args[0] != helperFlag {
		return false, 0
	}
	cfg, err := parseConfig(args)
	if err != nil {
		return true, 2
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return true, 3
	}
	if err = runHelper(cfg); err != nil {
		return true, 1
	}
	return true, 0
}

func runHelper(cfg helperConfig) error {
	parent, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, cfg.parent)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	var token windows.Token
	if err = windows.OpenProcessToken(parent, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	sid, err := tokenSID(token)
	token.Close()
	if err != nil || sid != cfg.sid {
		return errors.New("parent token does not match originating user")
	}
	name := make([]uint16, 32768)
	n := uint32(len(name))
	if err = windows.QueryFullProcessImageName(parent, 0, &name[0], &n); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !sameProductExecutable(self, windows.UTF16ToString(name[:n])) {
		return errors.New("parent is not this product executable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	conn, err := winio.DialPipeContext(ctx, cfg.pipe)
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close()
	validPeer := func() error {
		p, e := serverPID(conn)
		if e != nil || p != cfg.parent {
			return errors.New("pipe server is not the originating UI")
		}
		event, e := windows.WaitForSingleObject(parent, 0)
		if e != nil || event != uint32(windows.WAIT_TIMEOUT) {
			return errors.New("originating UI has exited")
		}
		return nil
	}
	if err = validPeer(); err != nil {
		return err
	}
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	greeting := hello{Version: protocolVersion, Session: cfg.session, PID: uint32(os.Getpid()), Parent: cfg.parent}
	if err = writeFrame(conn, greeting, 4096); err != nil {
		return err
	}
	var ack hello
	if err = readFrame(conn, &ack, 4096); err != nil {
		return err
	}
	if ack != greeting {
		return errors.New("parent handshake mismatch")
	}
	if err = validPeer(); err != nil {
		return err
	}
	if err = maintenance.Prepare(); err != nil {
		return err
	}
	d := dispatcher{maintenanceOnly: cfg.maintenanceOnly, gate: maintenance.Acquire}
	if !cfg.maintenanceOnly {
		lease, e := maintenance.Acquire()
		if e != nil {
			return e
		}
		dir, e := machineDirectory(lease)
		_ = lease.Close()
		if e != nil {
			return e
		}
		d.policy, d.firewall = privacy.NewMachine(dir), appfirewall.New(dir)
		// The held, already authenticated originating GUI handle is authority.
		// Never use this elevated process's HKCU/LocalAppData: UAC may have used
		// a different administrator account. A failed browser guard factory
		// disables browser-only actions and leaves the VPN/privacy helper usable.
		browser, browserErr := browserengineguard.New(parent)
		if browserErr == nil && browser != nil {
			d.browser = browser
			defer browser.Close()
		} else {
			d.browserDiagnostic = browserengineguard.UnavailableCapability(browserErr).Diagnostic
		}
		// Preparation derives all system authority from the same authenticated
		// originating GUI SID. A missing/foreign machine store or SCM/gate proof
		// leaves this optional action unavailable; it never becomes a report-only
		// success path.
		if portal, portalErr := prep.NewNativeManager(cfg.sid); portalErr == nil {
			d.portalPreparation = portal
			defer portal.Close()
		}
	}
	// Parent death closes the pipe even while idle; serve's idle deadline is a
	// second bound when the UI is hung or its heartbeat has stopped.
	watchCtx, stopWatch := context.WithCancel(context.Background())
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				if validPeer() != nil {
					conn.Close()
					return
				}
			}
		}
	}()
	defer func() { stopWatch(); <-watchDone }()
	return serve(conn, cfg.session, d, validPeer)
}

// A normal CLI can launch only the fixed GUI sibling as its headless helper.
// The two paths must resolve to the same directory, with fixed product names.
// This is also checked in the elevated child against the actual parent image.
func sameProductExecutable(a, b string) bool {
	if !filepath.IsAbs(a) || !filepath.IsAbs(b) {
		return false
	}
	aInfo, err := os.Stat(a)
	if err != nil || !aInfo.Mode().IsRegular() {
		return false
	}
	bInfo, err := os.Stat(b)
	if err != nil || !bInfo.Mode().IsRegular() {
		return false
	}
	if os.SameFile(aInfo, bInfo) {
		return true
	}
	ca, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	cb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	if !strings.EqualFold(filepath.Dir(ca), filepath.Dir(cb)) {
		return false
	}
	an, bn := strings.ToLower(filepath.Base(ca)), strings.ToLower(filepath.Base(cb))
	return an == "godusevpn.exe" && bn == "godusevpn-cli.exe" || an == "godusevpn-cli.exe" && bn == "godusevpn.exe"
}

func pipeHandle(conn net.Conn) (windows.Handle, error) {
	f, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return 0, errors.New("named pipe handle unavailable")
	}
	return windows.Handle(f.Fd()), nil
}

var getClientPID = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetNamedPipeClientProcessId")

func clientPID(conn net.Conn) (uint32, error) {
	h, err := pipeHandle(conn)
	if err != nil {
		return 0, err
	}
	var pid uint32
	ok, _, e := getClientPID.Call(uintptr(h), uintptr(unsafe.Pointer(&pid)))
	if ok == 0 {
		return 0, e
	}
	return pid, nil
}
func serverPID(conn net.Conn) (uint32, error) {
	h, err := pipeHandle(conn)
	if err != nil {
		return 0, err
	}
	var pid uint32
	err = windows.GetNamedPipeServerProcessId(h, &pid)
	return pid, err
}

// SHELLEXECUTEINFOW on 64-bit Windows. No handles are inherited; SW_HIDE keeps
// the helper headless. Only Windows' own UAC consent UI may be shown.
type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            windows.Handle
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          windows.Handle
	IDList                            unsafe.Pointer
	Class                             *uint16
	ClassKey                          windows.Handle
	HotKey                            uint32
	Icon                              windows.Handle
	Process                           windows.Handle
}

var shellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

func launchElevated(exe string, c helperConfig) (windows.Handle, error) {
	args := []string{helperFlag, "--pipe", c.pipe, "--session", c.session, "--parent", strconv.FormatUint(uint64(c.parent), 10), "--sid", c.sid}
	if c.maintenanceOnly {
		args = append(args, "--mode", "maintenance")
	}
	for i := range args {
		args[i] = syscall.EscapeArg(args[i])
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{Mask: 0x40 | 0x100 | 0x400, Verb: verb, File: file, Parameters: params, Show: windows.SW_HIDE}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, err := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 0, err
	}
	if info.Process == 0 {
		return 0, errors.New("Windows did not return the helper process handle")
	}
	return info.Process, nil
}
