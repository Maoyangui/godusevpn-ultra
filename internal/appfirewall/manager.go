// Package appfirewall implements optional per-program network restrictions.
// It never starts, stops, or modifies the VPN tunnel or its global guard.
package appfirewall

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var ErrUnsupported = errors.New("per-application WFP controls require 64-bit Windows")

type Rule struct {
	ID      string `json:"id"`
	Program string `json:"program"`
	Mode    string `json:"mode"`
}

type RuleStatus struct {
	Rule             Rule   `json:"rule"`
	State            string `json:"state"`
	InstalledFilters int    `json:"installedFilters"`
	ExpectedFilters  int    `json:"expectedFilters"`
	Detail           string `json:"detail"`
}

type Report struct {
	Supported      bool         `json:"supported"`
	Elevated       bool         `json:"elevated"`
	Rules          []RuleStatus `json:"rules"`
	Warnings       []string     `json:"warnings"`
	Scope          string       `json:"scope"`
	Loopback       string       `json:"loopback"`
	Enforcement    string       `json:"enforcement"`
	UnknownFilters int          `json:"unknownFilters"`
}

type Result struct {
	ID         string `json:"id"`
	Applied    bool   `json:"applied"`
	Persistent bool   `json:"persistent"`
	Verified   bool   `json:"verified"` // Installed filter evidence only, never end-to-end enforcement.
	State      string `json:"state"`
	Warning    string `json:"warning"`
	Rule       *Rule  `json:"rule,omitempty"`
}

type installed struct {
	Rule    Rule
	Key     string
	Matches bool
	Unknown bool // An enumerated owned-provider object cannot be safely decoded.
}

type backend interface {
	supported() bool
	elevated() bool
	snapshot() ([]installed, []string, error)
	replace(Rule) error
	remove(string) error
}

type Manager struct {
	backend backend
	lease   *maintenance.Lease
	gate    func() (*maintenance.Lease, error)
}

// New retains the application data-directory parameter for a uniform module API.
// WFP's protected persistent store is the single source of truth: its transaction
// atomically commits both metadata and rules, avoiding a JSON/WFP crash window.
// No connection history or separate per-program state is written to dir.
func New(dir string) *Manager { return &Manager{backend: newBackend(), gate: maintenance.Acquire} }
func (m *Manager) WithLease(l *maintenance.Lease) *Manager {
	return &Manager{backend: m.backend, gate: m.gate, lease: l}
}
func (m *Manager) enter() (func(), error) {
	if m.lease != nil {
		return func() {}, m.lease.Check()
	}
	if m.gate == nil {
		return func() {}, nil
	}
	l, err := m.gate()
	if err != nil {
		return nil, err
	}
	return func() { _ = l.Close() }, nil
}

// process-wide serialization also covers independently created Manager instances.
var opMu sync.Mutex

const limitation = "仅核对 WFP 过滤器已安装，不代表流量已阻断。现有隧道 guard 的 permitSelf 使用 CLEAR_ACTION_RIGHT 硬放行，可阻止匹配路径上的低权重普通阻断覆盖；其余软放行不据此判定。代理重发会丢失原应用身份。未进行实机流量验证。不会因此停止隧道或关闭其他保护。"

func (m *Manager) Report() (Report, error) {
	opMu.Lock()
	defer opMu.Unlock()
	return m.report()
}

func (m *Manager) report() (Report, error) {
	r := Report{Supported: m.backend.supported(), Elevated: m.backend.elevated(), Rules: []RuleStatus{}, Warnings: []string{limitation, "规则绑定 EXE 路径，不是代码签名或文件内容；更换路径或其他程序代发不受覆盖。发现限制仅针对 IP 组播/广播，不覆盖蓝牙、系统服务或单播发现协议。"}, Scope: "指定 EXE 路径的新建直接入站/出站连接；不覆盖代理重发、子进程、其他 EXE、已存在连接或管理员绕过", Loopback: "unprotected", Enforcement: "not_live_tested"}
	if !r.Supported {
		return r, nil
	}
	fs, warnings, err := m.backend.snapshot()
	r.Warnings = append(r.Warnings, warnings...)
	if err != nil {
		return r, fmt.Errorf("读取应用网络规则（不影响 VPN）: %w", err)
	}
	groups := map[string][]installed{}
	for _, f := range fs {
		if f.Unknown {
			r.UnknownFilters++
			continue
		}
		groups[f.Rule.ID] = append(groups[f.Rule.ID], f)
	}
	for _, items := range groups {
		rule := items[0].Rule
		expected := len(specs(rule))
		good := 0
		seen := map[string]bool{}
		mismatch := false
		for _, item := range items {
			if item.Rule != rule || !item.Matches || seen[item.Key] {
				mismatch = true
			}
			seen[item.Key] = true
			if item.Matches {
				good++
			}
		}
		state := "partial"
		if !mismatch && good == expected && expected > 0 {
			state = "filters_installed"
		}
		r.Rules = append(r.Rules, RuleStatus{Rule: rule, State: state, InstalledFilters: good, ExpectedFilters: expected, Detail: limitation})
	}
	sort.Slice(r.Rules, func(i, j int) bool { return r.Rules[i].Rule.ID < r.Rules[j].Rule.ID })
	return r, nil
}

