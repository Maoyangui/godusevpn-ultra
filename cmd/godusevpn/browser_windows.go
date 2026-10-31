//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
	"github.com/Maoyangui/godusevpn/internal/ipc"
	"golang.org/x/sys/windows"
)

type BrowserProfileDefaultsResult struct {
	Config             json.RawMessage                 `json:"config"`
	IdentityPreview    browserhost.IdentityPreview     `json:"identityPreview"`
	Regions            []browserhost.RegionConfig      `json:"regions"`
	GraphicsPresets    []browserhost.GraphicsConfig    `json:"graphicsPresets"`
	EnvironmentPresets []browserhost.EnvironmentPreset `json:"environmentPresets"`
	Status             string                          `json:"status"`
	Detail             string                          `json:"detail"`
	HostAvailable      bool                            `json:"hostAvailable"`
	CanLaunch          bool                            `json:"canLaunch"`
}

type BrowserProfilePreviewResult struct {
	Config                  json.RawMessage                                 `json:"config"`
	IdentityPreview         browserhost.IdentityPreview                     `json:"identityPreview"`
	Capabilities            browserhost.CapabilityMatrix                    `json:"capabilities"`
	Applied                 bool                                            `json:"applied"`
	CanLaunch               bool                                            `json:"canLaunch"`
	NeedsRuntimeReadback    bool                                            `json:"needsRuntimeReadback"`
	Warnings                []string                                        `json:"warnings"`
	GraphicsRequirements    *browserhost.GraphicsEnforcementRequirements    `json:"graphicsRequirements,omitempty"`
	EnvironmentRequirements *browserhost.EnvironmentEnforcementRequirements `json:"environmentRequirements,omitempty"`
	HardwareRequirements    *browserhost.HardwareEnforcementRequirements    `json:"hardwareRequirements,omitempty"`
	HostAvailable           bool                                            `json:"hostAvailable"`
	Status                  string                                          `json:"status"`
	Detail                  string                                          `json:"detail"`
}

type BrowserProfileLaunchResult struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type browserHostPlan struct {
	path    string
	release func()
}

type browserHostProcess struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	wait   func() error
	kill   func() error
}

type browserProfileRelay interface {
	Port() int
	Credentials() browserproxy.RelayCredentials
	Close()
}

type browserDependencies struct {
	runtime func() browserhost.RuntimeContext
	inspect func() (browserHostPlan, error)
	state   func(context.Context) (ipc.InstalledState, error)
	start   func(context.Context, string) (*browserHostProcess, error)
	ticks   func() (<-chan time.Time, func())
	after   func(time.Duration) <-chan time.Time
	relay   func(context.Context, browserproxy.Config) (browserProfileRelay, error)
	selfPID func() uint32
}

type browserController struct {
	mu      sync.Mutex
	ctx     context.Context
	deps    browserDependencies
	current *browserRun
	closed  bool
}

type browserRun struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	process     *browserHostProcess
	stopped     bool
	done        chan struct{}
	ready       chan error
	complete    sync.Once
	stopProcess sync.Once
	owner       *browserController
	release     func()
	relay       browserProfileRelay
}

var errBrowserRefused = errors.New("BROWSER_START_REFUSED：需要普通用户、可信安装及已验证的严格全局隧道")
var errBrowserConfig = errors.New("BROWSER_CONFIGURATION_REFUSED：配置包含本版主机不支持的参数")

func newBrowserController(ctx context.Context, deps browserDependencies) *browserController {
	return &browserController{ctx: ctx, deps: deps}
}

func browserPlaceholderBinding() browserhost.NetworkBinding {
	return browserhost.NetworkBinding{Mode: "tunnel", ReferenceID: "service-owned"}
}

// The UI never receives or supplies a service PID, proxy port, trust flag or
// binding. A duplicate binding in input is rejected by the strict model parser.
func parseBrowserProfile(data string, runtime browserhost.RuntimeContext) (browserhost.ProfileConfig, error) {
	data = strings.TrimSpace(data)
	if len(data) < 2 || len(data) > 32<<10 || data[0] != '{' {
		return browserhost.ProfileConfig{}, errBrowserConfig
	}
	binding, _ := json.Marshal(browserPlaceholderBinding())
	config, err := browserhost.DecodeProfile([]byte(`{"binding":`+string(binding)+`,`+data[1:]), runtime)
	if err != nil {
		return browserhost.ProfileConfig{}, errBrowserConfig
	}
	return config, nil
}

