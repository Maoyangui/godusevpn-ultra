package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	stdRuntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Maoyangui/godusevpn/internal/autostart"
	"github.com/Maoyangui/godusevpn/internal/buildinfo"
	"github.com/Maoyangui/godusevpn/internal/clash"
	"github.com/Maoyangui/godusevpn/internal/identity"
	"github.com/Maoyangui/godusevpn/internal/ipc"
	"github.com/Maoyangui/godusevpn/internal/paths"
	"github.com/Maoyangui/godusevpn/internal/privacy"
	"github.com/Maoyangui/godusevpn/internal/settings"
	"github.com/Maoyangui/godusevpn/internal/svc"
	"github.com/Maoyangui/godusevpn/internal/timeprivacy"
	"github.com/Maoyangui/godusevpn/internal/trial"
	"github.com/Maoyangui/godusevpn/internal/update"
)

const testURL = "http://www.gstatic.com/generate_204"

// UIState 推给页面与托盘的一份快照:服务在不在、服务给的状态、实时速度、界面偏好。
type UIState struct {
	Service  bool   `json:"service"`  // 控制管道可达
	SvcState string `json:"svcState"` // SCM state, no-permission, or identity-unknown (untrusted control peer)
	// SvcHint no-permission 时提示的种类(页面按它取译文):register = Windows 登记账户,admin = macOS 要管理员组,
	// root = Linux 要 root。CanRegister = 界面能替用户登记(Windows)
	// identity-unknown uses identity (no request sent) or response (result unknown).
	SvcHint     string          `json:"svcHint,omitempty"`
	CanRegister bool            `json:"canRegister,omitempty"`
	View        ipc.StateView   `json:"view"`
	Up          int64           `json:"up"`        // 字节/秒
	Down        int64           `json:"down"`      // 字节/秒
	TotalUp     int64           `json:"totalUp"`   // 本次连接累计上行(内核停了归零)
	TotalDown   int64           `json:"totalDown"` // 本次连接累计下行
	Lang        string          `json:"lang"`
	Theme       string          `json:"theme"` // system / light / dark
	Version     string          `json:"version"`
	Platform    string          `json:"platform"`
	Update      *update.Release `json:"update,omitempty"` // 有新版本时带上
}

// uiPrefs 托盘客户端自己的偏好(和服务的设置分开,放当前用户目录)。
type uiPrefs struct {
	Lang  string `json:"lang"`
	Theme string `json:"theme"`
}

type App struct {
	registerMu                   sync.Mutex // 「登记本账户」在途时不再叠一个 UAC
	ctx                          context.Context
	lastCheck                    time.Time // 最近一次检查更新
	lastStatus                   string    // 上次看到的连接状态,连上那一刻顺手查一次更新
	mu                           sync.Mutex
	minimized                    bool
	pendingLink                  string
	domReadyCh                   chan struct{}
	state                        UIState
	prefs                        uiPrefs
	clashKey                     string // port+secret,变了就重连速度流
	trafficStop                  context.CancelFunc
	tray                         *trayUI
	updating                     bool
	privacyMu                    sync.Mutex
	privacyUser                  *privacy.Manager
	privacyHost                  privacyHelper
	privacyHostError             string
	privacyBrowserGuard          *PrivacyBrowserGuardState
	privacyTime                  *timeprivacy.Manager
	metadata                     *metadataController
	privacyHostState             string
	privacyHostEpoch             uint64
	privacySnapshot              *privacyDetectionSnapshot
	privacyExport                *privacyExportSnapshot
	privacyEvidenceEpoch         uint64
	browserMu                    sync.Mutex
	browser                      *browserController
	browserClosed                bool
	clipboardMu                  sync.Mutex
	clipboard                    *clipboardBridge
	clipboardClosed              bool
	browserEngineMu              sync.Mutex
	browserEngine                *browserEngineController
	browserEngineClosed          bool
	browserProfilesMu            sync.Mutex
	browserProfiles              browserProfilesBackend
	browserProfilesDone          bool
	locationPortalMu             sync.Mutex
	locationPortal               locationPortalBackend
	locationPortalClosed         bool
	locationObserveMu            sync.Mutex
	locationObserveCancelPending bool
	locationObserveActive        bool
}

