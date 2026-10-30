//go:build windows

package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
	"github.com/Maoyangui/godusevpn/internal/ipc"
	"github.com/Maoyangui/godusevpn/internal/settings"
	"github.com/Maoyangui/godusevpn/internal/state"
	"golang.org/x/sys/windows"
)

type browserFixture struct {
	mu         sync.Mutex
	view       ipc.InstalledState
	readErr    error
	response   string
	noReady    bool
	reads      atomic.Int32
	starts     atomic.Int32
	releases   atomic.Int32
	ticks      chan time.Time
	frames     chan []byte
	leases     chan struct{}
	exited     chan struct{}
	controller *browserController
}

func strictBrowserFixtureState() ipc.InstalledState {
	return ipc.InstalledState{ServicePID: 4242, View: ipc.StateView{Protocol: ipc.Version, State: state.Snapshot{Status: state.Connected, Wanted: true}, Mode: "global", Guard: "on", Settings: settings.Settings{Mode: "global", TUN: true, StrictRoute: true, NoDirect: true, MixedPort: 31809}}}
}

func newBrowserFixture(t *testing.T) *browserFixture {
	t.Helper()
	f := &browserFixture{view: strictBrowserFixtureState(), ticks: make(chan time.Time, 8), frames: make(chan []byte, 8), leases: make(chan struct{}, 8), exited: make(chan struct{}), response: `{"type":"ready","schemaVersion":1}`}
	deps := browserDependencies{
		runtime: func() browserhost.RuntimeContext {
			return browserhost.RuntimeContext{Engine: browserhost.Engine{ID: "webview", Available: true, Version: "149.0.1234.56"}}
		},
		inspect: func() (browserHostPlan, error) {
			return browserHostPlan{path: "fixed-fixture-only", release: func() { f.releases.Add(1) }}, nil
		},
		state: func(context.Context) (ipc.InstalledState, error) {
			f.reads.Add(1)
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.view, f.readErr
		},
		ticks: func() (<-chan time.Time, func()) { return f.ticks, func() {} },
		start: func(context.Context, string) (*browserHostProcess, error) {
			f.starts.Add(1)
			inputReader, inputWriter := io.Pipe()
			outputReader, outputWriter := io.Pipe()
			go func() {
				defer close(f.exited)
				defer inputReader.Close()
				defer outputWriter.Close()
				scanner := bufio.NewScanner(inputReader)
				if !scanner.Scan() {
					return
				}
				f.frames <- append([]byte(nil), scanner.Bytes()...)
				if !f.noReady {
					if _, err := io.WriteString(outputWriter, f.response+"\n"); err != nil {
						return
					}
				}
				for scanner.Scan() {
					if string(scanner.Bytes()) == `{"type":"lease"}` {
						f.leases <- struct{}{}
					} else {
						return
					}
				}
			}()
			return &browserHostProcess{stdin: inputWriter, stdout: outputReader, wait: func() error { <-f.exited; return nil }, kill: func() error { return inputReader.Close() }}, nil
		},
	}
	f.controller = newBrowserController(context.Background(), deps)
	t.Cleanup(func() {
		f.controller.close(true)
		if f.starts.Load() > 0 {
			select {
			case <-f.exited:
			case <-time.After(time.Second):
				t.Error("fake host was not closed")
			}
		}
	})
	return f
}

func browserFixtureJSON(t *testing.T, f *browserFixture) string {
	t.Helper()
	d, err := f.controller.defaults("fixture-profile")
	if err != nil {
		t.Fatal(err)
	}
	return string(d.Config)
}

func waitBrowserClosed(t *testing.T, f *browserFixture) {
	t.Helper()
	select {
	case <-f.exited:
	case <-time.After(time.Second):
		t.Fatal("stdin EOF did not close fake host")
	}
}