func publicBrowserConfig(config browserhost.ProfileConfig) json.RawMessage {
	data, _ := json.Marshal(config)
	var object map[string]json.RawMessage
	_ = json.Unmarshal(data, &object)
	delete(object, "binding")
	data, _ = json.Marshal(object)
	return data
}

func browserHostProfileSupported(config browserhost.ProfileConfig) error {
	identitySupported := config.Identity.Mode == "native" || config.Identity.Mode == "generated" || config.Identity.Mode == "manual"
	if config.EngineID != "webview2" || !identitySupported || (config.Identity.Mode != "manual" && config.Identity.UserAgent != "") || config.Identity.ClientHints != nil ||
		config.WebRTC != "disable-non-proxied-udp" || config.WebGL != "runtime-default" || (config.WebGPU != "runtime-default" && config.WebGPU != "disabled") ||
		config.Cookies != "runtime-default" || (config.ExitCleanup != "retain-profile" && config.ExitCleanup != "delete-owned-profile") {
		return errBrowserConfig
	}
	p := config.Permissions
	if p.Camera != "block" || p.Microphone != "block" || p.Geolocation != "block" || p.Notifications != "block" || p.ClipboardRead != "block" {
		return errBrowserConfig
	}
	return nil
}

func (c *browserController) available() bool {
	plan, err := c.deps.inspect()
	if err != nil {
		return false
	}
	plan.release()
	return true
}

func (c *browserController) defaults(profileID string) (BrowserProfileDefaultsResult, error) {
	result := BrowserProfileDefaultsResult{Regions: browserhost.RegionPresets(), GraphicsPresets: browserhost.GraphicsPresets(), EnvironmentPresets: browserhost.EnvironmentPresets(), Status: "unavailable", Detail: "需要安装版浏览器主机和 WebView2 运行时；本方法不启动浏览器。"}
	runtime := c.deps.runtime()
	if !runtime.Engine.Available {
		return result, nil
	}
	config, err := browserhost.DefaultProfile(runtime, profileID, browserPlaceholderBinding())
	if err != nil {
		return result, errBrowserConfig
	}
	config.WebRTC = "disable-non-proxied-udp"
	config.Identity = browserhost.ProfileIdentity{Mode: "generated"}
	config.Permissions = browserhost.PermissionConfig{Camera: "block", Microphone: "block", Geolocation: "block", Notifications: "block", ClipboardRead: "block"}
	result.Config = publicBrowserConfig(config)
	result.IdentityPreview, err = browserhost.PreviewIdentity(config, runtime)
	if err != nil {
		return BrowserProfileDefaultsResult{}, errBrowserConfig
	}
	result.HostAvailable = c.available()
	result.CanLaunch = result.HostAvailable && browserenginehost.NativeLaunchEnabled()
	if result.HostAvailable {
		result.Status = "preview"
		if result.CanLaunch {
			result.Detail = "主机文件检查通过；启动时仍须重新验证严格全局隧道，并由主机读回配置。"
		} else {
			result.Detail = "主机文件检查通过；当前发行构建未开放独立浏览器启动，需通过受控 Windows VM 验收构建。"
		}
	}
	return result, nil
}

func (c *browserController) preview(data, seed string) (BrowserProfilePreviewResult, error) {
	runtime := c.deps.runtime()
	config, err := parseBrowserProfile(data, runtime)
	if err != nil {
		return BrowserProfilePreviewResult{}, err
	}
	if seed != "" {
		generated, err := browserhost.RandomizedProfilePreview(config, runtime, seed)
		if err != nil {
			return BrowserProfilePreviewResult{}, errBrowserConfig
		}
		config = generated.Config
	}
	if err := browserHostProfileSupported(config); err != nil {
		return BrowserProfilePreviewResult{}, err
	}
	preview, err := browserhost.PreviewProfile(config, runtime)
	if err != nil {
		return BrowserProfilePreviewResult{}, errBrowserConfig
	}
	hostAvailable := c.available()
	canLaunch := hostAvailable && browserenginehost.NativeLaunchEnabled()
	detail := "仅预览；没有启动浏览器或修改系统。"
	if hostAvailable && !canLaunch {
		detail = "仅预览；当前发行构建未开放独立浏览器启动，需通过受控 Windows VM 验收构建。"
	}
	return BrowserProfilePreviewResult{
		Config: publicBrowserConfig(config), IdentityPreview: preview.IdentityPreview, Capabilities: preview.Capabilities, NeedsRuntimeReadback: true,
		Warnings:                append(preview.Warnings, "GPU 选项仅改变浏览器可见 WebGL 字符串，要求 WebGPU 阻断和主机预导航读回；不改变真实硬件，尚无 VPS 真实浏览器验收。"),
		GraphicsRequirements:    preview.GraphicsRequirements,
		EnvironmentRequirements: preview.EnvironmentRequirements,
		HardwareRequirements:    preview.HardwareRequirements,
		HostAvailable:           hostAvailable, CanLaunch: canLaunch, Status: "preview", Detail: detail,
	}, nil
}