func (m *Manager) Apply(rule Rule) (Result, error) {
	opMu.Lock()
	defer opMu.Unlock()
	if !m.backend.supported() {
		return Result{}, ErrUnsupported
	}
	if !m.backend.elevated() {
		return Result{}, errors.New("需要已提升的管理员令牌；未改动隧道或防火墙")
	}
	release, err := m.enter()
	if err != nil {
		return Result{}, err
	}
	defer release()
	if rule.ID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return Result{}, err
		}
		rule.ID = hex.EncodeToString(id[:])
	}
	if err := validateRule(rule, true); err != nil {
		return Result{}, err
	}
	canonical, err := filepath.EvalSymlinks(rule.Program)
	if err != nil {
		return Result{}, fmt.Errorf("解析程序规范路径: %w", err)
	}
	rule.Program = filepath.Clean(canonical)
	fs, _, err := m.backend.snapshot()
	if err != nil {
		return Result{}, err
	}
	ids := map[string]bool{}
	for _, f := range fs {
		if f.Unknown {
			continue
		}
		ids[f.Rule.ID] = true
	}
	if len(ids) >= 128 && !ids[rule.ID] {
		return Result{}, errors.New("应用规则最多 128 项")
	}
	if err = m.backend.replace(rule); err != nil {
		return Result{ID: rule.ID, Rule: &rule}, fmt.Errorf("WFP 事务失败；请刷新核对原规则。未调用 VPN 连接或 guard 操作: %w", err)
	}
	result := Result{ID: rule.ID, Applied: true, Persistent: true, State: "installed_unverified", Warning: limitation, Rule: &rule}
	r, err := m.report()
	if err != nil {
		return result, fmt.Errorf("规则已提交，但复核失败；请重新检查或移除: %w", err)
	}
	for _, s := range r.Rules {
		if s.Rule == rule && s.State == "filters_installed" {
			result.State = s.State
			result.Verified = true
			result.Warning = strings.Join(r.Warnings, "\n")
			return result, nil
		}
	}
	return result, errors.New("规则已提交，但过滤器核对不完整；不代表有效保护，请检查或移除")
}

