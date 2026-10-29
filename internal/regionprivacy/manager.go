// Package regionprivacy edits only the current ordinary user's Windows country
// or region. It has no tunnel, network, GPS, keyboard or display-language API.
package regionprivacy

import (
	"errors"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrUnsupported = errors.New("REGION_CURRENT_ORDINARY_USER_UNAVAILABLE")
	ErrInput       = errors.New("REGION_CODE_NOT_IN_SYSTEM_CATALOG")
	ErrStore       = errors.New("REGION_PRIVATE_BACKUP_UNAVAILABLE")
	ErrBusy        = errors.New("REGION_OPERATION_BUSY")
	ErrConflict    = errors.New("REGION_EXTERNAL_CHANGE_PRESERVED")
	ErrPending     = errors.New("REGION_RECOVERY_REQUIRED")
	ErrReadback    = errors.New("REGION_CHANGE_NOT_CONFIRMED")
)

var codePattern = regexp.MustCompile(`\A(?:[A-Z]{2}|[0-9]{3})\z`)
var operationMu sync.Mutex

const Scope = "仅当前 Windows 用户的国家或地区；不更改 GPS、IP、显示语言、键盘、时区或浏览器环境。"
const CompetitionBoundary = "本应用操作已串行化；Windows 接口不提供与其他程序的原子比较替换，同时更改仍有竞争窗口。发现不同状态会停止，不强制恢复。"

type Choice struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}
type Report struct {
	Supported         bool     `json:"supported"`
	CurrentCode       string   `json:"currentCode"`
	Choices           []Choice `json:"choices"`
	Status            string   `json:"status"`
	Managed           bool     `json:"managed"`
	CanApply          bool     `json:"canApply"`
	CanRestore        bool     `json:"canRestore"`
	OriginalCode      string   `json:"originalCode,omitempty"`
	ObservedAt        string   `json:"observedAt,omitempty"`
	Scope             string   `json:"scope"`
	Detail            string   `json:"detail"`
	AtomicExternalCAS bool     `json:"atomicExternalCAS"`
	Warnings          []string `json:"warnings"`
}
type Result struct {
	Status      string `json:"status"`
	CurrentCode string `json:"currentCode"`
	Changed     bool   `json:"changed"`
	Verified    bool   `json:"verified"`
	Detail      string `json:"detail"`
}

type userIdentity struct{ sid, directory string }
type backend interface {
	identity() (userIdentity, error)
	choices() ([]Choice, error)
	current() (string, error)
	set(string) error
}
type store interface {
	read() ([]byte, error)
	save([]byte) error
	close()
}
type storeMode uint8

const (
	storeReadOnly storeMode = iota
	storeUpdate
	storeCreate
)

type openStore func(userIdentity, storeMode) (store, error)
type Manager struct {
	backend backend
	open    openStore
}

// New performs no system reads, writes or directory creation.
func New() *Manager { return &Manager{backend: nativeBackend{}, open: openNativeStore} }

