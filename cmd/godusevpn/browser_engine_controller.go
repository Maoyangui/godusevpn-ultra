package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

var errBrowserEngineRefused = errors.New("BROWSER_ENGINE_OPERATION_REFUSED")
var errBrowserEngineConfig = errors.New("BROWSER_ENGINE_CONFIGURATION_REFUSED_NATIVE_PROFILE_ONLY")
var errBrowserEngineCancelled = errors.New("BROWSER_ENGINE_START_CANCELLED")

type browserEngineProfileRequest struct {
	EngineID    string                         `json:"engineId"`
	Version     string                         `json:"version"`
	ProfileID   string                         `json:"profileId"`
	Language    string                         `json:"language"`
	Timezone    string                         `json:"timezone"`
	Width       int                            `json:"width"`
	Height      int                            `json:"height"`
	Identity    browserhost.ProfileIdentity    `json:"identity,omitempty"`
	Environment *browserhost.EnvironmentConfig `json:"environment,omitempty"`
	Hardware    *browserhost.HardwareConfig    `json:"hardware,omitempty"`
	Graphics    *browserhost.GraphicsConfig    `json:"graphics,omitempty"`
	WebGPU      string                         `json:"webgpu,omitempty"`
}
type BrowserEngineLaunchResult struct {
	Status         browserenginehost.Status `json:"status"`
	CanLaunch      bool                     `json:"canLaunch"`
	Detail         string                   `json:"detail"`
	StartID        string                   `json:"startId,omitempty"`
	CanCancelStart bool                     `json:"canCancelStart"`
	Code           string                   `json:"code,omitempty"`
}
type browserEngineSession interface {
	ID() string
	Status() browserenginehost.Status
	Navigate(context.Context, string) error
	Close() error
	Done() <-chan struct{}
}
type browserEngineDependencies struct {
	manager func(browserproxy.Config) (*browserengine.Manager, error)
	guard   func() (browserenginehost.Guard, error)
	start   func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error)
}
type browserEngineController struct {
	mu          sync.Mutex
	ctx         context.Context
	deps        browserEngineDependencies
	current     *browserEngineRun
	closed      bool
	maintenance bool
	last        BrowserEngineLaunchResult
}
type browserEngineRun struct {
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	session        browserEngineSession
	stopped        bool
	id             string
	starting       bool
	cancelledStart bool
	done           chan struct{}
	result         BrowserEngineLaunchResult
	err            error
	closeOnce      sync.Once
	closeStarted   bool
	closePending   bool
	closeErr       error
}

