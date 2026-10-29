package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
	"github.com/Maoyangui/godusevpn/internal/detection"
	"github.com/Maoyangui/godusevpn/internal/identity"
	"github.com/Maoyangui/godusevpn/internal/ipc"
	"github.com/Maoyangui/godusevpn/internal/localstate"
	"github.com/Maoyangui/godusevpn/internal/privacy"
	"github.com/Maoyangui/godusevpn/internal/privacydata"
	"github.com/Maoyangui/godusevpn/internal/privacyhost"
	"github.com/Maoyangui/godusevpn/internal/timeprivacy"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type privacyHelper interface {
	Call(context.Context, string, any, any) error
	Close() error
}

var launchPrivacyHelper = func(ctx context.Context, exe string) (privacyHelper, error) {
	return privacyHelperFromClient(privacyhost.Start(ctx, exe))
}

// Check the concrete pointer before putting it in an interface: a rejected
// launch returns a nil *Client, which must remain nil for failure cleanup.
func privacyHelperFromClient(client *privacyhost.Client, err error) (privacyHelper, error) {
	if client == nil {
		return nil, err
	}
	return client, err
}

type PrivacyHelperState struct {
	State        string                    `json:"state"`
	Detail       string                    `json:"detail"`
	BrowserGuard *PrivacyBrowserGuardState `json:"browserGuard,omitempty"`
}

type PrivacyBrowserGuardState = browserengineguard.Capability

func (a *App) PrivacyHelperStatus() PrivacyHelperState {
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	state := a.privacyHostState
	if state == "" {
		state = "not_requested"
	}
	result := PrivacyHelperState{State: state, Detail: a.privacyHostError}
	if state == "connected" && a.privacyHost != nil && a.privacyBrowserGuard != nil {
		copy := *a.privacyBrowserGuard
		if copy.Diagnostic != nil {
			diagnostic := *copy.Diagnostic
			copy.Diagnostic = &diagnostic
		}
		result.BrowserGuard = &copy
	}
	return result
}

func (a *App) PrivacyAuthorize() (PrivacyHelperState, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	err := a.authorizePrivacyHost(ctx, true)
	return a.PrivacyHelperStatus(), err
}

// These methods belong to the user-interface plane. No daemon, state-machine,
// builder, core, DNS, guard or connect method calls them.
func (a *App) userPrivacy() *privacy.Manager {
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	if a.privacyUser == nil {
		a.privacyUser = privacy.New(filepath.Join(userConfigDir(), identity.AppName, "privacy-user"))
	}
	return a.privacyUser
}

func (a *App) startPrivacyHost(ctx context.Context) {
	_ = a.authorizePrivacyHost(ctx, false)
}