func TestBrowserDefaultsAndSeedPreviewAreReadOnlyAndRedactBinding(t *testing.T) {
	f := newBrowserFixture(t)
	data := browserFixtureJSON(t, f)
	preview, err := f.controller.preview(data, "seed_42")
	if err != nil {
		t.Fatal(err)
	}
	if f.starts.Load() != 0 || f.reads.Load() != 0 || preview.Applied || (!browserenginehost.NativeLaunchEnabled() && preview.CanLaunch) || !preview.NeedsRuntimeReadback || !preview.HostAvailable {
		t.Fatalf("preview crossed launch/trust boundary: %+v", preview)
	}
	for _, secret := range []string{"binding", "servicePid", "proxyPort", "connectedStrictVerified", "31809", "4242"} {
		if strings.Contains(string(preview.Config), secret) {
			t.Fatal("UI received internal binding/trust state:", secret)
		}
	}
	second, err := f.controller.preview(data, "seed_42")
	if err != nil || string(second.Config) != string(preview.Config) {
		t.Fatal("fixed seed preview not repeatable", err)
	}
	var input map[string]any
	if err := json.Unmarshal(preview.Config, &input); err != nil {
		t.Fatal(err)
	}
	if input["webrtc"] != "disable-non-proxied-udp" || input["permissions"].(map[string]any)["camera"] != "block" {
		t.Fatal("randomization weakened host-required protection")
	}
}

func TestBrowserRejectsFrontendTrustAndUnsupportedHostParameters(t *testing.T) {
	f := newBrowserFixture(t)
	data := browserFixtureJSON(t, f)
	for _, field := range []string{`"binding":{"mode":"direct"}`, `"BINDING":null`, `"servicePid":1`, `"proxyPort":1234`, `"connectedStrictVerified":true`, `"extraArguments":["--no-sandbox"]`, `"remoteDebuggingPort":9222`} {
		injected := "{" + field + "," + data[1:]
		if _, err := f.controller.preview(injected, ""); err == nil {
			t.Fatal("frontend trust/path/argument injection accepted", field)
		}
		if _, err := f.controller.launch(injected); err == nil {
			t.Fatal("unsafe launch accepted")
		}
	}
	for _, mutate := range []func(map[string]any){
		func(c map[string]any) { c["webgl"] = "disabled" }, func(c map[string]any) { c["cookies"] = "block-third-party" },
		func(c map[string]any) { c["webrtc"] = "runtime-default" },
		func(c map[string]any) { c["permissions"].(map[string]any)["camera"] = "ask" },
	} {
		var config map[string]any
		_ = json.Unmarshal([]byte(data), &config)
		mutate(config)
		modified, _ := json.Marshal(config)
		if _, err := f.controller.preview(string(modified), ""); err == nil {
			t.Fatal("unsupported host parameter silently accepted")
		}
	}
	if f.starts.Load() != 0 || f.reads.Load() != 0 {
		t.Fatal("rejected input reached process/service")
	}
}

func TestBrowserStrictGatewayRejectsEveryMissingCondition(t *testing.T) {
	for _, mutate := range []func(*ipc.InstalledState){
		func(s *ipc.InstalledState) { s.ServicePID = 0 }, func(s *ipc.InstalledState) { s.View.Protocol = 0 },
		func(s *ipc.InstalledState) { s.View.State.Status = state.Degraded }, func(s *ipc.InstalledState) { s.View.State.Wanted = false },
		func(s *ipc.InstalledState) { s.View.Mode = "rule" }, func(s *ipc.InstalledState) { s.View.Settings.Mode = "direct" },
		func(s *ipc.InstalledState) { s.View.Settings.TUN = false }, func(s *ipc.InstalledState) { s.View.Settings.StrictRoute = false },
		func(s *ipc.InstalledState) { s.View.Settings.NoDirect = false }, func(s *ipc.InstalledState) { s.View.Guard = "" },
		func(s *ipc.InstalledState) { s.View.GuardError = "fixture failure" }, func(s *ipc.InstalledState) { s.View.Settings.MixedPort = 0 },
		func(s *ipc.InstalledState) { s.View.Settings.MixedPort = 65536 },
	} {
		observed := strictBrowserFixtureState()
		mutate(&observed)
		if _, _, err := strictBrowserGateway(observed); err == nil {
			t.Fatal("incomplete strict state accepted")
		}
	}
}