var engineProfileID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var engineChromeVersion = regexp.MustCompile(`^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}$`)
var engineFirefoxVersion = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}(?:\.[0-9]{1,3})?(?:esr)?$`)
var engineSessionID = regexp.MustCompile(`^[0-9a-f]{64}$`)

func parseBrowserEngineRequest(data string) (browserEngineProfileRequest, error) {
	var request browserEngineProfileRequest
	if len(data) == 0 || len(data) > 4096 {
		return request, errBrowserEngineConfig
	}
	d := json.NewDecoder(strings.NewReader(data))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return request, errBrowserEngineConfig
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[strings.ToLower(key)] {
			return request, errBrowserEngineConfig
		}
		seen[strings.ToLower(key)] = true
		switch key {
		case "engineId", "version", "profileId", "language", "timezone", "width", "height", "identity", "environment", "hardware", "graphics", "webgpu":
		default:
			return request, errBrowserEngineConfig
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return request, errBrowserEngineConfig
		}
		if key == "identity" && !strictBrowserEngineFields(value, map[string]bool{"mode": true, "userAgent": true}) {
			return request, errBrowserEngineConfig
		}
		if key == "environment" && !strictBrowserEngineFields(value, map[string]bool{"mode": true, "os": true, "devicePresetId": true, "osVersion": true}) {
			return request, errBrowserEngineConfig
		}
		if key == "graphics" && !strictBrowserEngineFields(value, map[string]bool{"mode": true, "presetId": true, "vendor": true, "renderer": true}) {
			return request, errBrowserEngineConfig
		}
		if key == "webgpu" {
			var mode string
			if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &mode) != nil || (mode != "runtime-default" && mode != "disabled") {
				return request, errBrowserEngineConfig
			}
		}
		if key == "hardware" {
			var hardware browserhost.HardwareConfig
			if json.Unmarshal(value, &hardware) != nil {
				return request, errBrowserEngineConfig
			}
		}
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') {
		return request, errBrowserEngineConfig
	}
	if _, err := d.Token(); err != io.EOF {
		return request, errBrowserEngineConfig
	}
	d = json.NewDecoder(bytes.NewBufferString(data))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil {
		return request, errBrowserEngineConfig
	}
	if request.EngineID == browserengine.Chrome {
		if !engineChromeVersion.MatchString(request.Version) {
			return request, errBrowserEngineConfig
		}
	} else if request.EngineID == browserengine.Firefox {
		if !engineFirefoxVersion.MatchString(request.Version) {
			return request, errBrowserEngineConfig
		}
	} else {
		return request, errBrowserEngineConfig
	}
	if request.Hardware != nil && request.EngineID != browserengine.Chrome {
		return request, errBrowserEngineConfig
	}
	if browserenginehost.ValidateGraphicsConfig(request.hostConfig(browserproxy.Config{})) != nil {
		return request, errBrowserEngineConfig
	}
	if !engineProfileID.MatchString(request.ProfileID) || strings.HasSuffix(request.ProfileID, "-") {
		return request, errBrowserEngineConfig
	}
	switch request.ProfileID {
	case "con", "prn", "aux", "nul":
		return request, errBrowserEngineConfig
	}
	if len(request.ProfileID) == 4 && (strings.HasPrefix(request.ProfileID, "com") || strings.HasPrefix(request.ProfileID, "lpt")) && request.ProfileID[3] >= '1' && request.ProfileID[3] <= '9' {
		return request, errBrowserEngineConfig
	}
	minWidth, minHeight := 800, 600
	if request.Environment != nil && request.Environment.Mode == "browser-emulation" {
		preset, err := browserhost.EffectiveEnvironment(*request.Environment)
		if err != nil {
			return request, errBrowserEngineConfig
		}
		if preset.Mobile {
			minWidth, minHeight = 320, 568
		}
	}
	if request.Width < minWidth || request.Width > 3840 || request.Height < minHeight || request.Height > 2160 {
		return request, errBrowserEngineConfig
	}
	region := false
	for _, preset := range browserhost.RegionPresets() {
		if request.Language == preset.Language && request.Timezone == preset.Timezone {
			region = true
		}
	}
	if !region {
		return request, errBrowserEngineConfig
	}
	if _, err := browserenginehost.PreviewIdentity(request.hostConfig(browserproxy.Config{})); err != nil {
		return request, errBrowserEngineConfig
	}
	return request, nil
}

func strictBrowserEngineFields(data []byte, allowed map[string]bool) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] || seen[strings.ToLower(key)] {
			return false
		}
		seen[strings.ToLower(key)] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || len(value) == 0 || value[0] != '"' {
			return false
		}
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') {
		return false
	}
	_, err = d.Token()
	return err == io.EOF
}
func (r browserEngineProfileRequest) hostConfig(proxy browserproxy.Config) browserenginehost.Config {
	return browserenginehost.Config{EngineID: r.EngineID, Version: r.Version, ProfileID: r.ProfileID, Language: r.Language, Timezone: r.Timezone, Width: r.Width, Height: r.Height, Identity: r.Identity, Environment: r.Environment, Hardware: r.Hardware, Graphics: r.Graphics, WebGPU: r.WebGPU, Proxy: proxy}
}

// BrowserEngineIdentity is a local plan for the selected actual engine version.
// The native driver must observe and verify that version before applying it.
func (a *App) BrowserEngineIdentity(configJSON string) (browserhost.IdentityPreview, error) {
	request, err := parseBrowserEngineRequest(configJSON)
	if err != nil {
		return browserhost.IdentityPreview{}, err
	}
	return browserenginehost.PreviewIdentity(request.hostConfig(browserproxy.Config{}))
}
func newBrowserEngineController(ctx context.Context, deps browserEngineDependencies) *browserEngineController {
	return &browserEngineController{ctx: ctx, deps: deps}
}

// Only a returned Session can be closed here. During Start, the host's own
// goroutine owns its partially assigned resources and cleans them after ctx
// cancellation; callers must not concurrently invoke its lifetime.Close.
func (r *browserEngineRun) closeSession() error {
	r.mu.Lock()
	session := r.session
	if session != nil && !r.closeStarted {
		r.closeStarted, r.closePending = true, true
	}
	r.mu.Unlock()
	if session == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		err := session.Close()
		r.mu.Lock()
		r.closeErr = err
		r.closePending = false
		r.mu.Unlock()
	})
	r.mu.Lock()
	err := r.closeErr
	r.mu.Unlock()
	if err != nil {
		return errBrowserEngineRefused
	}
	return nil
}
func (r *browserEngineRun) stop() error {
	r.mu.Lock()
	r.stopped = true
	if r.session != nil && !r.closeStarted {
		r.closeStarted, r.closePending = true, true
	}
	if r.starting {
		r.cancelledStart = true
	}
	r.mu.Unlock()
	r.cancel()
	return r.closeSession()
}
func engineSessionClosed(session browserEngineSession) bool {
	if session == nil {
		return true
	}
	status := session.Status()
	if status.State != "closed" || status.Ready || status.CanNavigate || session.Done() == nil {
		return false
	}
	select {
	case <-session.Done():
		return true
	default:
		return false
	}
}
func (c *browserEngineController) finish(r *browserEngineRun) {
	// A ready Session or an unknown Job/guard owner still occupies the slot.
	// Done alone is insufficient: closed_filters_unknown must not be forgotten.
	select {
	case <-r.done:
	default:
		return
	}
	r.mu.Lock()
	session, closeErr, closePending := r.session, r.closeErr, r.closePending
	r.mu.Unlock()
	if closePending || closeErr != nil || !engineSessionClosed(session) {
		return
	}
	last := c.snapshot(r)
	c.mu.Lock()
	if c.current == r {
		c.current = nil
		c.last = last
	}
	c.mu.Unlock()
}
func (c *browserEngineController) begin(data, proxyJSON string) (*browserEngineRun, error) {
	request, err := parseBrowserEngineRequest(data)
	if err != nil {
		return nil, err
	}
	proxy, err := browserproxy.Decode(proxyJSON)
	if err != nil {
		return nil, errBrowserEngineConfig
	}
	if c.ctx == nil {
		return nil, errBrowserEngineRefused
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errBrowserEngineRefused
	}
	ctx, cancel := context.WithCancel(c.ctx)
	r := &browserEngineRun{ctx: ctx, cancel: cancel, id: hex.EncodeToString(nonce[:]), starting: true, done: make(chan struct{})}
	c.mu.Lock()
	if c.closed || c.current != nil || c.maintenance || ctx.Err() != nil {
		c.mu.Unlock()
		cancel()
		return nil, errBrowserEngineRefused
	}
	c.current = r
	c.mu.Unlock()
	go c.execute(r, request, proxy)
	return r, nil
}
func (c *browserEngineController) perform(r *browserEngineRun, request browserEngineProfileRequest, proxy browserproxy.Config) (BrowserEngineLaunchResult, error) {
	result := BrowserEngineLaunchResult{Code: "host_start_failed"}
	ctx := r.ctx
	if c.deps.manager == nil || c.deps.guard == nil || c.deps.start == nil {
		result.Code = "host_not_ready"
		return result, errBrowserEngineRefused
	}
	if ctx.Err() != nil {
		return result, errBrowserEngineCancelled
	}
	guard, guardErr := c.deps.guard()
	if guardErr != nil || guard == nil {
		result.Code = "helper_required"
		result.Status.Diagnostic = browserengineguard.DiagnosticFromError(guardErr)
		return result, errBrowserEngineRefused
	}
	if ctx.Err() != nil {
		return result, errBrowserEngineCancelled
	}
	manager, managerErr := c.deps.manager(proxy)
	if managerErr != nil {
		result.Code = "version_store_unavailable"
		return result, errBrowserEngineRefused
	}
	if ctx.Err() != nil {
		return result, errBrowserEngineCancelled
	}
	config := request.hostConfig(proxy)
	session, startErr := c.deps.start(ctx, manager, config, guard)
	result.Status.Diagnostic = browserengineguard.DiagnosticFromError(startErr)
	if session != nil {
		r.mu.Lock()
		r.session = session
		r.mu.Unlock()
		if session.Done() != nil {
			go func() { <-session.Done(); <-r.done; r.cancel(); c.finish(r) }()
		}
	}
	if startErr != nil || session == nil || ctx.Err() != nil {
		if errors.Is(startErr, browserenginehost.ErrNotReady) {
			result.Code = "host_not_ready"
		}
		if errors.Is(startErr, browserenginehost.ErrGraphicsReadback) {
			result.Code = "graphics_readback_failed"
		}
		return result, errBrowserEngineRefused
	}
	status := session.Status()
	if !engineSessionID.MatchString(session.ID()) || status.SessionID != session.ID() || status.EngineID != request.EngineID || status.Version != request.Version || status.State != "ready" || !status.Ready || !status.CanNavigate {
		return result, errBrowserEngineRefused
	}
	return BrowserEngineLaunchResult{Status: status, CanLaunch: false, Code: "ready", Detail: "独立内核已返回本次会话状态；所选 UA、环境及受支持图形设置以本次真实读回证据为准。Firefox 图形使用原生偏好，Worker 执行保持阻断，未声称 Worker API 实测或硬件匿名。"}, nil
}
func (c *browserEngineController) execute(r *browserEngineRun, request browserEngineProfileRequest, proxy browserproxy.Config) {
	result, err := c.perform(r, request, proxy)
	// Cancellation and ready publication are serialized. A CancelStart that
	// wins this lock can never be followed by a successful launch response.
	r.mu.Lock()
	if err == nil && !r.stopped && r.ctx.Err() == nil {
		r.starting = false
		r.result = result
		r.mu.Unlock()
	} else {
		cancelled := r.cancelledStart || r.ctx.Err() != nil
		r.stopped = true
		r.cancelledStart = cancelled
		r.result = result
		r.mu.Unlock()
		r.cancel()
		_ = r.closeSession()
		r.mu.Lock()
		r.starting = false
		r.err = errBrowserEngineRefused
		if r.cancelledStart {
			r.err = errBrowserEngineCancelled
		}
		r.mu.Unlock()
	}
	close(r.done)
	c.finish(r)
}
func (c *browserEngineController) launch(data, proxyJSON string) (BrowserEngineLaunchResult, error) {
	r, err := c.begin(data, proxyJSON)
	if err != nil {
		return BrowserEngineLaunchResult{}, err
	}
	<-r.done
	r.mu.Lock()
	err = r.err
	r.mu.Unlock()
	return c.snapshot(r), err
}
func (c *browserEngineController) beginLaunch(data, proxyJSON string) (BrowserEngineLaunchResult, error) {
	r, err := c.begin(data, proxyJSON)
	if err != nil {
		return BrowserEngineLaunchResult{}, err
	}
	return c.snapshot(r), nil
}
func (c *browserEngineController) snapshot(r *browserEngineRun) BrowserEngineLaunchResult {
	r.mu.Lock()
	session, stopped, starting, cancelled, launchErr := r.session, r.stopped || r.ctx.Err() != nil, r.starting, r.cancelledStart, r.err
	closePending, closeErr := r.closePending, r.closeErr
	result := copyEngineLaunchResult(r.result)
	r.mu.Unlock()
	result.StartID = r.id
	result.CanCancelStart = starting && !stopped
	result.CanLaunch = false
	if session == nil {
		switch {
		case starting && stopped:
			result.Status.State, result.Detail = "cancelling", "已请求取消启动；等待下载、原生后端及清理完成，尚未声明退出。"
			result.Code = "cancellation_requested"
		case starting:
			result.Status.State, result.Detail = "starting", "准备中；代码/Job/WFP或运行时读回不完整时不会导航。"
			result.Code = "starting"
		case cancelled:
			result.Status.State, result.Detail = "start_cancelled", "本次启动已取消且启动后端已完成收尾；再次启动使用新的独立会话。"
			result.Code = "start_cancelled"
		default:
			result.Status.State, result.Detail = "start_failed", "本次启动未完成；启动后端已完成收尾，没有重试或直连回退。"
		}
		return result
	}
	status := session.Status()
	if result.Status.Diagnostic != nil {
		status.Diagnostic = result.Status.Diagnostic
	}
	if starting {
		// A returned Session is not yet a published launch. In particular, a
		// fast Close must not make cancellation terminal before Start's worker
		// has finished and released its ownership of partial resources.
		status.Ready, status.CanNavigate = false, false
		if status.State == "ready" || status.State == "closed" {
			status.State = "starting"
			result.Code = "starting"
			if stopped {
				status.State = "cancelling"
				result.Code = "cancellation_requested"
			}
		}
	}
	if stopped {
		status.Ready = false
		status.CanNavigate = false
		if status.State == "ready" {
			status.State = "closing_or_unknown"
		}
	}
	if !starting && status.State == "closed" && (closePending || closeErr != nil || !engineSessionClosed(session)) {
		status.State = "closing_or_unknown"
		status.Ready, status.CanNavigate = false, false
	}
	if !starting && !closePending && closeErr == nil && status.State == "closed" && engineSessionClosed(session) {
		if cancelled {
			status.State = "start_cancelled"
		} else if launchErr != nil {
			status.State = "start_failed"
		}
	}
	result.Status = status
	switch status.State {
	case "start_cancelled":
		result.Code = "start_cancelled"
	case "closing", "closing_or_unknown", "closed_filters_unknown":
		result.Code = "cleanup_unconfirmed"
	case "closed":
		result.Code = "closed"
	}
	result.Detail = "当前独立内核会话状态；未知进程树或防火墙收尾会继续保留本次启动，未修改VPN。"
	return copyEngineLaunchResult(result)
}

func copyEngineLaunchResult(result BrowserEngineLaunchResult) BrowserEngineLaunchResult {
	if browserengineguard.ValidDiagnostic(result.Status.Diagnostic) {
		copy := *result.Status.Diagnostic
		result.Status.Diagnostic = &copy
	} else {
		result.Status.Diagnostic = nil
	}
	if result.Status.Graphics != nil {
		copy := *result.Status.Graphics
		result.Status.Graphics = &copy
	}
	return result
}
func (c *browserEngineController) status() BrowserEngineLaunchResult {
	c.mu.Lock()
	r, last := c.current, c.last
	c.mu.Unlock()
	if r != nil {
		return c.snapshot(r)
	}
	if last.StartID != "" {
		return copyEngineLaunchResult(last)
	}
	return BrowserEngineLaunchResult{Status: browserenginehost.Status{State: "closed"}, Detail: "没有本界面仍在跟踪的引擎会话；未修改VPN。"}
}
func (c *browserEngineController) cancelStart(id string) (BrowserEngineLaunchResult, error) {
	if !engineSessionID.MatchString(id) {
		return BrowserEngineLaunchResult{}, errBrowserEngineRefused
	}
	c.mu.Lock()
	r, last := c.current, c.last
	c.mu.Unlock()
	if r == nil {
		if last.StartID == id && last.Status.State == "start_cancelled" {
			return copyEngineLaunchResult(last), nil
		}
		return BrowserEngineLaunchResult{}, errBrowserEngineRefused
	}
	r.mu.Lock()
	if r.id != id || !r.starting && !r.cancelledStart {
		r.mu.Unlock()
		return BrowserEngineLaunchResult{}, errBrowserEngineRefused
	}
	first := !r.stopped
	r.stopped, r.cancelledStart = true, true
	r.mu.Unlock()
	r.cancel()
	if first {
		// Nonblocking even if a returned Session needs a bounded Job shutdown.
		// closeOnce also covers the startup goroutine's late-session cleanup.
		go func() { _ = r.closeSession(); c.finish(r) }()
	}
	return c.snapshot(r), nil
}
func (c *browserEngineController) navigate(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || len(rawURL) == 0 || len(rawURL) > 4096 || u.Scheme != "https" && u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.ContainsAny(rawURL, "\r\n\x00") {
		return errBrowserEngineConfig
	}
	c.mu.Lock()
	r := c.current
	c.mu.Unlock()
	if r == nil {
		return errBrowserEngineRefused
	}
	r.mu.Lock()
	session, stopped := r.session, r.stopped
	r.mu.Unlock()
	if session == nil || stopped || r.ctx.Err() != nil {
		return errBrowserEngineRefused
	}
	status := session.Status()
	if !status.Ready || !status.CanNavigate {
		return errBrowserEngineRefused
	}
	if session.Navigate(r.ctx, rawURL) != nil {
		_ = r.stop()
		c.finish(r)
		return errBrowserEngineRefused
	}
	return nil
}
func (c *browserEngineController) close(permanent bool) error {
	c.mu.Lock()
	if permanent {
		c.closed = true
	}
	r := c.current
	c.mu.Unlock()
	if r != nil {
		err := r.stop()
		c.finish(r)
		return err
	}
	return nil
}

func (a *App) browserEngineController() (*browserEngineController, error) {
	a.browserEngineMu.Lock()
	defer a.browserEngineMu.Unlock()
	if a.browserEngineClosed || a.ctx == nil || a.ctx.Err() != nil {
		return nil, errBrowserEngineRefused
	}
	if a.browserEngine == nil {
		a.browserEngine = newBrowserEngineController(a.ctx, nativeBrowserEngineDependencies(a))
	}
	return a.browserEngine, nil
}
func (a *App) BrowserEngineProfileLaunch(configJSON, proxyJSON string) (BrowserEngineLaunchResult, error) {
	a.invalidatePrivacyEvidence()
	c, err := a.browserEngineController()
	if err != nil {
		return BrowserEngineLaunchResult{}, err
	}
	return c.launch(configJSON, proxyJSON)
}
func (a *App) BrowserEngineProfileBeginLaunch(configJSON, proxyJSON string) (BrowserEngineLaunchResult, error) {
	a.invalidatePrivacyEvidence()
	c, err := a.browserEngineController()
	if err != nil {
		return BrowserEngineLaunchResult{}, err
	}
	return c.beginLaunch(configJSON, proxyJSON)
}
func (a *App) BrowserEngineProfileCancelStart(startID string) (BrowserEngineLaunchResult, error) {
	a.invalidatePrivacyEvidence()
	a.browserEngineMu.Lock()
	c := a.browserEngine
	a.browserEngineMu.Unlock()
	if c == nil {
		return BrowserEngineLaunchResult{}, errBrowserEngineRefused
	}
	return c.cancelStart(startID)
}
func (a *App) BrowserEngineProfileStatus() BrowserEngineLaunchResult {
	a.browserEngineMu.Lock()
	c := a.browserEngine
	a.browserEngineMu.Unlock()
	if c == nil {
		return BrowserEngineLaunchResult{Status: browserenginehost.Status{State: "closed"}}
	}
	return c.status()
}
func (a *App) BrowserEngineProfileNavigate(rawURL string) error {
	a.browserEngineMu.Lock()
	c := a.browserEngine
	a.browserEngineMu.Unlock()
	if c == nil {
		return errBrowserEngineRefused
	}
	return c.navigate(rawURL)
}
func (a *App) BrowserEngineProfileClose() error {
	a.invalidatePrivacyEvidence()
	a.browserEngineMu.Lock()
	c := a.browserEngine
	a.browserEngineMu.Unlock()
	if c != nil {
		return c.close(false)
	}
	return nil
}
func (a *App) shutdownBrowserEngine() {
	a.browserEngineMu.Lock()
	a.browserEngineClosed = true
	c := a.browserEngine
	a.browserEngineMu.Unlock()
	if c != nil {
		_ = c.close(true)
	}
}