func (a *App) authorizePrivacyHost(ctx context.Context, explicit bool) error {
	if !identity.Ultra {
		return errors.New("此构建没有独立管理员助手")
	}
	for _, arg := range os.Args[1:] {
		if !explicit && arg == "--ultra-no-elevate" {
			a.privacyMu.Lock()
			a.privacyHostState = "not_requested"
			a.privacyHostError = "本次跳过管理员授权；原有隧道仍可使用"
			a.privacyMu.Unlock()
			return nil
		}
	}
	a.privacyMu.Lock()
	if a.privacyHostState == "authorizing" || a.privacyHostState == "closed" {
		a.privacyMu.Unlock()
		return errors.New("管理员授权正在进行或界面已关闭；没有重复请求")
	}
	// A helper object can remain non-nil after its session has been marked lost
	// or cancelled (for example, when a transport failure races the UI retry).
	// Treat that object as stale and close it before starting a fresh session;
	// otherwise the old pointer suppresses reauthorization while the state is no
	// longer connected.
	var stale privacyHelper
	if a.privacyHost != nil && a.privacyHostState == "connected" {
		a.privacyMu.Unlock()
		return nil
	}
	if a.privacyHost != nil {
		stale = a.privacyHost
		a.privacyHost = nil
		a.privacyHostEpoch++
		a.privacyEvidenceEpoch++
	}
	a.privacyHostState = "authorizing"
	a.privacyHostError = ""
	a.privacyBrowserGuard = nil
	a.privacyHostEpoch++
	a.privacyEvidenceEpoch++
	epoch := a.privacyHostEpoch
	a.privacySnapshot, a.privacyExport = nil, nil
	a.privacyMu.Unlock()
	if stale != nil {
		_ = stale.Close()
	}
	exe, err := os.Executable()
	var host privacyHelper
	if err == nil {
		host, err = launchPrivacyHelper(ctx, exe)
	}
	if err == nil && host == nil {
		err = errors.New("管理员助手未返回可用会话")
	}
	var browserCapability *PrivacyBrowserGuardState
	if err == nil && host != nil {
		if observer, ok := host.(interface {
			BrowserGuardCapability(context.Context) (browserengineguard.Capability, error)
		}); ok {
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			capability, observeErr := observer.BrowserGuardCapability(bounded)
			cancel()
			if observeErr != nil || !browserengineguard.ValidCapability(capability) {
				capability = browserengineguard.Capability{Diagnostic: &browserengineguard.Diagnostic{Stage: "guard_transport"}}
			}
			browserCapability = &capability
			if closed, ok := host.(interface{ Closed() bool }); ok && closed.Closed() {
				err = errors.New("管理员助手状态查询失联")
			}
		}
	}
	if err != nil && host != nil {
		_ = host.Close()
		host = nil
	}
	a.privacyMu.Lock()
	if ctx.Err() != nil || epoch != a.privacyHostEpoch || a.privacyHostState == "closed" {
		if epoch == a.privacyHostEpoch && a.privacyHostState != "closed" {
			a.privacyHostState = "unavailable"
			a.privacyHostError = "授权上下文已取消；可明确重新请求。"
			a.privacyEvidenceEpoch++
			a.privacySnapshot, a.privacyExport = nil, nil
		}
		a.privacyMu.Unlock()
		if host != nil {
			host.Close()
		}
		return errors.New("管理员授权上下文已失效；未继续操作")
	}
	a.privacyHost = host
	a.privacyBrowserGuard = browserCapability
	a.privacyEvidenceEpoch++
	a.privacySnapshot, a.privacyExport = nil, nil
	if err != nil {
		a.privacyHostState = "cancelled_or_unavailable"
		if errors.Is(err, syscall.Errno(1223)) {
			a.privacyHostState = "cancelled"
		}
		a.privacyHostError = "管理员授权未完成；可明确重新授权。没有自动重试写入，隧道不受影响。"
	} else {
		a.privacyHostState = "connected"
		a.privacyHostError = ""
	}
	a.privacyMu.Unlock()
	if err != nil {
		return errors.New("管理员授权未完成；请查看助手状态并明确重试，原隧道不受影响")
	}
	return nil
}

func (a *App) hostCall(method string, params, out any) error {
	a.privacyMu.Lock()
	h := a.privacyHost
	a.privacyMu.Unlock()
	if h == nil {
		return errors.New("管理员执行助手尚未就绪或授权已取消；此项未修改，隧道功能不受影响")
	}
	base := a.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 30*time.Second)
	defer cancel()
	err := h.Call(ctx, method, params, out)
	if err == nil {
		return nil
	}
	// A failed mutation is never retried. A bounded read distinguishes a policy
	// rejection from a dead transport before offering explicit reauthorization.
	lost := method == "Report"
	if !lost && base.Err() == nil {
		probeCtx, stop := context.WithTimeout(base, 5*time.Second)
		var probe privacy.Report
		lost = h.Call(probeCtx, "Report", nil, &probe) != nil
		stop()
	}
	if lost {
		a.privacyMu.Lock()
		if a.privacyHost == h {
			a.privacyHost = nil
			a.privacyHostState = "lost"
			a.privacyHostError = "管理员助手失联；已提交操作的结果可能未知。重新授权后先检查现状，不自动重试。"
			a.privacyEvidenceEpoch++
			a.privacySnapshot, a.privacyExport = nil, nil
		}
		a.privacyMu.Unlock()
		_ = h.Close()
	}
	return err
}

func (a *App) PrivacyReport() (privacy.Report, error) {
	r, err := a.userPrivacy().Report()
	if err != nil {
		return r, err
	}
	var machine privacy.Report
	if e := a.hostCall("Report", nil, &machine); e == nil {
		r.Admin = machine.Admin
		r.BackupHealthy = r.BackupHealthy && machine.BackupHealthy
		for i, c := range r.Controls {
			if !c.RequiresAdmin {
				continue
			}
			for _, m := range machine.Controls {
				if c.ID == m.ID {
					r.Controls[i] = m
					break
				}
			}
		}
		for _, w := range machine.Warnings {
			if !containsString(r.Warnings, w) {
				r.Warnings = append(r.Warnings, w)
			}
		}
	} else {
		r.Admin = false
		for i := range r.Controls {
			if r.Controls[i].RequiresAdmin {
				r.Controls[i].CanApply = false
				r.Controls[i].Detail += " 管理员助手尚不可用，只提供当前只读证据和系统引导。"
			}
		}
		r.Warnings = append(r.Warnings, "管理员助手不可用；当前用户浏览器策略仍可单独处理，机器策略只能检查或引导。")
	}
	return r, nil
}

