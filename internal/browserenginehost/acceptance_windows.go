//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
	"golang.org/x/sys/windows"
)

func ordinaryProcess() error {
	token := windows.GetCurrentProcessToken()
	account, err := token.GetTokenUser()
	if err != nil || account.User.Sid == nil {
		return ErrRefused
	}
	switch account.User.Sid.String() {
	case "S-1-5-18", "S-1-5-19", "S-1-5-20":
		return ErrRefused
	}
	var elevated, used uint32
	if windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &used) != nil || used != 4 || elevated != 0 {
		return ErrRefused
	}
	return nil
}

func startAcceptance(parent context.Context, engines *browserengine.Manager, c Config, guard Guard) (sessionResult *Session, result error) {
	c = copyHardwareConfig(c)
	if parent == nil || parent.Err() != nil || engines == nil || guard == nil || validateConfig(c) != nil || ordinaryProcess() != nil {
		return nil, ErrRefused
	}
	ctx, cancel := context.WithCancel(parent)
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		cancel()
		return nil, ErrRefused
	}
	id := hex.EncodeToString(nonce[:])
	session := &Session{id: id, done: make(chan struct{}), status: Status{SessionID: id, EngineID: c.EngineID, Version: c.Version, State: "starting", Detail: "正在核验官方执行实例、代理、进程树及配置；尚未允许导航。"}}
	life := &lifetime{session: session, cancel: cancel, guard: guard}
	session.close = life.close
	complete := false
	defer func() {
		if !complete {
			if retainFailedStartup(session) {
				sessionResult = session
			}
			result = startupErrorWithDiagnostic(result, session)
		}
	}()
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	backend, err := browserproxy.StartRelay(ctx, c.Proxy)
	if err != nil {
		return nil, ErrRefused
	}
	life.backend = backend
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	var authorizer *processAuthorizer
	frontend, err := startFrontend(ctx, backend.Port(), backend.Credentials(), func(connection net.Conn) (io.Closer, error) {
		if authorizer == nil {
			return nil, ErrRefused
		}
		return authorizer.authorize(connection)
	}, ownBackend, life.revoke)
	if err != nil {
		return nil, ErrRefused
	}
	life.frontend = frontend
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	profile, err := createProfile(engines, id, c, frontend.Port())
	if err != nil {
		return nil, ErrRefused
	}
	life.profile = profile
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	code, err := engines.AcquireCodeLease(ctx, c.EngineID, c.Version, id)
	if err != nil {
		return nil, ErrRefused
	}
	life.code = code
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	var control net.Listener
	controlPort := 0
	if c.EngineID == browserengine.Firefox {
		control, err = net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, ErrRefused
		}
		defer control.Close()
		controlPort = control.Addr().(*net.TCPAddr).Port
		if controlPort == frontend.Port() || controlPort == backend.Port() {
			return nil, ErrRefused
		}
	}
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	process, err := launchSuspended(c, code.MainPath(), profile.directory, frontend.Port(), controlPort)
	if process != nil {
		life.process = process
	}
	if err != nil {
		return nil, ErrRefused
	}
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	authorizer, err = newProcessAuthorizer(process, code.Programs())
	if err != nil {
		return nil, ErrRefused
	}
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	request := browserengineguard.InstallRequest{SessionID: id, EngineID: c.EngineID, Version: c.Version, JobHandle: uint64(process.job), MainPID: process.pid, MainCreated: process.created, RelayPort: uint16(frontend.Port()), ControlPort: uint16(controlPort)}
	life.guardAttempted = true
	guardResult, err := guard.Install(ctx, request)
	if !validInstalledGuard(guardResult, id, err) || ctx.Err() != nil {
		life.recordInstallResponse(guardResult, err)
		return nil, ErrRefused
	}
	if control != nil {
		control.Close()
	}
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	if process.resume() != nil {
		return nil, ErrRefused
	}
	if startupCheckpoint(ctx) != nil {
		return nil, ErrRefused
	}
	launch := protocolLaunch{RelayPort: frontend.Port(), ProfileDirectory: profile.directory, Arguments: append([]string(nil), process.arguments...)}
	var driver protocolDriver
	if c.EngineID == browserengine.Chrome {
		transport := &nulTransport{reader: bufio.NewReader(process.read), writer: process.write, closer: func() error { process.write.Close(); return process.read.Close() }}
		driver = newChromeDriver(transport, c, launch, life.revoke)
	} else {
		transport, err := connectBiDi(ctx, profile.directory, controlPort, authorizer)
		if err != nil {
			return nil, ErrRefused
		}
		driver = newFirefoxDriver(transport, c, launch, life.revoke)
	}
	life.driver = driver
	if startupCheckpoint(ctx) != nil || driver == nil {
		return nil, ErrRefused
	}
	if configureErr := driver.Configure(ctx); configureErr != nil {
		if errors.Is(configureErr, ErrGraphicsReadback) {
			return nil, ErrGraphicsReadback
		}
		return nil, ErrRefused
	}
	if ctx.Err() != nil {
		return nil, ErrRefused
	}
	var graphicsEvidence *GraphicsEvidence
	if graphicsRequested(c) {
		if reader, ok := driver.(interface{ GraphicsEvidence() *GraphicsEvidence }); ok {
			graphicsEvidence = reader.GraphicsEvidence()
		}
		if graphicsEvidence == nil {
			return nil, ErrGraphicsReadback
		}
	}
	guardResult, err = guard.Report(ctx, id)
	if !validInstalledGuard(guardResult, id, err) || startupCheckpoint(ctx) != nil {
		life.rememberGuardDiagnostic(guardResult, err)
		return nil, ErrRefused
	}
	if frontend.Open() != nil {
		return nil, ErrRefused
	}
	var navigation sync.Mutex
	session.navigate = func(callContext context.Context, raw string) error {
		if callContext == nil || callContext.Err() != nil || ctx.Err() != nil || !allowedNavigation(raw) {
			return ErrRefused
		}
		navigation.Lock()
		defer navigation.Unlock()
		status := session.Status()
		if !status.Ready || !status.CanNavigate {
			return ErrRefused
		}
		report, err := guard.Report(callContext, id)
		if !validInstalledGuard(report, id, err) {
			life.rememberGuardDiagnostic(report, err)
			life.revoke()
			return ErrRefused
		}
		if driver.Navigate(callContext, raw) != nil {
			life.revoke()
			return ErrRefused
		}
		return nil
	}
	session.mu.Lock()
	if ctx.Err() != nil || session.status.State != "starting" {
		session.mu.Unlock()
		return nil, ErrRefused
	}
	session.status.State = "ready"
	session.status.Ready = true
	session.status.CanNavigate = true
	session.status.Graphics = graphicsEvidence
	session.status.Detail = "本次官方引擎、独立代理及所选UA/网页环境已读回；属于受控VM验收会话。环境模拟不改变真实OS/内核/硬件，未声明GPU匿名或浏览器外OS应用共享隔离。"
	session.mu.Unlock()
	complete = true
	go watchSession(ctx, life, process, guard)
	return session, nil
}

func watchSession(ctx context.Context, life *lifetime, process *nativeProcess, guard Guard) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = life.session.Close()
			return
		case <-ticker.C:
			var exit uint32
			if windows.GetExitCodeProcess(process.process, &exit) != nil || exit != 259 {
				_ = life.session.Close()
				return
			}
			count, err := process.active()
			if err != nil || count == 0 {
				_ = life.session.Close()
				return
			}
			check, cancel := context.WithTimeout(ctx, 3*time.Second)
			report, err := guard.Report(check, life.session.id)
			cancel()
			if !validInstalledGuard(report, life.session.id, err) {
				life.rememberGuardDiagnostic(report, err)
				_ = life.session.Close()
				return
			}
		}
	}
}