func strictBrowserGateway(state ipc.InstalledState) (uint32, int, error) {
	v := state.View
	if state.ServicePID == 0 || v.Protocol != ipc.Version || string(v.State.Status) != "connected" || !v.State.Wanted ||
		v.Mode != "global" || v.Settings.Mode != "global" || !v.Settings.TUN || !v.Settings.StrictRoute || !v.Settings.NoDirect ||
		v.Guard != "on" || v.GuardError != "" || v.Settings.MixedPort < 1 || v.Settings.MixedPort > 65535 {
		return 0, 0, errBrowserRefused
	}
	return state.ServicePID, v.Settings.MixedPort, nil
}

func (c *browserController) gateway(ctx context.Context, pid uint32, port int) (uint32, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	observed, err := c.deps.state(ctx)
	if err != nil {
		return 0, 0, errBrowserRefused
	}
	nextPID, nextPort, err := strictBrowserGateway(observed)
	if err != nil || (pid != 0 && (nextPID != pid || nextPort != port)) || ctx.Err() != nil {
		return 0, 0, errBrowserRefused
	}
	return nextPID, nextPort, nil
}

func (r *browserRun) stop() {
	r.mu.Lock()
	r.stopped = true
	p := r.process
	relay := r.relay
	r.mu.Unlock()
	r.cancel()
	if relay != nil {
		relay.Close()
	}
	if p != nil {
		r.stopProcess.Do(func() {
			// EOF is an irreversible close instruction. Close also unblocks a
			// stalled write, without waiting for another lease or service query.
			_ = p.stdin.Close()
			_ = p.stdout.Close()
			after := r.owner.deps.after
			if after == nil {
				after = time.After
			}
			deadline := after(4 * time.Second)
			go func() {
				select {
				case <-r.done:
				case <-deadline:
					_ = p.kill()
				}
			}()
		})
	}
}

func (r *browserRun) finish() {
	r.complete.Do(func() {
		r.stop()
		if r.release != nil {
			r.release()
		}
		r.owner.mu.Lock()
		if r.owner.current == r {
			r.owner.current = nil
		}
		r.owner.mu.Unlock()
		close(r.done)
	})
}

func (r *browserRun) write(data []byte) error {
	r.mu.Lock()
	p, stopped := r.process, r.stopped
	r.mu.Unlock()
	if stopped || p == nil {
		return errBrowserRefused
	}
	done := make(chan error, 1)
	go func() {
		n, err := p.stdin.Write(data)
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-r.ctx.Done():
		return errBrowserRefused
	case <-time.After(400 * time.Millisecond):
		r.stop()
		return errBrowserRefused
	}
}

func browserReadyFrame(line []byte) (bool, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, line); err != nil {
		return false, errBrowserRefused
	}
	s := compact.String()
	if s == `{"type":"ready","schemaVersion":1}` || s == `{"schemaVersion":1,"type":"ready"}` {
		return true, nil
	}
	for _, code := range []string{"start_refused", "configuration_refused"} {
		if s == `{"type":"error","code":"`+code+`"}` || s == `{"code":"`+code+`","type":"error"}` {
			return false, errBrowserRefused
		}
	}
	return false, errBrowserRefused
}

func (r *browserRun) observe() {
	scanner := bufio.NewScanner(r.process.stdout)
	scanner.Buffer(make([]byte, 512), 2048)
	readySeen := false
	for scanner.Scan() {
		ready, err := browserReadyFrame(scanner.Bytes())
		if err != nil || !ready || readySeen {
			select {
			case r.ready <- errBrowserRefused:
			default:
			}
			r.stop()
			return
		}
		readySeen = true
		r.ready <- nil
	}
	r.stop()
}

