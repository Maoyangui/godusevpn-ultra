// Package timeprivacy provides an explicit Windows time-zone picker, independent
// of tunnels and UTC clock synchronization. Nothing is selected by location/IP.
package timeprivacy

import (
	"errors"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Choice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Report struct {
	CanApply               bool     `json:"canApply"`
	Supported              bool     `json:"supported"`
	CurrentID              string   `json:"currentId"`
	Choices                []Choice `json:"choices"`
	Managed                bool     `json:"managed"`
	Status                 string   `json:"status"`
	Detail                 string   `json:"detail"`
	DaylightSavingDisabled bool     `json:"daylightSavingDisabled"`
}
type Result struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Changed bool   `json:"changed"`
}
type zoneState struct {
	ID          string `json:"id"`
	DSTDisabled bool   `json:"dstDisabled"`
}
type backend interface {
	supported() bool
	choices() ([]Choice, error)
	current() (zoneState, error)
	set(zoneState) error
}
type Manager struct {
	gate    func() (*maintenance.Lease, error)
	ready   func() (bool, error)
	mu      sync.Mutex
	dir     string
	backend backend
	persist func(string, []byte) error
}

func New(dir string) *Manager {
	return &Manager{dir: filepath.Clean(dir), backend: newBackend(), persist: durableWrite, gate: maintenance.Acquire, ready: maintenance.Ready}
}
func (m *Manager) enter() (func(), error) {
	if m.gate == nil {
		return func() {}, nil
	}
	l, e := m.gate()
	if e != nil {
		return nil, e
	}
	return func() { _ = l.Close() }, nil
}

// CurrentID dynamically reads Windows' current time-zone key, without loading a
// journal, enumerating options, using Go's cached time.Local, or changing state.
func CurrentID() (string, error) {
	state, err := newBackend().current()
	if err != nil {
		return "", err
	}
	return state.ID, nil
}

func (m *Manager) Report() (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := Report{Supported: m.backend.supported(), Choices: []Choice{}, Status: "manual", Detail: "仅 Windows 支持本地时区选择；不修改 UTC 时钟、语言、键盘或隧道。"}
	if !r.Supported {
		return r, nil
	}
	choices, err := m.backend.choices()
	if err != nil {
		r.Status = "unknown"
		r.Detail = "系统时区列表无法读取；不提供未经核实的时区选项。"
		return r, nil
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	r.Choices = choices
	current, cerr := m.backend.current()
	if cerr == nil {
		r.CurrentID = current.ID
		r.DaylightSavingDisabled = current.DSTDisabled
	}
	r.Status = "available"
	r.CanApply = true
	r.Detail = "用户明确选择后修改系统时区，并保持现有夏令时开关；不根据出口自动猜测，不修改 UTC 时钟。浏览器可能有独立时区覆盖，需要另行确认。"
	j, jerr := m.load()
	if jerr != nil {
		r.CanApply = false
		r.Status = "backup_error"
		r.Detail = jerr.Error()
		return r, nil
	}
	if j != nil {
		r.Managed = true
		r.Status = "configured"
		r.Detail = "系统时区已由本应用设置；已保存原时区和夏令时偏好，可选择恢复。不是网页环境一致性的实测证明。"
		if j.Stage != "active" {
			r.Status = "recovery_pending"
			r.Detail = "上次操作未完成或结果未确认；原始备份保留，请恢复后再设置。"
		} else if cerr == nil && current != j.Desired {
			r.Status = "conflict"
			r.Detail = "系统时区或夏令时偏好已被外部修改；不会覆盖该变更。"
		}
	}
	if cerr != nil {
		r.CanApply = false
		r.Status = "unknown"
		r.Detail = "当前系统时区无法确认；不会继续修改，已有原始备份保留。"
	}
	if m.ready != nil {
		if ok, _ := m.ready(); !ok {
			r.CanApply = false
			r.Detail += " 请先授权管理员助手初始化机器隐私维护门禁。"
		}
	}
	return r, nil
}

func (m *Manager) Apply(id string) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.backend.supported() {
		return fail("当前平台只提供手动时区设置说明")
	}
	release, gateErr := m.enter()
	if gateErr != nil {
		return fail(gateErr.Error())
	}
	defer release()
	choices, err := m.backend.choices()
	if err != nil || !listed(choices, id) {
		return fail("只允许选择当前 Windows 枚举出的时区 ID")
	}
	unlock, err := lockStore(m.dir)
	if err != nil {
		return fail("时区备份目录无法安全加锁")
	}
	defer unlock()
	j, err := m.load()
	if err != nil {
		return fail(err.Error())
	}
	current, err := m.backend.current()
	if err != nil || !listed(choices, current.ID) {
		return fail("当前时区无法确认或是非标准自定义时区，未修改任何设置")
	}
	if j != nil {
		if j.Stage != "active" {
			return fail("上次时区操作未完成，请先恢复；不覆盖原始备份")
		}
		if current != j.Desired {
			return fail("检测到外部时区变更；请先核对并处理恢复冲突")
		}
	}
	desired := zoneState{ID: id, DSTDisabled: current.DSTDisabled}
	if current == desired {
		return Result{"configured", "已是所选时区，未修改，也不会接管外部设置", false}, nil
	}
	if j == nil {
		j = &transaction{Before: current}
	}
	j.Previous = current
	j.Desired = desired
	j.Stage = "applying"
	if err = m.save(j); err != nil {
		return fail(err.Error())
	}
	// A fresh comparison narrows the race with Windows automatic time-zone
	// selection and other applications. We never disable those features silently.
	check, err := m.backend.current()
	if err != nil || check != current {
		return pending(false, "写入前时区已变化或无法读取；未继续设置，备份保留")
	}
	setErr := m.backend.set(desired)
	after, readErr := m.backend.current()
	changed := readErr == nil && after != current
	if setErr != nil || readErr != nil || after != desired {
		return pending(changed, "时区设置失败或无法确认结果；可能缺少更改时区权限。原始备份保留，可重试恢复，隧道不受影响")
	}
	j.Stage = "active"
	if err = m.save(j); err != nil {
		return pending(changed, "系统已回读到所选时区，但完成记录保存失败；原始备份保留，可恢复")
	}
	return Result{"configured", "系统时区已设置并回读；保持原夏令时偏好，未修改 UTC 时钟。浏览器时区需单独确认", changed}, nil
}