func newApp(minimized bool, link string) *App {
	a := &App{minimized: minimized, pendingLink: link, domReadyCh: make(chan struct{})}
	a.prefs = loadPrefs()
	a.state = UIState{Lang: a.prefs.Lang, Theme: a.prefs.Theme, Version: buildinfo.Version, SvcState: "unknown", Platform: stdRuntime.GOOS}
	return a
}

// prefsPath 界面偏好(语言、主题)放当前用户目录下,不需要管理员权限;各平台的位置见 platform_*.go。
func prefsPath() string { return filepath.Join(userConfigDir(), paths.AppName, "ui.json") }

func loadPrefs() uiPrefs {
	p := uiPrefs{Lang: "zh", Theme: "system"}
	if b, err := os.ReadFile(prefsPath()); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	if p.Lang != "en" {
		p.Lang = "zh"
	}
	if p.Theme != "light" && p.Theme != "dark" {
		p.Theme = "system"
	}
	return p
}

func (a *App) savePrefs() {
	_ = os.MkdirAll(filepath.Dir(prefsPath()), 0o700)
	b, _ := json.Marshal(a.prefs)
	_ = os.WriteFile(prefsPath(), b, 0o600)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.tray = newTray(a)
	go a.tray.run()
	// 服务被上次"退出"停掉了:登录后由托盘拉起(安装时已放开普通用户启停权限)
	if st := svc.QueryStatus(); st == "stopped" {
		if err := svc.StartUser(); err != nil {
			a.logf("启动服务失败: %v", err)
		}
	}
	go a.loop()
	go a.updateLoop()
}

func (a *App) domReady(context.Context) {
	select {
	case <-a.domReadyCh:
	default:
		close(a.domReadyCh)
	}
	if a.pendingLink != "" {
		link := a.pendingLink
		a.pendingLink = ""
		go a.handleDeepLink(link)
	}
}

func (a *App) shutdown(context.Context) {
	a.shutdownLocationPortal()
	a.shutdownBrowserProfiles()
	a.shutdownBrowserEngine()
	a.shutdownClipboard()
	a.shutdownBrowser()
	a.privacyMu.Lock()
	host := a.privacyHost
	a.privacyHost = nil
	a.privacyHostEpoch++
	a.privacyHostState = "closed"
	a.privacySnapshot, a.privacyExport = nil, nil
	a.privacyMu.Unlock()
	if host != nil {
		host.Close()
	}
	a.mu.Lock()
	stop := a.trafficStop
	a.mu.Unlock()
	if stop != nil {
		stop()
	}
	a.tray.quit()
}

func (a *App) logf(format string, v ...any) {
	runtime.LogInfof(a.ctx, format, v...)
}

// secondInstance 第二个实例启动了(双击图标或点了 godusevpn:// 链接):把窗口带到前台,有链接就导入。
func (a *App) secondInstance(link string) {
	a.showWindow()
	if link != "" {
		go a.handleDeepLink(link)
	}
}

// handleDeepLink godusevpn://import?url=<订阅地址>[&name=<订阅名>]
func (a *App) handleDeepLink(link string) {
	u, err := url.Parse(link)
	if err != nil || !strings.EqualFold(u.Host, "import") {
		return
	}
	sub := u.Query().Get("url")
	if sub == "" {
		return
	}
	<-a.domReadyCh
	a.showWindow()
	runtime.EventsEmit(a.ctx, "import", map[string]string{"url": sub, "name": u.Query().Get("name")})
}

// ---- 轮询服务、维护速度流 ----