func (c *browserController) lease(r *browserRun, pid uint32, port int) {
	ticks, stop := c.deps.ticks()
	defer stop()
	for {
		select {
		case <-r.ctx.Done():
			r.stop()
			return
		case _, ok := <-ticks:
			if !ok {
				r.stop()
				return
			}
			if _, _, err := c.gateway(r.ctx, pid, port); err != nil || r.write([]byte("{\"type\":\"lease\"}\n")) != nil {
				r.stop()
				return
			}
		}
	}
}

func (c *browserController) launch(data string) (result BrowserProfileLaunchResult, err error) {
	return c.launchWithProxy(data, nil)
}

func (c *browserController) launchProxy(data, proxyJSON string) (BrowserProfileLaunchResult, error) {
	proxy, err := browserproxy.Decode(proxyJSON)
	if err != nil {
		return BrowserProfileLaunchResult{}, errBrowserConfig
	}
	return c.launchWithProxy(data, &proxy)
}

func (c *browserController) launchWithProxy(data string, customProxy *browserproxy.Config) (result BrowserProfileLaunchResult, err error) {
	ctx, cancel := context.WithCancel(c.ctx)
	r := &browserRun{ctx: ctx, cancel: cancel, done: make(chan struct{}), ready: make(chan error, 1), owner: c}
	c.mu.Lock()
	if c.closed || c.current != nil || ctx.Err() != nil {
		c.mu.Unlock()
		cancel()
		return result, errBrowserRefused
	}
	c.current = r
	c.mu.Unlock()
	started := false
	defer func() {
		if err != nil {
			r.stop()
		}
		if !started {
			r.finish()
		}
	}()
	runtime := c.deps.runtime()
	config, err := parseBrowserProfile(data, runtime)
	if err != nil {
		return result, err
	}
	if err = browserHostProfileSupported(config); err != nil {
		return result, err
	}
	plan, err := c.deps.inspect()
	if err != nil {
		return result, errBrowserRefused
	}
	r.release = plan.release
	pid, port, err := c.gateway(ctx, 0, 0)
	if err != nil {
		return result, err
	}
	if ctx.Err() != nil {
		return result, errBrowserRefused
	}
	proxyPort := port
	var relayPID uint32
	var relayAuth browserproxy.RelayCredentials
	if customProxy != nil {
		if c.deps.relay == nil || c.deps.selfPID == nil {
			return result, errBrowserRefused
		}
		relayPID = c.deps.selfPID()
		if relayPID == 0 {
			return result, errBrowserRefused
		}
		relay, relayErr := c.deps.relay(ctx, *customProxy)
		if relayErr != nil || relay == nil {
			if relay != nil {
				relay.Close()
			}
			return result, errBrowserRefused
		}
		r.mu.Lock()
		r.relay = relay
		relayStopped := r.stopped
		r.mu.Unlock()
		if relayStopped || ctx.Err() != nil {
			relay.Close()
			return result, errBrowserRefused
		}
		proxyPort = relay.Port()
		relayAuth = relay.Credentials()
		if proxyPort < 1 || proxyPort > 65535 || !validBrowserRelayCredentials(relayAuth) {
			return result, errBrowserRefused
		}
	}
	process, err := c.deps.start(ctx, plan.path)
	if err != nil {
		return result, errBrowserRefused
	}
	started = true
	r.mu.Lock()
	r.process = process
	stopped := r.stopped
	r.mu.Unlock()
	go func() { _ = process.wait(); r.finish() }()
	if stopped {
		r.stop()
		return result, errBrowserRefused
	}
	if _, _, err = c.gateway(ctx, pid, port); err != nil {
		return result, err
	}
	reference := "installed-service"
	if customProxy != nil {
		reference = "profile-relay"
	}
	config.Binding = browserhost.NetworkBinding{Mode: "loopback-proxy", ReferenceID: reference, ProxyURL: "socks5://127.0.0.1:" + strconv.Itoa(proxyPort)}
	frame := struct {
		SchemaVersion           int                       `json:"schemaVersion"`
		ServicePID              uint32                    `json:"servicePid"`
		ProxyPort               int                       `json:"proxyPort"`
		ConnectedStrictVerified bool                      `json:"connectedStrictVerified"`
		RelayPID                uint32                    `json:"relayPid,omitempty"`
		RelayUsername           string                    `json:"relayUsername,omitempty"`
		RelayPassword           string                    `json:"relayPassword,omitempty"`
		Profile                 browserhost.ProfileConfig `json:"profile"`
	}{1, pid, proxyPort, true, relayPID, relayAuth.Username, relayAuth.Password, config}
	first, _ := json.Marshal(frame)
	if err = r.write(append(first, '\n')); err != nil {
		return result, errBrowserRefused
	}
	go r.observe()
	go c.lease(r, pid, port)
	select {
	case err = <-r.ready:
		if err != nil {
			return result, err
		}
		if _, _, err = c.gateway(ctx, pid, port); err != nil || ctx.Err() != nil {
			return result, errBrowserRefused
		}
		detail := "独立主机已完成本次配置读回；每 500ms 复核严格隧道和服务，变化或核验失败即关闭。"
		if customProxy != nil {
			detail = "主机已完成本次配置读回，并绑定内存认证的配置代理；当前仍要求 Ultra 严格隧道叠加，每 500ms 复核，失败即关闭；尚无 VPS 实测。"
		}
		return BrowserProfileLaunchResult{Status: "ready", Detail: detail}, nil
	case <-ctx.Done():
		return result, errBrowserRefused
	case <-time.After(30 * time.Second):
		return result, errBrowserRefused
	}
}