// BrowserHostReport is a read-only inventory of browser runtimes that the
// Windows client could host. It never launches a browser, downloads a runtime,
// changes policy, or treats the Wails UI WebView as a private browser profile.
func (a *App) BrowserHostReport() browserhost.Report { return browserhost.Read() }

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func (a *App) machinePolicy(id string) (bool, error) {
	r, err := a.userPrivacy().Report()
	if err != nil {
		return false, err
	}
	for _, c := range r.Controls {
		if c.ID == id {
			return c.RequiresAdmin, nil
		}
	}
	return false, errors.New("未知隐私项目；只允许内置白名单")
}

func (a *App) PrivacyApply(id string) (privacy.ActionResult, error) {
	a.invalidatePrivacyEvidence()
	machine, err := a.machinePolicy(id)
	if err != nil {
		return privacy.ActionResult{}, err
	}
	if !machine {
		return a.userPrivacy().Apply(id)
	}
	var result privacy.ActionResult
	err = a.hostCall("Apply", id, &result)
	return result, err
}

func (a *App) PrivacyRestore(id string) (privacy.ActionResult, error) {
	a.invalidatePrivacyEvidence()
	machine, err := a.machinePolicy(id)
	if err != nil {
		return privacy.ActionResult{}, err
	}
	if !machine {
		return a.userPrivacy().Restore(id)
	}
	var result privacy.ActionResult
	err = a.hostCall("Restore", id, &result)
	return result, err
}

func (a *App) PrivacyRestoreAll() ([]privacy.ActionResult, error) {
	a.invalidatePrivacyEvidence()
	results, userErr := a.userPrivacy().RestoreAll()
	var machine []privacy.ActionResult
	machineErr := a.hostCall("RestoreAll", nil, &machine)
	return append(results, machine...), errors.Join(userErr, machineErr)
}

func (a *App) PrivacyFirewallReport() (appfirewall.Report, error) {
	var r appfirewall.Report
	if err := a.hostCall("FirewallReport", nil, &r); err == nil {
		return r, nil
	}
	r, err := appfirewall.New("").Report()
	r.Warnings = append(r.Warnings, "没有可用的管理员执行助手；不会尝试安装或删除规则。")
	return r, err
}

func (a *App) PrivacyFirewallApply(rule appfirewall.Rule) (appfirewall.Result, error) {
	a.invalidatePrivacyEvidence()
	var r appfirewall.Result
	err := a.hostCall("FirewallApply", rule, &r)
	return r, err
}
func (a *App) PrivacyFirewallRemove(id string) (appfirewall.Result, error) {
	a.invalidatePrivacyEvidence()
	var r appfirewall.Result
	err := a.hostCall("FirewallRemove", id, &r)
	return r, err
}

func (a *App) PrivacyChooseProgram() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择要限制的本地 Windows 程序", Filters: []runtime.FileFilter{{DisplayName: "Windows 程序 (*.exe)", Pattern: "*.exe"}}})
}