func (m *Manager) Restore() (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.backend.supported() {
		return fail("当前平台不支持自动恢复 Windows 时区")
	}
	release, gateErr := m.enter()
	if gateErr != nil {
		return fail(gateErr.Error())
	}
	defer release()
	unlock, err := lockStore(m.dir)
	if err != nil {
		return fail("时区备份目录无法安全加锁")
	}
	defer unlock()
	j, err := m.load()
	if err != nil {
		return fail(err.Error())
	}
	if j == nil {
		return Result{"unmanaged", "没有本应用保存的时区修改，未改变系统设置", false}, nil
	}
	choices, err := m.backend.choices()
	if err != nil || !listed(choices, j.Before.ID) || !listed(choices, j.Desired.ID) {
		return fail("备份中的时区已不在系统可选列表；保留备份，不执行未知 ID")
	}
	current, err := m.backend.current()
	if err != nil {
		return pending(false, "当前时区无法读取，未修改；备份保留")
	}
	if current == j.Before {
		if err = m.save(nil); err != nil {
			return pending(false, "系统已是原时区，但恢复完成记录无法保存；可重试")
		}
		return Result{"restored", "系统已是原时区，恢复记录已完成", false}, nil
	}
	allowed := current == j.Desired || ((j.Stage == "applying" || j.Stage == "restoring") && current == j.Previous)
	if !allowed {
		return Result{"conflict", "检测到其他程序或用户修改的时区／夏令时偏好，已保留外部变更和原始备份", false}, errors.New("外部时区变更，拒绝覆盖")
	}
	j.Previous = current
	j.Stage = "restoring"
	if err = m.save(j); err != nil {
		return fail(err.Error())
	}
	check, err := m.backend.current()
	if err != nil || check != current {
		return pending(false, "恢复前时区再次变化或无法读取；未继续设置，备份保留")
	}
	setErr := m.backend.set(j.Before)
	after, readErr := m.backend.current()
	changed := readErr == nil && after != current
	if setErr != nil || readErr != nil || after != j.Before {
		return pending(changed, "恢复命令失败或无法确认结果；备份保留，可重试，不影响隧道")
	}
	if err = m.save(nil); err != nil {
		return pending(changed, "原时区已恢复，但完成记录无法保存；可重试恢复")
	}
	return Result{"restored", "已恢复原系统时区和原夏令时偏好；未修改 UTC 时钟", changed}, nil
}

func listed(choices []Choice, id string) bool {
	if id == "" || len(id) > 127 || strings.ContainsAny(id, "\x00\r\n\"\\/") {
		return false
	}
	for _, c := range choices {
		if c.ID == id {
			return true
		}
	}
	return false
}
func fail(msg string) (Result, error) { return Result{"failed", msg, false}, errors.New(msg) }
func pending(changed bool, msg string) (Result, error) {
	return Result{"recovery_pending", msg, changed}, errors.New(msg)
}