func TestBrowserLaunchUsesFreshBackendBindingAndLeaseClosesOnPIDChange(t *testing.T) {
	f := newBrowserFixture(t)
	result, err := f.controller.launch(browserFixtureJSON(t, f))
	if err != nil || result.Status != "ready" {
		t.Fatalf("launch result=%+v err=%v", result, err)
	}
	var frame struct {
		SchemaVersion int                       `json:"schemaVersion"`
		ServicePID    uint32                    `json:"servicePid"`
		ProxyPort     int                       `json:"proxyPort"`
		Strict        bool                      `json:"connectedStrictVerified"`
		Profile       browserhost.ProfileConfig `json:"profile"`
	}
	if err := json.Unmarshal(<-f.frames, &frame); err != nil {
		t.Fatal(err)
	}
	if frame.SchemaVersion != 1 || frame.ServicePID != 4242 || frame.ProxyPort != 31809 || !frame.Strict || frame.Profile.Binding.ProxyURL != "socks5://127.0.0.1:31809" || frame.Profile.Binding.AllowDirectFallback {
		t.Fatalf("host frame did not use backend state: %+v", frame)
	}
	f.ticks <- time.Now()
	select {
	case <-f.leases:
	case <-time.After(time.Second):
		t.Fatal("no lease after fresh strict recheck")
	}
	f.mu.Lock()
	f.view.ServicePID++
	f.mu.Unlock()
	f.ticks <- time.Now()
	waitBrowserClosed(t, f)
	select {
	case <-f.leases:
		t.Fatal("lease renewed after PID change")
	default:
	}
}

func TestBrowserLeaseClosesOnPortStrictStateOrIdentityReadFailure(t *testing.T) {
	for _, kind := range []string{"port", "guard", "wanted", "mode", "error"} {
		t.Run(kind, func(t *testing.T) {
			f := newBrowserFixture(t)
			if _, err := f.controller.launch(browserFixtureJSON(t, f)); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			switch kind {
			case "port":
				f.view.View.Settings.MixedPort++
			case "guard":
				f.view.View.Guard = ""
			case "wanted":
				f.view.View.State.Wanted = false
			case "mode":
				f.view.View.Settings.Mode = "rule"
			case "error":
				f.readErr = ipc.ErrUntrustedService
			}
			f.mu.Unlock()
			f.ticks <- time.Now()
			waitBrowserClosed(t, f)
		})
	}
}

func TestBrowserStartRefusedBeforeProcessWhenServiceUnverified(t *testing.T) {
	f := newBrowserFixture(t)
	f.readErr = ipc.ErrUntrustedService
	if _, err := f.controller.launch(browserFixtureJSON(t, f)); err == nil || f.starts.Load() != 0 {
		t.Fatal("unverified service started host")
	}
}

func TestBrowserRechecksIdentityAfterProcessCreationBeforeSendingFirstFrame(t *testing.T) {
	f := newBrowserFixture(t)
	originalStart := f.controller.deps.start
	f.controller.deps.start = func(ctx context.Context, path string) (*browserHostProcess, error) {
		process, err := originalStart(ctx, path)
		f.mu.Lock()
		f.view.ServicePID++
		f.mu.Unlock()
		return process, err
	}
	if _, err := f.controller.launch(browserFixtureJSON(t, f)); err == nil {
		t.Fatal("service changed during process creation but launch succeeded")
	}
	waitBrowserClosed(t, f)
	select {
	case <-f.frames:
		t.Fatal("stale binding first frame was sent")
	default:
	}
}

func TestBrowserCloseCancelsPendingServiceReadWithoutStartingHost(t *testing.T) {
	f := newBrowserFixture(t)
	data := browserFixtureJSON(t, f)
	reading := make(chan struct{})
	done := make(chan error, 1)
	f.controller.deps.state = func(ctx context.Context) (ipc.InstalledState, error) {
		close(reading)
		<-ctx.Done()
		return ipc.InstalledState{}, ctx.Err()
	}
	go func() { _, err := f.controller.launch(data); done <- err }()
	select {
	case <-reading:
	case <-time.After(time.Second):
		t.Fatal("fake gateway not called")
	}
	f.controller.close(false)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled service query accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not cancel pending query")
	}
	if f.starts.Load() != 0 {
		t.Fatal("host started after cancellation")
	}
}

func TestBrowserCloseCancelsPendingHandshakeAndShutdownIsPermanent(t *testing.T) {
	f := newBrowserFixture(t)
	f.noReady = true
	data := browserFixtureJSON(t, f)
	done := make(chan error, 1)
	go func() { _, err := f.controller.launch(data); done <- err }()
	select {
	case <-f.frames:
	case <-time.After(time.Second):
		t.Fatal("fake host never received first frame")
	}
	a := &App{ctx: context.Background(), browser: f.controller}
	a.shutdownBrowser()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled launch reported ready")
		}
	case <-time.After(time.Second):
		t.Fatal("close blocked behind launch")
	}
	waitBrowserClosed(t, f)
	if _, err := a.browserController(); err == nil {
		t.Fatal("UI shutdown allowed a new launcher")
	}
	if _, err := f.controller.launch(data); err == nil {
		t.Fatal("permanently closed controller restarted host")
	}
}