func (a *App) loop() {
	t := time.NewTicker(1500 * time.Millisecond)
	defer t.Stop()
	for {
		a.refresh()
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *App) refresh() {
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()
	var view ipc.StateView
	err := ipc.Call(ctx, ipc.MGetState, nil, &view)
	scmState := svc.QueryStatus()
	a.mu.Lock()
	a.state = serviceReadState(a.state, view, err, scmState)
	// 服务管理器明确说停了 / 没装时不算"没权限":macOS / Linux 上服务崩了会留下 socket 文件,
	// 普通用户连它照样拿到 EACCES,那种情况该显示"服务没有运行 + 修复",而不是把修复拦下来。
	if errors.Is(err, ipc.ErrNoPermission) && a.state.SvcState != "stopped" && a.state.SvcState != "not-installed" {
		// 服务在跑、控制口也在,只是当前账户没权限连:Windows 上是不在控制名单里(同一台电脑的第二个账户),
		// macOS 上是 socket 只对管理员组开放。服务管理器视角它是 running,页面若一边说"服务运行中"、
		// 一边照 service=false 挂出"服务没有运行 + 修复",用户只会去点「修复」—— 而修复走的是 uninstall,
		// 会把主账户正在用的闸撤掉、网卡 IPv6 还原。这里单独标出来,页面、托盘、RepairService 都据此分支。
		a.state.SvcState = "no-permission"
		a.state.SvcHint, a.state.CanRegister = noPermissionHint()
	}
	a.state.Lang, a.state.Theme = a.prefs.Lang, a.prefs.Theme
	st := a.state
	justConnected := st.View.State.Status == "connected" && a.lastStatus != "connected"
	a.lastStatus = string(st.View.State.Status)
	a.mu.Unlock()
	a.manageTraffic(st)
	a.tray.update(st)
	if justConnected {
		go a.pokeUpdate(false)
	}
	runtime.EventsEmit(a.ctx, "state", st)
}

// manageTraffic 内核在跑就保持一条速度流;停了就断开并把速度归零。
func (a *App) manageTraffic(st UIState) {
	running := st.Service && (st.View.State.Status == "connected" || st.View.State.Status == "degraded")
	if !running {
		a.mu.Lock()
		stop := a.trafficStop
		a.trafficStop, a.clashKey = nil, ""
		if st.SvcState != serviceIdentityUnknown {
			a.state.TotalUp, a.state.TotalDown = 0, 0 // confirmed ordinary disconnected state
		}
		a.mu.Unlock()
		if stop != nil {
			stop()
		}
		return
	}
	var info ipc.ClashInfo
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	err := ipc.Call(ctx, ipc.MGetClashInfo, nil, &info)
	cancel()
	if err != nil || !info.Running {
		return
	}
	key := fmt.Sprintf("%d:%s", info.Port, info.Secret)
	a.mu.Lock()
	if a.clashKey == key && a.trafficStop != nil {
		a.mu.Unlock()
		return
	}
	if a.trafficStop != nil {
		a.trafficStop()
	}
	sctx, stop := context.WithCancel(a.ctx)
	a.trafficStop, a.clashKey = stop, key
	a.mu.Unlock()
	go func() {
		c := clash.New(info.Port, info.Secret)
		for sctx.Err() == nil {
			_ = c.Traffic(sctx, func(up, down int64) {
				// 内核的速度流每秒一条,给的是这一秒的字节数,累加起来就是本次连接的总量。
				// 累计值放在壳里而不是页面里:页面刷新、切到托盘再回来都不会把数字清掉。
				a.mu.Lock()
				if a.state.SvcState == serviceIdentityUnknown {
					a.mu.Unlock()
					return // stopped observer may still deliver its final callback
				}
				a.state.Up, a.state.Down = up, down
				a.state.TotalUp, a.state.TotalDown = a.state.TotalUp+up, a.state.TotalDown+down
				tu, td := a.state.TotalUp, a.state.TotalDown
				a.mu.Unlock()
				runtime.EventsEmit(a.ctx, "traffic", map[string]int64{"up": up, "down": down, "totalUp": tu, "totalDown": td})
			})
			if sctx.Err() == nil {
				time.Sleep(2 * time.Second)
			}
		}
	}()
}

func (a *App) clashClient() (*clash.Client, error) {
	var info ipc.ClashInfo
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()
	if err := ipc.Call(ctx, ipc.MGetClashInfo, nil, &info); err != nil {
		return nil, err
	}
	if !info.Running {
		return nil, errors.New("内核未运行")
	}
	return clash.New(info.Port, info.Secret), nil
}

func (a *App) call(method string, params, result any) error {
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	err := ipc.Call(ctx, method, params, result)
	if errors.Is(err, ipc.ErrUntrustedService) || errors.Is(err, ipc.ErrResponseMismatch) {
		a.markServiceUnconfirmed(err)
		return err
	}
	if errors.Is(err, ipc.ErrNoService) {
		return serviceDownError{}
	}
	return err
}

// ---- 自更新 ----

func (a *App) updateLoop() {
	time.Sleep(20 * time.Second)
	for {
		a.pokeUpdate(true)
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

// pokeUpdate 查一次更新;force 为假时 15 分钟内只查一次(抽屉打开、连上网都会来戳)。结果有变就推一次状态。
func (a *App) pokeUpdate(force bool) {
	if identity.Ultra {
		return // An upstream installer must never replace Ultra or its recovery tools.
	}
	a.mu.Lock()
	if !force && time.Since(a.lastCheck) < 15*time.Minute {
		a.mu.Unlock()
		return
	}
	a.lastCheck = time.Now()
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	rel, err := a.checkUpdate(ctx)
	if err != nil {
		return
	}
	a.mu.Lock()
	old := a.state.Update
	changed := (rel == nil) != (old == nil) || (rel != nil && old != nil && rel.Version != old.Version)
	a.state.Update = rel
	a.mu.Unlock()
	if changed {
		a.refresh()
	}
}

// PokeUpdate 页面打开抽屉时戳一下(有节流)。
func (a *App) PokeUpdate() { go a.pokeUpdate(false) }

// checkUpdate 先直连 GitHub;不通而内核在跑时改走本机混合端口再试一次(TUN 关着的场合)。
func (a *App) checkUpdate(ctx context.Context) (*update.Release, error) {
	if identity.Ultra {
		return nil, errors.New("Ultra 尚未配置独立签名更新通道；为避免被原版覆盖，当前不下载或安装上游更新")
	}
	rel, err := update.Check(ctx, buildinfo.Version, true, nil)
	if err == nil {
		return rel, nil
	}
	st := a.GetState()
	port := st.View.Settings.MixedPort
	if st.Service && port > 0 && (st.View.State.Status == "connected" || st.View.State.Status == "degraded") {
		pu, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
		c := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
		if rel2, err2 := update.Check(ctx, buildinfo.Version, true, c); err2 == nil {
			return rel2, nil
		}
	}
	return nil, err
}

// CheckUpdate 手动检查,有就返回。
func (a *App) CheckUpdate() (*update.Release, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	rel, err := a.checkUpdate(ctx)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.state.Update = rel
	a.lastCheck = time.Now()
	a.mu.Unlock()
	return rel, nil
}

// ApplyUpdate 下载安装包(进度经 update-progress 事件推给页面),校验后以管理员身份静默安装并重新拉起。
func (a *App) ApplyUpdate() error {
	if identity.Ultra {
		return errors.New("Ultra 更新通道尚未配置，请使用 Ultra 的独立发行包")
	}
	a.mu.Lock()
	rel := a.state.Update
	if a.updating || rel == nil {
		a.mu.Unlock()
		return errors.New("没有可用的更新")
	}
	a.updating = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.updating = false; a.mu.Unlock() }()
	dir := filepath.Join(os.TempDir(), "godusevpn-update")
	path, err := update.Download(a.ctx, rel, dir, nil, func(done, total int64) {
		runtime.EventsEmit(a.ctx, "update-progress", map[string]int64{"done": done, "total": total})
	})
	if err != nil {
		return err
	}
	// 装法各平台不一样(Windows 静默跑安装包,macOS 解压覆盖再重启服务),见 platform_windows.go / platform_darwin.go
	return applyUpdatePackage(path)
}

// ---- 窗口 ----

func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}

// showAbout 托盘"升级到 vX"点进来:显示窗口并跳到关于页。
func (a *App) showAbout() {
	a.showWindow()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "nav", "about")
	}
}