func (m *Manager) Remove(id string) (Result, error) {
	opMu.Lock()
	defer opMu.Unlock()
	if !m.backend.supported() {
		return Result{}, ErrUnsupported
	}
	if !m.backend.elevated() {
		return Result{}, errors.New("需要已提升的管理员令牌；未改动隧道或防火墙")
	}
	release, err := m.enter()
	if err != nil {
		return Result{}, err
	}
	defer release()
	if !validID.MatchString(id) {
		return Result{}, errors.New("无效的规则 ID")
	}
	if err := m.backend.remove(id); err != nil {
		return Result{ID: id}, fmt.Errorf("撤销事务失败；请刷新核对规则。未调用 VPN 连接或 guard 操作: %w", err)
	}
	result := Result{ID: id, Applied: true, Persistent: true, State: "removed_unverified", Warning: "撤销事务已提交，完成状态仍待核对；未改动原 VPN guard 或其他防火墙规则。"}
	fs, warnings, err := m.backend.snapshot()
	if err != nil {
		return result, fmt.Errorf("撤销已提交但复核失败: %w", err)
	}
	if len(warnings) > 0 {
		result.Warning += "\n" + strings.Join(warnings, "\n")
	}
	for _, f := range fs {
		if f.Unknown {
			result.State = "unknown_remnants"
			result.Warning += " 仍有无法识别的本产品过滤器，无法排除该规则残留；保留对象供核对。"
			return result, errors.New("撤销已提交，但未知过滤器残留使完成状态无法确认；未删除无法安全归属的对象")
		}
		if f.Rule.ID == id {
			return result, errors.New("仍检测到本规则的过滤器")
		}
	}
	result.Verified = true
	result.State = "removed"
	return result, nil
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func validateRule(r Rule, checkFile bool) error {
	if !validID.MatchString(r.ID) {
		return errors.New("规则 ID 只能含 1–64 位字母、数字、下划线或连字符")
	}
	if r.Mode != "block_all" && r.Mode != "block_lan" && r.Mode != "block_discovery" {
		return errors.New("不支持的规则模式")
	}
	if strings.IndexByte(r.Program, 0) >= 0 || !filepath.IsAbs(r.Program) || !strings.EqualFold(filepath.Ext(r.Program), ".exe") {
		return errors.New("必须选择本地绝对路径的 EXE 文件")
	}
	p := strings.ToLower(filepath.Clean(r.Program))
	if strings.HasPrefix(p, `\\`) || strings.Contains(p[2:], ":") {
		return errors.New("不支持 UNC、设备路径或备用数据流")
	}
	base := strings.ToLower(filepath.Base(p))
	if strings.HasPrefix(base, "godusevpn") || base == "sing-box.exe" || base == "xray.exe" || base == "mihomo.exe" || base == "clash.exe" {
		return errors.New("不能限制本产品或已知代理内核，以免阻断隧道及恢复入口")
	}
	if self, err := os.Executable(); err == nil && strings.EqualFold(filepath.Clean(self), p) {
		return errors.New("不能限制当前程序")
	}
	if win := systemDirectory(); win != "" {
		root := strings.ToLower(filepath.Clean(win)) + string(filepath.Separator)
		if strings.HasPrefix(p, root) {
			return errors.New("此版本不允许限制 Windows 系统目录内程序，避免误伤系统服务")
		}
	}
	if checkFile {
		info, err := os.Lstat(r.Program)
		if err != nil {
			return fmt.Errorf("检查程序路径: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("程序必须是普通本地 EXE 文件")
		}
		// Inspect reparse points directly: EvalSymlinks string comparison also
		// changes harmless Windows short (8.3) paths and would reject those.
		for current := filepath.Clean(r.Program); ; current = filepath.Dir(current) {
			linked, err := isReparsePoint(current)
			if err != nil {
				return fmt.Errorf("检查路径重解析点: %w", err)
			}
			if linked {
				return errors.New("程序路径不能经过符号链接或目录联接")
			}
			if parent := filepath.Dir(current); parent == current {
				break
			}
		}
		resolved, err := filepath.EvalSymlinks(r.Program)
		if err != nil {
			return fmt.Errorf("解析程序路径: %w", err)
		}
		canonical := r
		canonical.Program = resolved
		if err = validateRule(canonical, false); err != nil {
			return err
		}
		if self, err := os.Executable(); err == nil {
			if ownInfo, err := os.Stat(self); err == nil && os.SameFile(info, ownInfo) {
				return errors.New("不能限制当前程序或其文件别名")
			}
		}
	}
	return nil
}

type transactionHooks struct{ begin, commit, abort func() error }

// The OS transaction commits filter metadata and restrictions together. Any
// error or panic before a successful commit aborts the entire replacement.
func runTransaction(h transactionHooks, change func() error) error {
	if err := h.begin(); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = h.abort()
		}
	}()
	if err := change(); err != nil {
		return err
	}
	if err := h.commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

type filterSpec struct {
	Layer  string
	Prefix string
	Suffix string
}

func specs(r Rule) []filterSpec {
	var prefixes []string
	switch r.Mode {
	case "block_all":
		prefixes = []string{"", "v6"}
	case "block_lan":
		prefixes = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7", "fe80::/10"}
	case "block_discovery":
		prefixes = []string{"224.0.0.0/4", "255.255.255.255/32", "ff00::/8"}
	default:
		return nil
	}
	var result []filterSpec
	for _, prefix := range prefixes {
		family := "4"
		if strings.Contains(prefix, ":") || prefix == "v6" {
			family = "6"
		}
		for _, direction := range []string{"out", "in"} {
			result = append(result, filterSpec{Layer: direction + family, Prefix: prefix, Suffix: direction + family + ":" + prefix})
		}
	}
	return result
}