type browserInertWriteCloser struct{}

func (browserInertWriteCloser) Write(data []byte) (int, error) { return len(data), nil }
func (browserInertWriteCloser) Close() error                   { return nil }

func TestBrowserCloseDuringProcessCreationStillSchedulesBoundedKillAndReleasesPins(t *testing.T) {
	f := newBrowserFixture(t)
	data := browserFixtureJSON(t, f)
	entered := make(chan struct{})
	returnProcess := make(chan struct{})
	exited := make(chan struct{})
	killed := make(chan struct{})
	scheduled := make(chan time.Duration, 1)
	deadline := make(chan time.Time, 1)
	f.controller.deps.after = func(duration time.Duration) <-chan time.Time { scheduled <- duration; return deadline }
	f.controller.deps.start = func(context.Context, string) (*browserHostProcess, error) {
		close(entered)
		<-returnProcess
		return &browserHostProcess{stdin: browserInertWriteCloser{}, stdout: io.NopCloser(strings.NewReader("")), wait: func() error { <-exited; return nil }, kill: func() error { close(killed); close(exited); return nil }}, nil
	}
	baseline := f.releases.Load()
	done := make(chan error, 1)
	go func() { _, err := f.controller.launch(data); done <- err }()
	<-entered
	f.controller.mu.Lock()
	run := f.controller.current
	f.controller.mu.Unlock()
	f.controller.close(false)
	close(returnProcess)
	select {
	case duration := <-scheduled:
		if duration != 4*time.Second {
			t.Fatal("wrong bounded kill interval")
		}
	case <-time.After(time.Second):
		t.Fatal("new process never got a kill timer after earlier Close")
	}
	if f.releases.Load() != baseline {
		t.Fatal("code pins released while fake process was still alive")
	}
	deadline <- time.Now()
	select {
	case <-killed:
	case <-time.After(time.Second):
		t.Fatal("stalled fake process not killed")
	}
	select {
	case <-run.done:
	case <-time.After(time.Second):
		t.Fatal("process wait did not release pins")
	}
	if err := <-done; err == nil || f.releases.Load() != baseline+1 {
		t.Fatal("closed launch succeeded or pins leaked", err)
	}
}

func TestBrowserGraphicsPresetsAndSeedRemainBackendValidated(t *testing.T) {
	f := newBrowserFixture(t)
	defaults, err := f.controller.defaults("graphics-fixture")
	if err != nil || len(defaults.GraphicsPresets) != 3 {
		t.Fatal("graphics presets missing from Wails defaults", err)
	}
	var input map[string]any
	_ = json.Unmarshal(defaults.Config, &input)
	input["graphics"] = defaults.GraphicsPresets[0]
	input["webgpu"] = "disabled"
	data, _ := json.Marshal(input)
	preview, err := f.controller.preview(string(data), "graphics_seed_42")
	if err != nil || preview.GraphicsRequirements == nil {
		t.Fatal("supported graphics preview rejected", err)
	}
	var config map[string]any
	_ = json.Unmarshal(preview.Config, &config)
	if config["graphics"].(map[string]any)["mode"] != "webgl-metadata" || config["webgpu"] != "disabled" || preview.Applied || (!browserenginehost.NativeLaunchEnabled() && preview.CanLaunch) {
		t.Fatal("GPU preview lost settings or claimed live application")
	}
	input["webgpu"] = "runtime-default"
	data, _ = json.Marshal(input)
	if _, err := f.controller.preview(string(data), ""); err == nil {
		t.Fatal("WebGL metadata left real WebGPU adapter exposed")
	}
}

func TestBrowserMalformedHostFramesNeverReportReady(t *testing.T) {
	for _, line := range []string{`{"type":"ready","schemaVersion":2}`, `{"type":"ready","schemaVersion":1,"url":"https://example.invalid"}`, `{"type":"ready","type":"ready","schemaVersion":1}`, `{"type":"error","code":"configuration_refused"}`, `{"type":"ready"}`, strings.Repeat("x", 3000)} {
		f := newBrowserFixture(t)
		f.response = line
		if _, err := f.controller.launch(browserFixtureJSON(t, f)); err == nil {
			t.Fatal("malformed/error host frame reported ready")
		}
		waitBrowserClosed(t, f)
	}
}