func (a *App) toggleWindow() { a.showWindow() }

func (a *App) HideWindow() { runtime.WindowHide(a.ctx) }

func (a *App) Minimize() { runtime.WindowMinimise(a.ctx) }

// QuitApp 托盘"退出":断开连接(TUN 随之拆除)、停掉后台服务进程,再退出自己。
// 下次登录托盘自启时会把服务再拉起来;上次是手动断开的,不会自动连。
func (a *App) QuitApp() {
	quitServiceActions(a.GetState(), func() error {
		var v ipc.StateView
		return a.call(ipc.MDisconnect, nil, &v)
	}, func() {
		if err := svc.StopUser(); err != nil {
			a.logf("停止服务: %v", err)
		}
	}, func() {
		a.shutdown(a.ctx)
		runtime.Quit(a.ctx)
	})
}

// ---- 绑定给页面的方法(window.go.main.App.*) ----

func (a *App) GetState() UIState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

func (a *App) Connect() (ipc.StateView, error) {
	var v ipc.StateView
	err := a.call(ipc.MConnect, nil, &v)
	go a.refresh()
	return v, err
}

func (a *App) Disconnect() (ipc.StateView, error) {
	var v ipc.StateView
	err := a.call(ipc.MDisconnect, nil, &v)
	go a.refresh()
	return v, err
}