func (a *App) PrivacyDetection() (detection.Report, error) {
	p, err := a.PrivacyReport()
	if err != nil {
		return detection.Report{}, err
	}
	policyAt := time.Now().UTC().Format(time.RFC3339Nano)
	f, fe := a.PrivacyFirewallReport()
	firewallAt := ""
	if fe == nil {
		firewallAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if fe != nil {
		f.Warnings = append(f.Warnings, "无法核对 WFP 规则，不能确认规则存在。")
	}
	network, networkAt := readPrivacyNetwork(a.ctx)
	facts := detection.ReadFacts()
	in := detection.Input{Policies: p, Firewall: f, Facts: facts, ExecutionMode: "desktop_ui", HelperAuthorized: p.Admin, Network: network, SourceTimes: map[string]string{"policies": policyAt, "firewall": firewallAt, "network": networkAt, "facts": time.Now().UTC().Format(time.RFC3339Nano)}}
	r := detection.Build(in)
	attachOwnedBrowserObservation(&r, a)
	if fe != nil {
		r.Warnings = append(r.Warnings, "应用规则核对失败，当前状态未知；没有为此改变任何网络设置。")
	}
	return r, nil
}

var privacyServiceState = func(ctx context.Context) (ipc.StateView, error) {
	var v ipc.StateView
	err := ipc.Call(ctx, ipc.MGetState, nil, &v)
	return v, err
}

func readPrivacyNetwork(base context.Context) (detection.Network, string) {
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 3*time.Second)
	defer cancel()
	v, err := privacyServiceState(ctx)
	if err != nil {
		return detection.Network{}, ""
	}
	n := detection.Network{Available: true, Status: string(v.State.Status), Mode: v.Mode, Guard: v.Guard, GuardError: v.GuardError != "", IPv6: v.Settings.IPv6, LANBypass: v.Settings.LANBypass, NoDirect: v.Settings.NoDirect}
	if v.Settings.Schema > 0 {
		n.SettingsAvailable = true
		if v.Settings.LocalDNS == "system" {
			n.LocalDNSMode = "system"
		} else if v.Settings.LocalDNS != "" {
			n.LocalDNSMode = "doh"
		}
		if v.Settings.RemoteDNS != "" {
			n.RemoteDNSMode = "doh"
		}
		n.LogLevel = v.Settings.LogLevel
		days := v.Settings.LogDays
		n.LogDays = &days
	}
	return n, time.Now().UTC().Format(time.RFC3339Nano)
}

const privacySnapshotLifetime = 10 * time.Minute

type privacyDetectionSnapshot struct {
	id      string
	report  detection.Report
	expires time.Time
}
type privacyExportSnapshot struct {
	id       string
	sourceID string
	content  []byte
	expires  time.Time
}
type PrivacyDetectionView struct {
	SnapshotID string           `json:"snapshotId"`
	ExpiresAt  string           `json:"expiresAt"`
	Report     detection.Report `json:"report"`
}
type PrivacyExportSelection struct {
	SnapshotID         string   `json:"snapshotId"`
	Scope              string   `json:"scope"`
	FindingIDs         []string `json:"findingIds"`
	IncludeEnvironment bool     `json:"includeEnvironment"`
	IncludeObserver    bool     `json:"includeObserver"`
}
type PrivacyExportPreview struct {
	PreviewID string `json:"previewId"`
	Content   string `json:"content"`
	ExpiresAt string `json:"expiresAt"`
	Count     int    `json:"count"`
}

func privacyToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func (a *App) invalidatePrivacyEvidence() {
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	a.privacyEvidenceEpoch++
	a.privacySnapshot, a.privacyExport = nil, nil
}
func (a *App) PrivacyDetectionSnapshot() (PrivacyDetectionView, error) {
	a.privacyMu.Lock()
	epoch := a.privacyEvidenceEpoch
	a.privacyMu.Unlock()
	r, err := a.PrivacyDetection()
	if err != nil {
		return PrivacyDetectionView{}, err
	}
	id, err := privacyToken()
	if err != nil {
		return PrivacyDetectionView{}, err
	}
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	if epoch != a.privacyEvidenceEpoch {
		return PrivacyDetectionView{}, errors.New("检测期间发生隐私操作；本次结果未视为最新，请手动重新检测")
	}
	s := &privacyDetectionSnapshot{id, r, time.Now().Add(privacySnapshotLifetime)}
	a.privacySnapshot, a.privacyExport = s, nil
	return PrivacyDetectionView{id, s.expires.UTC().Format(time.RFC3339), r}, nil
}
func (a *App) PrivacyPreviewDetectionExport(selection PrivacyExportSelection) (PrivacyExportPreview, error) {
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	s := a.privacySnapshot
	if s == nil || s.id != selection.SnapshotID || time.Now().After(s.expires) {
		return PrivacyExportPreview{}, errors.New("检测快照已过期或失效；请重新检测并审阅")
	}
	if selection.Scope != "all" && selection.Scope != "filtered" {
		return PrivacyExportPreview{}, errors.New("请选择完整或当前筛选范围")
	}
	allowed := map[string]bool{}
	for _, f := range s.report.Findings {
		allowed[f.ID] = true
	}
	wanted := map[string]bool{}
	for _, id := range selection.FindingIDs {
		if !allowed[id] {
			return PrivacyExportPreview{}, errors.New("筛选项不属于当前快照")
		}
		wanted[id] = true
	}
	findings := []detection.Finding{}
	for _, f := range s.report.Findings {
		if selection.Scope == "filtered" && !wanted[f.ID] {
			continue
		}
		if !selection.IncludeEnvironment && (f.ID == "timezone" || f.ID == "locale") {
			continue
		}
		findings = append(findings, f)
	}
	doc := map[string]any{"schema": "godusevpn-ultra-privacy-export-v1", "generatedAt": s.report.GeneratedAt, "scope": selection.Scope, "findings": findings, "warnings": s.report.Warnings}
	if selection.IncludeObserver {
		doc["observer"], doc["platform"], doc["observerElevated"], doc["helperAuthorized"] = s.report.Observer, s.report.Platform, s.report.Admin, s.report.HelperAuthorized
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return PrivacyExportPreview{}, err
	}
	id, err := privacyToken()
	if err != nil {
		return PrivacyExportPreview{}, err
	}
	a.privacyExport = &privacyExportSnapshot{id, s.id, b, s.expires}
	return PrivacyExportPreview{id, string(b), s.expires.UTC().Format(time.RFC3339), len(findings)}, nil
}