func validBrowserRelayCredentials(auth browserproxy.RelayCredentials) bool {
	if auth.Username != "ultra-browser" || len(auth.Password) != 64 {
		return false
	}
	for _, ch := range auth.Password {
		if !(ch >= 'a' && ch <= 'f' || ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

func (c *browserController) close(permanent bool) {
	c.mu.Lock()
	if permanent {
		c.closed = true
	}
	r := c.current
	c.mu.Unlock()
	if r != nil {
		r.stop()
	}
}

func (a *App) browserController() (*browserController, error) {
	a.browserMu.Lock()
	defer a.browserMu.Unlock()
	if a.browserClosed || a.ctx == nil || a.ctx.Err() != nil {
		return nil, errBrowserRefused
	}
	if a.browser == nil {
		a.browser = newBrowserController(a.ctx, nativeBrowserDependencies())
	}
	return a.browser, nil
}

func (a *App) BrowserProfileDefaults(profileID string) (BrowserProfileDefaultsResult, error) {
	c, err := a.browserController()
	if err != nil {
		return BrowserProfileDefaultsResult{}, err
	}
	return c.defaults(profileID)
}
func (a *App) BrowserProfilePreview(configJSON string, seed string) (BrowserProfilePreviewResult, error) {
	c, err := a.browserController()
	if err != nil {
		return BrowserProfilePreviewResult{}, err
	}
	return c.preview(configJSON, seed)
}

// BrowserProfileV2Preview validates the complete profile envelope before
// delegating to the legacy pure preview. Unsupported extras are returned in a
// capability receipt and never silently discarded.
func (a *App) BrowserProfileV2Preview(requestJSON string, seed string) (BrowserProfileV2PreviewResult, error) {
	request, receipt, err := parseBrowserProfileV2Request(requestJSON)
	if err != nil {
		return BrowserProfileV2PreviewResult{}, err
	}
	if request.Engine.EngineID != "webview2" && request.Engine.EngineID != "webview" {
		receipt = browserProfileV2Unsupported(receipt, "engine")
	}
	if !receipt.Accepted {
		return BrowserProfileV2PreviewResult{Status: "unsupported", Detail: "请求包含当前主机未实现的字段；未启动浏览器。", Receipt: receipt}, nil
	}
	preview, err := a.BrowserProfilePreview(string(request.Config), seed)
	if err != nil {
		return BrowserProfileV2PreviewResult{}, err
	}
	return BrowserProfileV2PreviewResult{Status: "preview", Detail: preview.Detail, Receipt: receipt, Preview: &preview}, nil
}

// BrowserProfileIdentity plans browser-visible identity from local inventory.
// It neither starts a runtime nor treats a product version as observed CH.
func (a *App) BrowserProfileIdentity(configJSON string) (browserhost.IdentityPreview, error) {
	c, err := a.browserController()
	if err != nil {
		return browserhost.IdentityPreview{}, err
	}
	runtime := c.deps.runtime()
	config, err := parseBrowserProfile(configJSON, runtime)
	if err != nil {
		return browserhost.IdentityPreview{}, err
	}
	if err = browserHostProfileSupported(config); err != nil {
		return browserhost.IdentityPreview{}, err
	}
	return browserhost.PreviewIdentity(config, runtime)
}

// BrowserProfileLaunch is called only by an explicit user action. Startup,
// preview, refresh, Connect and Disconnect never invoke it.
func (a *App) BrowserProfileLaunch(configJSON string) (BrowserProfileLaunchResult, error) {
	c, err := a.browserController()
	if err != nil {
		return BrowserProfileLaunchResult{}, err
	}
	return c.launch(configJSON)
}

// BrowserProfileV2Launch refuses any valid-but-unimplemented extra before the
// trusted Windows controller can create a process or bind a proxy.
func (a *App) BrowserProfileV2Launch(requestJSON string) (BrowserProfileV2LaunchResult, error) {
	request, receipt, err := parseBrowserProfileV2Request(requestJSON)
	if err != nil {
		return BrowserProfileV2LaunchResult{}, err
	}
	if request.Engine.EngineID != "webview2" && request.Engine.EngineID != "webview" {
		receipt = browserProfileV2Unsupported(receipt, "engine")
	}
	if !receipt.Accepted {
		return BrowserProfileV2LaunchResult{Status: "unsupported", Detail: "请求包含当前主机未实现的字段；未启动浏览器。", Receipt: receipt}, nil
	}
	launch, err := a.BrowserProfileLaunch(string(request.Config))
	if err != nil {
		return BrowserProfileV2LaunchResult{}, err
	}
	return BrowserProfileV2LaunchResult{Status: launch.Status, Detail: launch.Detail, Receipt: receipt, Launch: &launch}, nil
}

// The custom proxy remains layered over the same freshly verified strict
// Ultra tunnel. A probe result never authorizes launch or replaces that lease.
func (a *App) BrowserProfileLaunchProxy(configJSON, proxyJSON string) (BrowserProfileLaunchResult, error) {
	c, err := a.browserController()
	if err != nil {
		return BrowserProfileLaunchResult{}, err
	}
	return c.launchProxy(configJSON, proxyJSON)
}

func (a *App) BrowserProfileV2LaunchProxy(requestJSON, proxyJSON string) (BrowserProfileV2LaunchResult, error) {
	request, receipt, err := parseBrowserProfileV2Request(requestJSON)
	if err != nil {
		return BrowserProfileV2LaunchResult{}, err
	}
	if request.Engine.EngineID != "webview2" && request.Engine.EngineID != "webview" {
		receipt = browserProfileV2Unsupported(receipt, "engine")
	}
	if !receipt.Accepted {
		return BrowserProfileV2LaunchResult{Status: "unsupported", Detail: "请求包含当前主机未实现的字段；未启动浏览器。", Receipt: receipt}, nil
	}
	launch, err := a.BrowserProfileLaunchProxy(string(request.Config), proxyJSON)
	if err != nil {
		return BrowserProfileV2LaunchResult{}, err
	}
	return BrowserProfileV2LaunchResult{Status: launch.Status, Detail: launch.Detail, Receipt: receipt, Launch: &launch}, nil
}
func (a *App) BrowserProfileClose() error {
	a.browserMu.Lock()
	c := a.browser
	a.browserMu.Unlock()
	if c != nil {
		c.close(false)
	}
	return nil
}
func (a *App) shutdownBrowser() {
	a.browserMu.Lock()
	a.browserClosed = true
	c := a.browser
	a.browserMu.Unlock()
	if c != nil {
		c.close(true)
	}
}

// Convert only a real Relay pointer into the interface. Returning a nil *Relay
// directly would create a non-nil interface and panic in error cleanup. Keep a
// non-nil partial owner with its error so the caller can still close it.
func normalizeBrowserProfileRelay(relay *browserproxy.Relay, err error) (browserProfileRelay, error) {
	if relay == nil {
		return nil, err
	}
	return relay, err
}

func nativeBrowserDependencies() browserDependencies {
	return browserDependencies{
		runtime: func() browserhost.RuntimeContext {
			for _, engine := range browserhost.Read().Engines {
				if engine.ID == "webview" || engine.ID == "webview2" {
					return browserhost.RuntimeContext{Engine: engine}
				}
			}
			return browserhost.RuntimeContext{Engine: browserhost.Engine{ID: "webview2"}}
		}, inspect: inspectInstalledBrowserHost, state: ipc.ReadInstalledState, start: startInstalledBrowserHost,
		relay: func(ctx context.Context, config browserproxy.Config) (browserProfileRelay, error) {
			relay, err := browserproxy.StartRelay(ctx, config)
			return normalizeBrowserProfileRelay(relay, err)
		}, selfPID: func() uint32 { return uint32(os.Getpid()) },
		ticks: func() (<-chan time.Time, func()) {
			ticker := time.NewTicker(500 * time.Millisecond)
			return ticker.C, ticker.Stop
		},
	}
}

func ordinaryBrowserUser() error {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return errBrowserRefused
	}
	if sid := user.User.Sid.String(); sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20" {
		return errBrowserRefused
	}
	var elevated, used uint32
	if windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &used) != nil || used != 4 || elevated != 0 {
		return errBrowserRefused
	}
	buf := make([]byte, 512)
	if windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buf[0], uint32(len(buf)), &used) != nil {
		return errBrowserRefused
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buf[0]))
	if label.Label.Sid == nil {
		return errBrowserRefused
	}
	sid := label.Label.Sid.String()
	if sid != "S-1-16-8192" && sid != "S-1-16-8448" {
		return errBrowserRefused
	}
	return nil
}