func (a *App) SetMode(mode string) (ipc.StateView, error) {
	var v ipc.StateView
	err := a.call(ipc.MSetMode, map[string]string{"mode": mode}, &v)
	go a.refresh()
	return v, err
}

func (a *App) SelectNode(tag string) (ipc.StateView, error) {
	var v ipc.StateView
	err := a.call(ipc.MSelectNode, map[string]string{"tag": tag}, &v)
	go a.refresh()
	return v, err
}

// NodeInfo 节点页一行。
type NodeInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Delay   int    `json:"delay"` // 0 = 未测;-1 = 上次失败
	Current bool   `json:"current"`
	AutoNow string `json:"autoNow,omitempty"` // auto 组当前落在哪个节点
}

// GetNodes 节点列表(带最近一次延迟);内核没跑时只给订阅里的名字。
func (a *App) GetNodes() ([]NodeInfo, error) {
	st := a.GetState()
	c, err := a.clashClient()
	if err != nil {
		out := []NodeInfo{}
		for _, n := range st.View.Nodes {
			out = append(out, NodeInfo{Name: n, Delay: st.View.Delays[n], Current: n == st.View.Node})
		}
		return out, nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	proxies, err := c.Proxies(ctx)
	if err != nil {
		return nil, err
	}
	group, ok := proxies["proxy"]
	if !ok {
		return nil, errors.New("内核里没有 proxy 组")
	}
	out := make([]NodeInfo, 0, len(group.All))
	for _, name := range group.All {
		p := proxies[name]
		ni := NodeInfo{Name: name, Type: p.Type, Delay: p.LastDelay(), Current: name == group.Now}
		if name == "auto" {
			ni.AutoNow = p.Now
		}
		out = append(out, ni)
	}
	return out, nil
}

// TestAll 给全部节点测一次延迟,返回 节点 → 毫秒。
func (a *App) TestAll() (map[string]int, error) {
	n := len(a.GetState().View.Nodes)
	if c, err := a.clashClient(); err == nil {
		ctx, cancel := context.WithTimeout(a.ctx, clash.DelayBudget(n, 10, 8*time.Second))
		defer cancel()
		return c.GroupDelay(ctx, "proxy", testURL, 8*time.Second)
	}
	// 内核没跑:让服务用订阅里的出站起临时实例测,结果进状态里,GetNodes 就能显示。
	// 时限和服务那边的 probeBudget 一样按节点数算,再放宽十秒,别在服务还在测的时候这头先超时
	ctx, cancel := context.WithTimeout(a.ctx, clash.DelayBudget(n, 8, 12*time.Second)+10*time.Second)
	defer cancel()
	var res map[string]int
	err := ipc.Call(ctx, ipc.MProbeNodes, nil, &res)
	go a.refresh()
	return res, err
}

func (a *App) TestLatency(tag string) (int, error) {
	var r struct {
		Ms int `json:"ms"`
	}
	err := a.call(ipc.MTestLatency, map[string]string{"tag": tag}, &r)
	return r.Ms, err
}

func (a *App) SetProfileURL(url string) (*ipc.ProfileView, error) {
	var p *ipc.ProfileView
	err := a.call(ipc.MSetProfileURL, map[string]string{"url": strings.TrimSpace(url)}, &p)
	go a.refresh()
	return p, err
}

// RefreshProfile 刷新一条订阅(id 为空 = 当前),返回全部订阅。
func (a *App) RefreshProfile(id string) ([]ipc.ProfileView, error) {
	var out []ipc.ProfileView
	err := a.call(ipc.MRefreshProfile, map[string]string{"id": id}, &out)
	go a.refresh()
	return out, err
}

func (a *App) GetProfiles() ([]ipc.ProfileView, error) {
	var out []ipc.ProfileView
	err := a.call(ipc.MGetProfiles, nil, &out)
	if out == nil {
		out = []ipc.ProfileView{}
	}
	return out, err
}

// AddProfile 新增订阅(服务会先拉一次,拉不到就不加);第一条会自动成为当前订阅。
func (a *App) AddProfile(name, url string) ([]ipc.ProfileView, error) {
	var out []ipc.ProfileView
	err := a.call(ipc.MAddProfile, map[string]string{"name": name, "url": strings.TrimSpace(url)}, &out)
	go a.refresh()
	return out, err
}

// UpsertTrialProfile 保存内置试用订阅:同一设备再次拿到新地址时替换原试用记录。
func (a *App) UpsertTrialProfile(name, url string) ([]ipc.ProfileView, error) {
	var out []ipc.ProfileView
	err := a.call(ipc.MUpsertTrialProfile, map[string]string{"name": name, "url": strings.TrimSpace(url)}, &out)
	go a.refresh()
	return out, err
}

func (a *App) RemoveProfile(id string) ([]ipc.ProfileView, error) {
	var out []ipc.ProfileView
	err := a.call(ipc.MRemoveProfile, map[string]string{"id": id}, &out)
	go a.refresh()
	return out, err
}

func (a *App) SelectProfile(id string) (ipc.StateView, error) {
	var v ipc.StateView
	err := a.call(ipc.MSelectProfile, map[string]string{"id": id}, &v)
	go a.refresh()
	return v, err
}

// UpdateProfile 改名或改地址(空字段表示不改)。
func (a *App) UpdateProfile(id, name, url string) ([]ipc.ProfileView, error) {
	var out []ipc.ProfileView
	err := a.call(ipc.MRenameProfile, map[string]string{"id": id, "name": name, "url": strings.TrimSpace(url)}, &out)
	go a.refresh()
	return out, err
}

// TrialStatus 读取后端试用状态。设备标识只保存在当前用户的应用配置目录，
// 不读取 MAC/序列号；后端负责按月限额，状态读取失败时页面保留购买入口。
func (a *App) TrialStatus() (trial.Status, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return trial.New(trial.DefaultEndpoint, trialDataDir()).Status(ctx)
}

// TrialClaim 申请本月免费试用并返回新的订阅链接。服务端返回的链接由客户端
// 复用 AddProfile 的 HTTPS 校验，不把后台管理员令牌放入客户端。
func (a *App) TrialClaim() (trial.ClaimResult, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return trial.New(trial.DefaultEndpoint, trialDataDir()).Claim(ctx)
}

// trialDataDir belongs to the interactive user. The service store under
// ProgramData is ACL-protected for SYSTEM/Administrators, while the Wails UI
// normally runs as a standard user. Keeping only the random install key in the
// per-user config directory lets that user claim a trial without weakening the
// service store ACLs.
func trialDataDir() string { return filepath.Join(userConfigDir(), paths.AppName) }

func (a *App) GetSettings() (settings.Settings, error) {
	var s settings.Settings
	err := a.call(ipc.MGetSettings, nil, &s)
	return s, err
}

func (a *App) SaveSettings(s settings.Settings) (settings.Settings, error) {
	var out settings.Settings
	err := a.call(ipc.MSetSettings, s, &out)
	go a.refresh()
	return out, err
}

func (a *App) GetLogs(lines int, core bool) ([]string, error) {
	var out []string
	err := a.call(ipc.MGetLogs, map[string]any{"lines": lines, "core": core}, &out)
	return out, err
}

func (a *App) Diagnose() (string, error) {
	var out map[string]any
	if err := a.call(ipc.MDiagnose, nil, &out); err != nil {
		return "", err
	}
	out["ui"] = map[string]any{"version": buildinfo.Version, "autostart": autostart.Enabled(), "svcState": svc.QueryStatus()}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b), nil
}