func checkedChoices(choices []Choice) ([]Choice, error) {
	if len(choices) == 0 || len(choices) > 1024 {
		return nil, ErrUnsupported
	}
	seen := map[string]bool{}
	out := append([]Choice(nil), choices...)
	for _, choice := range out {
		if !codePattern.MatchString(choice.Code) || choice.Label == "" || len(choice.Label) > 512 || !utf8.ValidString(choice.Label) || stringsHaveControls(choice.Label) || seen[choice.Code] {
			return nil, ErrUnsupported
		}
		seen[choice.Code] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func stringsHaveControls(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}
func listed(choices []Choice, code string) bool {
	for _, choice := range choices {
		if choice.Code == code {
			return true
		}
	}
	return false
}
func (m *Manager) context() (userIdentity, []Choice, error) {
	if m == nil || m.backend == nil || m.open == nil {
		return userIdentity{}, nil, ErrUnsupported
	}
	identity, err := m.backend.identity()
	if err != nil || identity.sid == "" || identity.directory == "" {
		return userIdentity{}, nil, ErrUnsupported
	}
	choices, err := m.backend.choices()
	if err != nil {
		return identity, nil, ErrUnsupported
	}
	choices, err = checkedChoices(choices)
	return identity, choices, err
}
func current(b backend) (string, error) {
	value, err := b.current()
	if err != nil || !codePattern.MatchString(value) {
		return "", ErrReadback
	}
	return value, nil
}

func (m *Manager) Report() (Report, error) {
	operationMu.Lock()
	defer operationMu.Unlock()
	report := Report{Choices: []Choice{}, Status: "unsupported", Scope: Scope, Detail: "当前用户或 Windows 接口不可用；没有修改设置。", Warnings: []string{CompetitionBoundary}}
	identity, choices, err := m.context()
	if err != nil {
		return report, nil
	}
	report.Supported, report.Choices = true, choices
	report.CurrentCode, err = current(m.backend)
	if err != nil {
		report.Status, report.Detail = "unknown", "当前国家或地区无法独立读取，修改与恢复已停用。"
		return report, nil
	}
	report.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	report.Status, report.Detail = "available", "选择系统提供的国家或地区，确认后才会影响当前用户的其他应用。"
	report.CanApply = listed(choices, report.CurrentCode)
	session, err := m.open(identity, storeReadOnly)
	if errors.Is(err, os.ErrNotExist) {
		return report, nil
	}
	if err != nil {
		report.Status, report.CanApply, report.Detail = "backup_error", false, "当前值已读回，但备份不可确认；未修改或修复目录。"
		return report, nil
	}
	defer session.close()
	journal, err := load(session, identity.sid)
	if err != nil {
		report.Status, report.CanApply, report.Detail = "backup_error", false, "恢复记录损坏或不属于当前用户；保留记录，停止修改。"
		return report, nil
	}
	if journal == nil {
		return report, nil
	}
	report.OriginalCode = journal.Before
	if !journal.accepts(report.CurrentCode) {
		report.Status, report.CanApply, report.Detail = "external_change", false, "其他来源已更改国家或地区；保留当前值，不强制覆盖或恢复。"
		return report, nil
	}
	if journal.Stage == "restored" {
		return report, nil
	}
	report.Managed = true
	report.CanRestore = listed(choices, journal.Before)
	if journal.Stage != "active" {
		report.Status, report.CanApply, report.Detail = "recovery_pending", false, "上次操作未完成；先检查并恢复原地区，再提交新的选择。"
	} else {
		report.Status, report.Detail = "configured", "已由本应用修改并读回；其他应用可能缓存旧值，需要重新打开后核对。"
	}
	return report, nil
}

func (m *Manager) Apply(code string) (Result, error) {
	if !codePattern.MatchString(code) {
		return Result{}, ErrInput
	}
	operationMu.Lock()
	defer operationMu.Unlock()
	identity, choices, err := m.context()
	if err != nil {
		return Result{}, err
	}
	if !listed(choices, code) {
		return Result{}, ErrInput
	}
	session, err := m.open(identity, storeCreate)
	if err != nil {
		return Result{}, err
	}
	defer session.close()
	journal, err := load(session, identity.sid)
	if err != nil {
		return Result{}, err
	}
	before, err := current(m.backend)
	if err != nil || !listed(choices, before) {
		return Result{}, ErrReadback
	}
	if journal != nil {
		if !journal.accepts(before) {
			return Result{}, ErrConflict
		}
		if journal.Stage != "active" && journal.Stage != "restored" {
			return Result{}, ErrPending
		}
	}
	if before == code {
		return Result{Status: "unchanged", CurrentCode: before, Verified: true, Detail: "当前地区与所选一致；没有写入或冒领现有设置。"}, nil
	}
	original := before
	if journal != nil && journal.Stage == "active" {
		original = journal.Before
	}
	pending := &transaction{Owner: identity.sid, Before: original, Expected: before, Pending: code, Stage: "applying"}
	return m.commit(session, pending, code, "active", "applied")
}

func (m *Manager) Restore() (Result, error) {
	operationMu.Lock()
	defer operationMu.Unlock()
	identity, choices, err := m.context()
	if err != nil {
		return Result{}, err
	}
	session, err := m.open(identity, storeUpdate)
	if errors.Is(err, os.ErrNotExist) {
		return Result{Status: "unchanged", Detail: "没有本应用的地区恢复记录。"}, nil
	}
	if err != nil {
		return Result{}, err
	}
	defer session.close()
	journal, err := load(session, identity.sid)
	if err != nil {
		return Result{}, err
	}
	if journal == nil {
		return Result{Status: "unchanged", Detail: "没有本应用的地区恢复记录。"}, nil
	}
	before, err := current(m.backend)
	if err != nil || !listed(choices, journal.Before) {
		return Result{}, ErrReadback
	}
	if !journal.accepts(before) {
		return Result{}, ErrConflict
	}
	if journal.Stage == "restored" {
		return Result{Status: "unchanged", CurrentCode: before, Verified: true, Detail: "原地区已恢复；没有再次写入。"}, nil
	}
	pending := &transaction{Owner: identity.sid, Before: journal.Before, Expected: before, Pending: journal.Before, Stage: "restoring"}
	return m.commit(session, pending, journal.Before, "restored", "restored")
}

func (m *Manager) commit(session store, pending *transaction, target, stage, resultStatus string) (Result, error) {
	if err := save(session, pending); err != nil {
		return Result{}, err
	}
	// Recheck after durable pending, immediately before the official setter.
	before, err := current(m.backend)
	if err != nil || before != pending.Expected {
		return Result{Status: "recovery_pending", Detail: "写前发现状态改变；未调用地区修改，保留恢复记录。"}, ErrConflict
	}
	identity, err := m.backend.identity()
	if err != nil || identity.sid != pending.Owner {
		return Result{Status: "recovery_pending", Detail: "当前用户身份无法确认；未调用地区修改。"}, ErrUnsupported
	}
	changed := before != target
	if changed {
		err = m.backend.set(target)
	}
	after, readErr := current(m.backend)
	if err != nil || readErr != nil || after != target {
		// No unconditional rollback: the actual state could belong to another
		// process. Pending retains both known pre-call and intended values.
		return Result{Status: "recovery_pending", CurrentCode: after, Changed: readErr == nil && after != before, Detail: "修改结果未完整确认；没有盲目回写，保留恢复线索。"}, ErrReadback
	}
	finished := &transaction{Owner: pending.Owner, Before: pending.Before, Expected: target, Stage: stage}
	if err := save(session, finished); err != nil {
		return Result{Status: "recovery_pending", CurrentCode: after, Changed: changed, Detail: "当前值已读回，但完成记录未确认；请保留备份并重新检查。"}, err
	}
	// A durable commit can take time. Never report success using only a value
	// observed before that disk operation.
	after, err = current(m.backend)
	if err != nil || after != target {
		return Result{Status: "recovery_pending", CurrentCode: after, Changed: changed, Detail: "完成记录后状态再次改变或无法读取；没有回写，需重新检查。"}, ErrReadback
	}
	return Result{Status: resultStatus, CurrentCode: after, Changed: changed, Verified: true, Detail: "当前用户国家或地区已独立读回；不代表 GPS、IP 或网页位置改变。"}, nil
}
