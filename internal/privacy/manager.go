package privacy

import (
	"errors"
	"fmt"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Manager struct {
	gate        func() (*maintenance.Lease, error)
	lease       *maintenance.Lease
	mu          sync.Mutex
	dir         string
	backend     backend
	persist     func(string, []byte) error
	machineOnly bool
	lockStorage func(string) (func(), error)
	checkStore  func(string) error
	openJournal func(string) (*os.File, error)
}

func New(dir string) *Manager {
	return &Manager{dir: filepath.Clean(dir), backend: newBackend(), persist: durableWrite, gate: maintenance.Acquire}
}

// NewMachine is for the elevated one-shot helper only. It exposes the fixed
// machine-policy subset and never reads or writes browser/current-user keys.
// Keep this store in an administrator-only ProgramData directory, distinct from
// the interactive user's store. A later authorized administrator can restore it.
func NewMachine(dir string) *Manager {
	return &Manager{dir: filepath.Clean(dir), backend: machineBackend{newBackend()}, persist: durableMachineWrite, machineOnly: true, gate: maintenance.Acquire,
		lockStorage: lockMachineStore, checkStore: validateMachineStoreRead, openJournal: openMachineJournal}
}

// ValidateMachineStoreDirectory is a read-only trust check for the helper's
// fixed machine directory. It never repairs legacy ACLs or creates objects.
func ValidateMachineStoreDirectory(dir string) error { return validateMachineDirectoryRead(dir) }

type machineBackend struct{ backend }

func (b machineBackend) environment() environment {
	if scoped, ok := b.backend.(interface{ machineEnvironment() environment }); ok {
		return scoped.machineEnvironment()
	}
	return b.backend.environment()
}

func machineContext(e environment) bool { return e.Platform == "windows" && e.Admin }

func (m *Manager) contextAllowed(e environment) bool {
	if m.machineOnly {
		return machineContext(e)
	}
	return e.Platform == "windows" && e.SafeUser
}

func (m *Manager) lock() (func(), error) {
	if m.lockStorage != nil {
		return m.lockStorage(m.dir)
	}
	return lockStore(m.dir)
}

func machineLocation(l location) bool {
	for _, c := range catalog() {
		if c.SystemPolicy {
			for _, x := range c.Changes {
				if x.Location == l {
					return true
				}
			}
		}
	}
	return false
}
func (b machineBackend) read(l location) (value, error) {
	if !machineLocation(l) || !machineContext(b.environment()) {
		return value{}, errors.New("current-user access is forbidden in machine helper")
	}
	return b.backend.read(l)
}
func (b machineBackend) write(l location, v value) error {
	if !machineLocation(l) || !machineContext(b.environment()) {
		return errors.New("current-user access is forbidden in machine helper")
	}
	if scoped, ok := b.backend.(interface{ writeMachine(location, value) error }); ok {
		return scoped.writeMachine(l, v)
	}
	return b.backend.write(l, v)
}
func (m *Manager) owner() string {
	if m.machineOnly {
		return "machine-policy-store-v1"
	}
	return m.backend.environment().UserKey
}
func (m *Manager) controls() []control {
	if !m.machineOnly {
		return catalog()
	}
	var out []control
	for _, c := range catalog() {
		if c.SystemPolicy {
			out = append(out, c)
		}
	}
	return out
}
func (m *Manager) control(id string) (control, bool) {
	for _, c := range m.controls() {
		if c.State.ID == id {
			return c, true
		}
	}
	return control{}, false
}

func eligibleForScope(c control, e environment, machineOnly bool) (bool, string) {
	if e.Platform != "windows" {
		return false, "仅 Windows 支持自动设置；原隧道不受影响。"
	}
	if machineOnly && (!c.SystemPolicy || !machineContext(e)) {
		return false, "机器策略通道仅允许已提升进程操作固定 HKLM 策略。"
	}
	if !machineOnly && !e.SafeUser {
		return false, "当前为服务账号或无法确认交互用户；拒绝修改 systemprofile／其他用户的配置。"
	}
	if c.SystemPolicy {
		if e.Build < c.MinBuild {
			return false, "此系统版本不在该正式策略的支持范围；请通过系统设置检查。"
		}
		ok := false
		for _, p := range []string{"Professional", "Enterprise", "Education", "IoTEnterprise"} {
			if strings.HasPrefix(e.Edition, p) {
				ok = true
			}
		}
		if !ok {
			return false, "此 Windows 版本的组策略适用性未核实；请在系统设置手动处理。"
		}
		if !e.Admin {
			return false, "需要管理员权限；未授权不会影响现有 VPN 或其他策略。"
		}
	}
	return true, ""
}

func (m *Manager) Report() (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.backend.environment()
	r := Report{Platform: e.Platform, Admin: e.Admin, ObserverElevated: e.Admin, Controls: []ControlState{}, Capabilities: capabilities(), Warnings: append([]string{}, e.Warnings...), BackupHealthy: true}
	if m.machineOnly {
		r.Capabilities = []Capability{}
	}
	r.Warnings = append(r.Warnings, "策略检查不发送 HTTP 探测请求；用户恢复存储仅接受可确认的本地固定盘目录，不支持网络重定向或重解析路径。策略回读不代表浏览器已接纳，更不代表匿名或所有应用都无法读取信息。", "隐私策略和 VPN 隧道独立：任何策略失败都不能触发直连回退、停止隧道或关闭其他保护。")
	j, jerr := m.load()
	if jerr != nil {
		r.BackupHealthy = false
		r.Warnings = append(r.Warnings, jerr.Error())
	}
	for _, c := range m.controls() {
		s := c.State
		var writeReason string
		s.CanApply, writeReason = eligibleForScope(c, e, m.machineOnly)
		s.ReadStatus = "not_checked"
		s.Detail = writeReason
		if !s.CanApply {
			s.Status = "manual"
		} else {
			s.Status = "available"
		}
		// Permission to read policy values and permission/support to write them
		// are separate facts. Ordinary users can normally read fixed HKLM policy
		// keys even after declining UAC or on an unverified Windows edition.
		canRead := m.contextAllowed(e)
		if canRead {
			all := true
			readFailed := false
			override := false
			for _, x := range c.Changes {
				v, err := m.backend.read(x.Location)
				if err != nil {
					readFailed = true
					break
				}
				if !same(v, x.Desired) {
					all = false
				}
				if x.Location.Hive == "HKCU" {
					machine := x.Location
					machine.Hive = "HKLM"
					mv, err := m.backend.read(machine)
					if err != nil {
						readFailed = true
						break
					}
					if mv.Exists {
						override = true
					}
				}
			}
			s.ReadStatus = "different"
			s.Detail = "本机固定策略值未设置或与目标值不同；这不表示当前一定正在泄漏。"
			if all {
				s.ReadStatus = "match"
				if s.CanApply {
					s.Status = "configured"
				}
				s.Detail = "已只读取得目标策略值；仅是配置证据，不证明版本适用或已生效。浏览器需在策略页确认，Windows 功能需在系统设置验证。"
			}
			if override {
				s.ReadStatus = "machine_override"
				s.Status = "policy_conflict"
				s.Detail = "检测到对应机器级策略；本应用不覆盖它，请检查组织／设备管理和浏览器策略页。"
				s.CanApply = false
			}
			if readFailed {
				s.ReadStatus = "unknown"
				s.Status = "unknown"
				s.Detail = "本地策略无法读取，不能判断保护状态。"
				s.CanApply = false
			}
			if writeReason != "" {
				s.Detail += " " + writeReason
			}
		}
		if jerr == nil {
			if saved, ok := j.Controls[s.ID]; ok {
				s.Managed = true
				if saved.Stage != "active" {
					s.Status = "recovery_pending"
					s.Detail = "上次操作未完成；可安全重试恢复。已保存原值，不影响隧道。"
					s.CanApply = false
				} else if !canRead {
					s.Status = "unknown"
					s.Detail = "当前平台或用户上下文不支持安全读取该作用域；原始备份保留。"
					s.CanApply = false
				} else {
					for _, x := range saved.Changes {
						v, err := m.backend.read(x.Location)
						if err != nil {
							s.Status = "unknown"
							s.ReadStatus = "unknown"
							s.Detail = "已保存原值，但本次无法回读当前策略；不能推断发生外部修改。"
							s.CanApply = false
							break
						}
						if !same(v, x.Desired) {
							s.Status = "external_change"
							s.Detail = "策略已被其他软件或系统修改；恢复会保留外部变更并报告冲突。"
							s.CanApply = false
							break
						}
					}
				}
			}
		}
		if jerr != nil {
			s.CanApply = false
			s.Detail += " 备份异常期间拒绝写入和恢复。"
		}
		r.Controls = append(r.Controls, s)
	}
	for _, cap := range r.Capabilities {
		if cap.Status == "manual" || cap.Status == "unsupported" || cap.Status == "planned" {
			r.Controls = append(r.Controls, ControlState{ID: cap.ID, Title: cap.Title, Category: "guidance", Scope: cap.Scope, Description: cap.Description, Status: cap.Status, Detail: cap.Description, Guide: cap.Guide, SettingsID: cap.SettingsID, Sources: []string{}})
		}
	}
	// Report remains usable when backups are damaged; writes always return errors.
	return r, nil
}

func (m *Manager) Apply(id string) (ActionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.control(id)
	if !ok {
		return failure(id, "此项只提供引导，或不在可执行白名单中。")
	}
	if yes, why := eligibleForScope(c, m.backend.environment(), m.machineOnly); !yes {
		return failure(id, why)
	}
	release, gateErr := m.enterMaintenance(c.SystemPolicy)
	if gateErr != nil {
		return failure(id, gateErr.Error())
	}
	defer release()
	unlock, err := m.lock()
	if err != nil {
		return failure(id, "隐私备份目录无法加锁；请关闭其他修改窗口并检查权限。")
	}
	defer unlock()
	j, err := m.load()
	if err != nil {
		return failure(id, err.Error())
	}
	if old, exists := j.Controls[id]; exists {
		if old.Stage != "active" {
			return failure(id, "上次事务未完成；请先恢复此项，再重新启用。")
		}
		for _, x := range old.Changes {
			v, err := m.backend.read(x.Location)
			if err != nil || !same(v, x.Desired) {
				return failure(id, "检测到外部变更；请先恢复并处理冲突，不覆盖原始备份。")
			}
		}
		return ActionResult{id, "configured", "已保存的策略仍在；需在浏览器或系统中验证生效。", false}, nil
	}
	s := savedControl{Stage: "applying", Changes: []savedChange{}}
	for _, x := range c.Changes {
		if x.Location.Hive == "HKCU" {
			higher := x.Location
			higher.Hive = "HKLM"
			v, err := m.backend.read(higher)
			if err != nil {
				return failure(id, "机器级策略无法检查；停止写入。")
			}
			if v.Exists {
				return failure(id, "已有机器级策略；本应用不覆盖组织或设备级设置。")
			}
		}
		v, err := m.backend.read(x.Location)
		if err != nil {
			return failure(id, "原策略无法读取；未修改任何值。")
		}
		if same(v, x.Desired) {
			continue
		}
		if !validValue(v) || len(v.Data) > 65536 {
			return failure(id, "原策略类型或大小无法安全备份；请手动处理。")
		}
		s.Changes = append(s.Changes, savedChange{x.Location, v, x.Desired})
	}
	if len(s.Changes) == 0 {
		return ActionResult{id, "configured", "当前已有目标策略值；未接管原有设置，也不会在恢复时移除它。", false}, nil
	}
	j.Controls[id] = s
	if err = m.save(j); err != nil {
		return failure(id, err.Error())
	}
	changed := false
	for _, x := range s.Changes {
		v, readErr := m.backend.read(x.Location)
		if readErr != nil || !same(v, x.Before) {
			return m.rollbackAfterFailure(id, j, changed, "写入前检测到外部变更或读取失败。")
		}
		if err = m.backend.write(x.Location, x.Desired); err != nil {
			return m.rollbackAfterFailure(id, j, changed, "策略写入失败。")
		}
		changed = true
		v, err = m.backend.read(x.Location)
		if err != nil || !same(v, x.Desired) {
			return m.rollbackAfterFailure(id, j, changed, "策略写入后校验失败。")
		}
	}
	s.Stage = "active"
	j.Controls[id] = s
	if err = m.save(j); err != nil {
		return m.rollbackAfterFailure(id, j, changed, "策略已写入，但完成记录无法持久保存。")
	}
	return ActionResult{id, "configured", "策略已写入并回读；重启浏览器后检查策略页。此结果不是网页或原生应用的运行时防泄漏验证。", changed}, nil
}

func (m *Manager) rollbackAfterFailure(id string, j journal, changed bool, reason string) (ActionResult, error) {
	r, err := m.restoreLocked(id, j)
	if err != nil {
		msg := reason + " 自动回滚未全部完成；原值备份保留，可重试恢复。"
		return ActionResult{id, "recovery_pending", msg, changed || r.Changed}, errors.New(msg)
	}
	msg := reason + " 已回滚本次可恢复的写入；VPN 和其他策略不受影响。"
	return ActionResult{id, "rolled_back", msg, changed || r.Changed}, errors.New(msg)
}

func (m *Manager) Restore(id string) (ActionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.control(id)
	if !ok {
		return failure(id, "此项不在可恢复策略白名单中。")
	}
	if !m.contextAllowed(m.backend.environment()) {
		return failure(id, "当前平台或用户不支持恢复。")
	}
	release, gateErr := m.enterMaintenance(c.SystemPolicy)
	if gateErr != nil {
		return failure(id, gateErr.Error())
	}
	defer release()
	unlock, err := m.lock()
	if err != nil {
		return failure(id, "隐私备份目录无法加锁。")
	}
	defer unlock()
	j, err := m.load()
	if err != nil {
		return failure(id, err.Error())
	}
	return m.restoreLocked(id, j)
}

func (m *Manager) restoreLocked(id string, j journal) (ActionResult, error) {
	s, ok := j.Controls[id]
	if !ok {
		return ActionResult{id, "unmanaged", "没有本应用保存的原值；不会修改外部设置。", false}, nil
	}
	for _, x := range s.Changes {
		if x.Location.Hive == "HKLM" && !m.backend.environment().Admin {
			return failure(id, "恢复机器级策略需要管理员权限；备份已保留。")
		}
	}
	s.Stage = "restoring"
	j.Controls[id] = s
	if err := m.save(j); err != nil {
		return failure(id, err.Error())
	}
	changed := false
	conflicts := 0
	failed := 0
	for i := len(s.Changes) - 1; i >= 0; i-- {
		x := s.Changes[i]
		v, err := m.backend.read(x.Location)
		if err != nil {
			failed++
			continue
		}
		if same(v, x.Before) {
			continue
		}
		if !same(v, x.Desired) {
			conflicts++
			continue
		}
		if err = m.backend.write(x.Location, x.Before); err != nil {
			failed++
			continue
		}
		changed = true
		v, err = m.backend.read(x.Location)
		if err != nil || !same(v, x.Before) {
			failed++
		}
	}
	if failed > 0 || conflicts > 0 {
		msg := "恢复未全部完成；备份保留，可重试。"
		status := "recovery_pending"
		if conflicts > 0 {
			msg = "检测到外部变更，已保留外部设置；备份保留。请核对后重试，或手动将冲突项还原到保存的策略状态再恢复。"
			status = "conflict"
		}
		return ActionResult{id, status, msg, changed}, errors.New(msg)
	}
	delete(j.Controls, id)
	if err := m.save(j); err != nil {
		return ActionResult{id, "recovery_pending", "原值已恢复，但完成记录保存失败；可重试恢复。", changed}, err
	}
	return ActionResult{id, "restored", "已恢复本应用修改前的设置；原本不存在的值已移除。", changed}, nil
}

func (m *Manager) RestoreAll() ([]ActionResult, error) {
	// Keep one store lock for the batch; a failed item does not prevent others.
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.backend.environment()
	if !m.contextAllowed(e) {
		return []ActionResult{}, errors.New("当前平台或用户不支持恢复")
	}
	unlock, err := m.lock()
	if err != nil {
		return []ActionResult{}, errors.New("隐私备份目录无法加锁")
	}
	defer unlock()
	j, err := m.load()
	if err != nil {
		return []ActionResult{}, err
	}
	ids := make([]string, 0, len(j.Controls))
	needsGate := m.machineOnly
	for id := range j.Controls {
		if c, ok := m.control(id); ok && c.SystemPolicy {
			needsGate = true
		}
		ids = append(ids, id)
	}
	release, gateErr := m.enterMaintenance(needsGate)
	if gateErr != nil {
		return []ActionResult{}, gateErr
	}
	defer release()
	sort.Strings(ids)
	results := []ActionResult{}
	var problems []error
	for _, id := range ids {
		fresh, err := m.load()
		if err != nil {
			problems = append(problems, err)
			break
		}
		r, err := m.restoreLocked(id, fresh)
		results = append(results, r)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", id, err))
		}
	}
	return results, errors.Join(problems...)
}
func failure(id, msg string) (ActionResult, error) {
	return ActionResult{id, "failed", msg, false}, errors.New(msg)
}