// ExportDiag is the compatibility entry point for a minimal diagnostic only.
// Detailed content requires an explicitly reviewed PreviewDiag snapshot.
func (a *App) ExportDiag() (string, error) {
	p, err := a.PreviewDiag(ipc.DiagnosticOptions{})
	if err != nil {
		return "", err
	}
	return a.ExportDiagPreview(p.ID, p.SHA256)
}

// ConnRow 连接页一行。
type ConnRow struct {
	ID    string `json:"id"`
	Host  string `json:"host"`
	Net   string `json:"net"`
	Chain string `json:"chain"`
	Rule  string `json:"rule"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Start string `json:"start"`
	App   string `json:"app"`
}

func (a *App) GetConnections() ([]ConnRow, error) {
	c, err := a.clashClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	cs, err := c.Connections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ConnRow, 0, len(cs.Connections))
	for _, x := range cs.Connections {
		host := x.Metadata.Host
		if host == "" {
			host = x.Metadata.DestinationIP
		}
		out = append(out, ConnRow{ID: x.ID, Host: host + ":" + x.Metadata.DestinationPort, Net: x.Metadata.Network,
			Chain: strings.Join(x.Chains, " → "), Rule: x.Rule, Up: x.Upload, Down: x.Download, Start: x.Start,
			App: filepath.Base(x.Metadata.ProcessPath)})
	}
	return out, nil
}

func (a *App) CloseConnection(id string) error {
	c, err := a.clashClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	return c.CloseConnection(ctx, id)
}

func (a *App) GetAutostart() bool { return autostart.Enabled() }

func (a *App) SetAutostart(on bool) error { return autostart.Set(on) }

func (a *App) SetLang(lang string) {
	if lang != "en" {
		lang = "zh"
	}
	a.mu.Lock()
	a.prefs.Lang, a.state.Lang = lang, lang
	a.mu.Unlock()
	a.savePrefs()
	a.tray.relabel()
	go a.refresh()
}

func (a *App) SetTheme(theme string) {
	if theme != "light" && theme != "dark" {
		theme = "system"
	}
	a.mu.Lock()
	a.prefs.Theme, a.state.Theme = theme, theme
	a.mu.Unlock()
	a.savePrefs()
}

// RepairService 以管理员身份重新注册并启动后台服务(弹一次 UAC)。
func (a *App) RepairService() error {
	if a.GetState().SvcState == serviceIdentityUnknown {
		return unconfirmedServiceError(a.GetState())
	}
	if st := a.GetState(); st.SvcState == "no-permission" {
		// 服务没坏,是这个账户没权限。「修复」= uninstall + install,会把别的账户正在用的闸撤掉、网卡 IPv6 还原,
		// 而且 install 见名单已有人就不会再登记本账户 —— 白拆一次隧道,问题原样。
		return errors.New("E_NO_PERMISSION: ")
	}
	// A previously readable snapshot may have become untrusted before this
	// click. Authenticate a fresh read before any SCM start/repair side effect.
	var current ipc.StateView
	readErr := a.call(ipc.MGetState, nil, &current)
	if errors.Is(readErr, ipc.ErrUntrustedService) || errors.Is(readErr, ipc.ErrResponseMismatch) || errors.Is(readErr, ipc.ErrNoPermission) {
		return readErr
	}
	if errors.Is(readErr, ipc.ErrNoService) {
		if currentSCM := svc.QueryStatus(); currentSCM != "stopped" && currentSCM != "not-installed" {
			a.markServiceUnconfirmed(ipc.ErrUntrustedService)
			return ipc.ErrUntrustedService
		}
	}
	// 「登记本账户」正在重启服务:这时横幅会短暂变成"服务没有运行 + 修复",别在这个空档里再动服务
	if !a.registerMu.TryLock() {
		return errors.New("E_REGISTER_BUSY: ")
	}
	a.registerMu.Unlock()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	svcExe, err := serviceBinary(filepath.Dir(exe))
	if err != nil {
		return err
	}
	if st := svc.QueryStatus(); st == "stopped" {
		if err := svc.StartUser(); err == nil {
			return nil
		}
	}
	// 不走 uninstall:那会撤闸、还原网卡 IPv6 —— 服务要是同一台电脑上别的账户在用,就是拆别人的保护。
	// repair = 停掉(停服务不撤闸)+ 按 install 重新注册并启动。
	return runElevated(svcExe, "repair")
}

// RegisterController 把当前 Windows 账户登记为控制用户并重启服务(提权跑 register-controller --restart)。
// 闸是持久的、第二代提供者不绑服务名,重启服务不撤闸;隧道按落盘的意愿自动重连。别的平台没有名单机制,直接报不支持。
func (a *App) RegisterController() error {
	if !a.GetState().CanRegister {
		return errors.New("E_NO_PERMISSION: ")
	}
	if !a.registerMu.TryLock() {
		return errors.New("E_REGISTER_BUSY: ")
	}
	defer a.registerMu.Unlock()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	svcExe, err := serviceBinary(filepath.Dir(exe))
	if err != nil {
		return err
	}
	// 等提权进程跑完、看它的退出码:0.7.4 只管把进程拉起来就报成功,重启服务失败、服务停在那里,界面也说"已登记"
	err = registerController(svcExe)
	go a.refresh()
	return err
}

// DismissNICLost 用户看过"有几张网卡的 IPv6 原值丢了"的提示,点了"知道了"。
func (a *App) DismissNICLost() error {
	err := a.call(ipc.MDismissNICLost, nil, nil)
	go a.refresh()
	return err
}

// ReadClipboard 读系统剪贴板文本,给"粘贴"按钮用;失败返回空串。
func (a *App) ReadClipboard() string {
	s, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return ""
	}
	return s
}

func (a *App) OpenURL(url string) {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		runtime.BrowserOpenURL(a.ctx, url)
	}
}

func (a *App) Version() map[string]string {
	return map[string]string{"ui": buildinfo.Version, "name": buildinfo.DisplayName}
}