func browserCodeDescriptor(sd *windows.SECURITY_DESCRIPTOR, ancestor bool) error {
	if sd == nil {
		return errBrowserRefused
	}
	trusted := func(s string) bool {
		return s == "S-1-5-18" || s == "S-1-5-32-544" || ancestor && s == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trusted(owner.String()) {
		return errBrowserRefused
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errBrowserRefused
	}
	mask := uint32(0x500d0040)
	if !ancestor {
		mask |= 0x116
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil {
			return errBrowserRefused
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errBrowserRefused
		}
		if uint32(ace.Mask)&mask != 0 && !trusted((*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()) {
			return errBrowserRefused
		}
	}
	return nil
}

func inspectInstalledBrowserHost() (browserHostPlan, error) {
	if ordinaryBrowserUser() != nil {
		return browserHostPlan{}, errBrowserRefused
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return browserHostPlan{}, errBrowserRefused
	}
	root := filepath.Join(base, "GodUseVPN Ultra")
	self, err := os.Executable()
	if err != nil || !strings.EqualFold(filepath.Clean(self), filepath.Join(root, "godusevpn.exe")) {
		return browserHostPlan{}, errBrowserRefused
	}
	volume := filepath.VolumeName(root)
	if len(volume) != 2 || windows.GetDriveType(windows.StringToUTF16Ptr(volume+`\`)) != windows.DRIVE_FIXED {
		return browserHostPlan{}, errBrowserRefused
	}
	var held []*os.File
	files := map[string]*os.File{}
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			_ = held[i].Close()
		}
	}
	complete := false
	defer func() {
		if !complete {
			release()
		}
	}()
	pin := func(path string, directory, ancestor bool) error {
		access, share, flags := uint32(windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES), uint32(windows.FILE_SHARE_READ), uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
		if directory {
			access |= windows.FILE_LIST_DIRECTORY
			share |= windows.FILE_SHARE_WRITE
			flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		} else {
			access |= windows.FILE_READ_DATA
		}
		h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), access, share, nil, windows.OPEN_EXISTING, flags, 0)
		if err != nil {
			return errBrowserRefused
		}
		f := os.NewFile(uintptr(h), "browser-install-code")
		if f == nil {
			windows.CloseHandle(h)
			return errBrowserRefused
		}
		held = append(held, f)
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
			return errBrowserRefused
		}
		sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil || browserCodeDescriptor(sd, ancestor) != nil {
			return errBrowserRefused
		}
		if !directory {
			files[path] = f
		}
		return nil
	}
	current := volume + `\`
	if err := pin(current, true, true); err != nil {
		return browserHostPlan{}, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(root, current), `\`) {
		current = filepath.Join(current, part)
		if err := pin(current, true, current != root); err != nil {
			return browserHostPlan{}, err
		}
	}
	if err := pin(self, false, false); err != nil {
		return browserHostPlan{}, err
	}
	manifestPath := filepath.Join(root, "SHA256SUMS")
	if err := pin(manifestPath, false, false); err != nil {
		return browserHostPlan{}, err
	}
	hostRoot := filepath.Join(root, "browser-host")
	count := 0
	err = filepath.WalkDir(hostRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return errBrowserRefused
		}
		count++
		if count > 10000 {
			return errBrowserRefused
		}
		return pin(path, entry.IsDir(), false)
	})
	if err != nil {
		return browserHostPlan{}, errBrowserRefused
	}
	host := filepath.Join(hostRoot, "GodUseVPN.Ultra.BrowserHost.exe")
	info, err := os.Stat(host)
	if err != nil || !info.Mode().IsRegular() {
		return browserHostPlan{}, errBrowserRefused
	}
	manifest, err := io.ReadAll(io.LimitReader(files[manifestPath], (4<<20)+1))
	if err != nil || len(manifest) > 4<<20 {
		return browserHostPlan{}, errBrowserRefused
	}
	expected, err := browserManifestHashes(manifest)
	if err != nil {
		return browserHostPlan{}, err
	}
	actual := map[string]string{}
	for path, file := range files {
		if path == manifestPath {
			continue
		}
		stat, err := file.Stat()
		if err != nil || stat.Size() > 256<<20 {
			return browserHostPlan{}, errBrowserRefused
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return browserHostPlan{}, errBrowserRefused
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return browserHostPlan{}, errBrowserRefused
		}
		actual[strings.ToLower(filepath.ToSlash(relative))] = hex.EncodeToString(hash.Sum(nil))
	}
	if err := verifyBrowserHostHashes(expected, actual); err != nil {
		return browserHostPlan{}, err
	}
	complete = true
	return browserHostPlan{path: host, release: release}, nil
}

func browserManifestHashes(data []byte) (map[string]string, error) {
	result := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 65536)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return nil, errBrowserRefused
		}
		digest, name := line[:64], line[66:]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
			return nil, errBrowserRefused
		}
		for _, part := range strings.Split(name, "/") {
			if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
				return nil, errBrowserRefused
			}
			for _, ch := range part {
				if ch < 32 {
					return nil, errBrowserRefused
				}
			}
		}
		name = strings.ToLower(name)
		if _, duplicate := result[name]; duplicate {
			return nil, errBrowserRefused
		}
		result[name] = strings.ToLower(digest)
	}
	if scanner.Err() != nil || len(result) == 0 {
		return nil, errBrowserRefused
	}
	return result, nil
}

func verifyBrowserHostHashes(expected, actual map[string]string) error {
	for _, name := range []string{"godusevpn.exe", "browser-host/godusevpn.ultra.browserhost.exe", "browser-host/godusevpn.ultra.browserhost.exe.config", "browser-host/microsoft.web.webview2.core.dll", "browser-host/microsoft.web.webview2.winforms.dll", "browser-host/webview2loader.dll"} {
		if expected[name] == "" || actual[name] != expected[name] {
			return errBrowserRefused
		}
	}
	for name, digest := range actual {
		if expected[name] != digest {
			return errBrowserRefused
		}
	}
	for name := range expected {
		if strings.HasPrefix(name, "browser-host/") && actual[name] == "" {
			return errBrowserRefused
		}
	}
	return nil
}

func browserChildEnvironment(environment []string) []string {
	allowed := map[string]bool{"SYSTEMROOT": true, "WINDIR": true, "TEMP": true, "TMP": true, "USERPROFILE": true, "LOCALAPPDATA": true, "APPDATA": true, "PROGRAMDATA": true, "PROGRAMFILES": true, "PROGRAMFILES(X86)": true, "HOMEDRIVE": true, "HOMEPATH": true, "USERNAME": true, "USERDOMAIN": true}
	var result []string
	for _, value := range environment {
		key, _, ok := strings.Cut(value, "=")
		if ok && allowed[strings.ToUpper(key)] {
			result = append(result, value)
		}
	}
	return result
}

func startInstalledBrowserHost(ctx context.Context, path string) (*browserHostProcess, error) {
	if ctx.Err() != nil || ordinaryBrowserUser() != nil {
		return nil, errBrowserRefused
	}
	command := exec.Command(path)
	command.Dir = filepath.Dir(path)
	command.Env = browserChildEnvironment(os.Environ())
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, err
	}
	return &browserHostProcess{stdin: stdin, stdout: stdout, wait: command.Wait, kill: command.Process.Kill}, nil
}