func TestBrowserChildEnvironmentRejectsRuntimeAndWebViewInjection(t *testing.T) {
	input := []string{"SystemRoot=C:\\Windows", "LOCALAPPDATA=C:\\Users\\fixture", "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--no-sandbox", "WEBVIEW2_BROWSER_EXECUTABLE_FOLDER=remote", "DOTNET_STARTUP_HOOKS=evil.dll", "COMPlus_EnableDiagnostics=1", "COR_ENABLE_PROFILING=1", "CORECLR_PROFILER_PATH=evil.dll", "PATH=untrusted", "__COMPAT_LAYER=RunAsAdmin"}
	result := browserChildEnvironment(input)
	if len(result) != 2 || result[0] != input[0] || result[1] != input[1] {
		t.Fatalf("unsafe environment inherited: %v", result)
	}
}

func TestBrowserAvailabilityFailureDoesNotStartOrReadService(t *testing.T) {
	f := newBrowserFixture(t)
	f.controller.deps.inspect = func() (browserHostPlan, error) {
		return browserHostPlan{}, errors.New("fixture missing installed host")
	}
	defaults, err := f.controller.defaults("fixture-profile")
	if err != nil || defaults.HostAvailable || defaults.Status != "unavailable" || len(defaults.Config) == 0 {
		t.Fatalf("wrong unavailable boundary: %+v %v", defaults, err)
	}
	if f.reads.Load() != 0 || f.starts.Load() != 0 {
		t.Fatal("availability check touched service/process")
	}
}

func TestBrowserManifestRejectsTraversalDuplicatesMissingAndModifiedDependencies(t *testing.T) {
	hash := sha256.Sum256([]byte("inert fixture bytes"))
	digest := hex.EncodeToString(hash[:])
	names := []string{"godusevpn.exe", "browser-host/godusevpn.ultra.browserhost.exe", "browser-host/godusevpn.ultra.browserhost.exe.config", "browser-host/microsoft.web.webview2.core.dll", "browser-host/microsoft.web.webview2.winforms.dll", "browser-host/webview2loader.dll"}
	var lines []string
	actual := map[string]string{}
	for _, name := range names {
		lines = append(lines, digest+"  "+name)
		actual[name] = digest
	}
	manifest := strings.Join(lines, "\n") + "\n"
	expected, err := browserManifestHashes([]byte(manifest))
	if err != nil || verifyBrowserHostHashes(expected, actual) != nil {
		t.Fatal("valid synthetic manifest refused", err)
	}
	for _, path := range []string{"../outside.exe", "browser-host/../outside.exe", "/absolute.exe", `C:\outside.exe`, "browser-host/file.exe:stream", "browser-host/file.exe.", "browser-host//file.exe"} {
		if _, err := browserManifestHashes([]byte(digest + "  " + path + "\n")); err == nil {
			t.Fatal("unsafe manifest path accepted:", path)
		}
	}
	if _, err := browserManifestHashes([]byte(manifest + digest + "  GODUSEVPN.EXE\n")); err == nil {
		t.Fatal("casefold duplicate accepted")
	}
	actual[names[1]] = strings.Repeat("0", 64)
	if verifyBrowserHostHashes(expected, actual) == nil {
		t.Fatal("modified executable accepted")
	}
	actual[names[1]] = digest
	delete(actual, names[2])
	if verifyBrowserHostHashes(expected, actual) == nil {
		t.Fatal("missing pre-CLR config accepted")
	}
	actual[names[2]] = digest
	actual["browser-host/unlisted.dll"] = digest
	if verifyBrowserHostHashes(expected, actual) == nil {
		t.Fatal("unlisted dependency accepted")
	}
}

func TestBrowserCodeACLUsesOnlySyntheticDescriptors(t *testing.T) {
	for _, test := range []struct {
		descriptor        string
		ancestor, allowed bool
	}{
		{"O:BAG:BAD:P(A;;GA;;;BA)(A;;GA;;;SY)(A;;GRGX;;;BU)", false, true},
		{"O:BUG:BUD:P(A;;GA;;;BA)", false, false},
		{"O:BAG:BAD:P(A;;GA;;;BA)(A;;GW;;;BU)", false, false},
		{"O:BAG:BAD:P(A;;GA;;;BA)(A;;0x40;;;BU)", true, false},
	} {
		sd, err := windows.SecurityDescriptorFromString(test.descriptor)
		if err != nil {
			t.Fatal(err)
		}
		if (browserCodeDescriptor(sd, test.ancestor) == nil) != test.allowed {
			t.Fatal("unexpected synthetic ACL policy", test.descriptor)
		}
	}
}