var choosePrivacyExportDestination = func(ctx context.Context) (string, error) {
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: "保存已预览的检测报告（不覆盖已有文件）", DefaultFilename: "Ultra-privacy-report.json", Filters: []runtime.FileFilter{{DisplayName: "JSON 报告", Pattern: "*.json"}}})
}
var savePrivacyExport = func(path string, content []byte) error {
	release, err := localstate.AcquireDirectory(filepath.Dir(path), false)
	if err != nil {
		return errors.New("输出目录不是可确认的本地目录；没有保存文件")
	}
	defer release()
	return privacydata.WriteNewLocal(path, content)
}

func (a *App) PrivacyExportDetection(previewID string) (string, error) {
	a.privacyMu.Lock()
	p := a.privacyExport
	valid := p != nil && p.id == previewID && a.privacySnapshot != nil && a.privacySnapshot.id == p.sourceID && time.Now().Before(p.expires)
	a.privacyMu.Unlock()
	if !valid {
		return "", errors.New("导出预览已过期或失效；请重新预览")
	}
	path, err := choosePrivacyExportDestination(a.ctx)
	if err != nil || path == "" {
		return "", err
	}
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	if a.privacyExport != p || a.privacySnapshot == nil || a.privacySnapshot.id != p.sourceID || time.Now().After(p.expires) {
		return "", errors.New("选择位置期间预览已失效；未保存文件")
	}
	if err := savePrivacyExport(path, p.content); err != nil {
		return "", err
	}
	a.privacyExport = nil
	return path, nil
}

func (a *App) PrivacyInspectImage() (*privacydata.Report, error) {
	path, err := chooseImage(a.ctx)
	if err != nil || path == "" {
		return nil, err
	}
	r, err := privacydata.Inspect(path)
	return &r, err
}

func (a *App) PrivacyCleanImage() (*privacydata.Report, error) {
	source, err := chooseImage(a.ctx)
	if err != nil || source == "" {
		return nil, err
	}
	ext := filepath.Ext(source)
	name := strings.TrimSuffix(filepath.Base(source), ext) + "-clean" + ext
	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "保存清理后的图片副本（不覆盖原文件）", DefaultFilename: name})
	if err != nil || dest == "" {
		return nil, err
	}
	r, err := privacydata.Clean(source, dest)
	return &r, err
}

func chooseImage(ctx context.Context) (string, error) {
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: "选择本地 JPEG 或 PNG 图片", Filters: []runtime.FileFilter{{DisplayName: "JPEG / PNG", Pattern: "*.jpg;*.jpeg;*.png"}}})
}

func (a *App) timePrivacy() *timeprivacy.Manager {
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	if a.privacyTime == nil {
		a.privacyTime = timeprivacy.New(filepath.Join(userConfigDir(), identity.AppName, "privacy-time"))
	}
	return a.privacyTime
}
func (a *App) PrivacyTimeReport() (timeprivacy.Report, error) { return a.timePrivacy().Report() }
func (a *App) PrivacyTimeApply(id string) (timeprivacy.Result, error) {
	a.invalidatePrivacyEvidence()
	return a.timePrivacy().Apply(id)
}
func (a *App) PrivacyTimeRestore() (timeprivacy.Result, error) {
	a.invalidatePrivacyEvidence()
	return a.timePrivacy().Restore()
}
