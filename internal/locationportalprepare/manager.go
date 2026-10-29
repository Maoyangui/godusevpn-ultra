package locationportalprepare

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"github.com/Maoyangui/godusevpn/internal/locationportalgate"
)

// The operation slot and durable store lease serialize all preparation and
// restoration. Report stays nonblocking while a FoD operation is in flight.
type Manager struct {
	mu         sync.Mutex
	inspectMu  sync.Mutex
	generation uint64
	owner      string
	recovery   bool
	backend    backend
	gate       gate
	open       openStore
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	closed     bool
	report     Report
	connection *locationportal.Config
}

func newManager(owner string, recovery bool, b backend, g gate, open openStore) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{owner: owner, recovery: recovery, backend: b, gate: g, open: open, ctx: ctx, cancel: cancel, report: initialReport(!recovery)}
}
func initialReport(canPrepare bool) Report {
	return Report{Status: "not_prepared", CanPrepare: canPrepare, HTTPPort: HTTPPort, HTTPSPort: HTTPSPort, Detail: "尚未准备固定本机门户；准备会安装 Windows 开发组件并启用开发模式。", Warnings: []string{"Windows 设备门户默认可能通过 DNS-SD 广告主机与服务；固定 TCP 入站限制不等同于禁止发现广播。", "准备会安装含远程开发组件的 Windows FoD；本应用不会启用设备发现，新增 SSH Broker/Proxy 将保持禁用且停止。", "门户随机凭据仅在管理员备份中以 DPAPI 密文保存；微软认证命令短暂使用凭据参数，界面和日志不显示。"}}
}
func cloneReport(r Report) Report { r.Warnings = append([]string{}, r.Warnings...); return r }
func (m *Manager) cached() Report { m.mu.Lock(); defer m.mu.Unlock(); return cloneReport(m.report) }
func (m *Manager) publish(r Report) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		r.Ready = false
		r.CanPrepare = false
		r.CanRestore = false
	}
	m.report = cloneReport(r)
}
func (m *Manager) phase(stage string, j *journal) {
	r := m.cached()
	r.Stage = stage
	if j != nil {
		r.HasBackup = true
	}
	m.publish(r)
}

// Connection is deliberately separate from the public JSON report. Only the
// authenticated helper's private response encoder may transport these values.
func (m *Manager) Connection() *locationportal.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.recovery || m.report.Busy || !m.report.Ready || m.connection == nil {
		return nil
	}
	v := *m.connection
	return &v
}
func (m *Manager) Report() Report {
	if m == nil {
		return Report{Status: "unsupported", HTTPPort: HTTPPort, HTTPSPort: HTTPSPort, Detail: "当前系统不支持自动准备。"}
	}
	if !m.inspectMu.TryLock() {
		return m.cached()
	}
	defer m.inspectMu.Unlock()
	m.mu.Lock()
	if m.report.Busy || m.closed {
		r := cloneReport(m.report)
		m.mu.Unlock()
		return r
	}
	switch m.report.Status {
	case "location_restore_required", "cleanup_unknown", "external_conflict", "failed", "reboot_required":
		r := cloneReport(m.report)
		m.mu.Unlock()
		return r
	}
	r, generation := cloneReport(m.report), m.generation
	m.mu.Unlock()
	if m.open == nil || m.backend == nil || m.gate == nil {
		r.Status = "unsupported"
		r.CanPrepare = false
		r.CanRestore = false
		r.Detail = "固定的 Windows Device Portal 准备后端不可用；未修改系统。"
		return r
	}
	r, c := m.inspectReport(r)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed && !m.report.Busy && m.generation == generation {
		m.report = cloneReport(r)
		m.connection = c
	}
	return cloneReport(m.report)
}
func (m *Manager) inspectReport(r Report) (Report, *locationportal.Config) {
	fail := func(stage string, e error, backup bool) (Report, *locationportal.Config) {
		return buildFailure(r, stage, e, backup, m.recovery), nil
	}
	s, e := m.open(false)
	if errors.Is(e, os.ErrNotExist) {
		if r.HasBackup || r.Ready {
			return fail("backup", ErrStore, true)
		}
		return r, nil
	}
	if e != nil {
		return fail("backup", ErrStore, r.HasBackup)
	}
	defer s.close()
	j, e := s.load()
	if e != nil {
		return fail("backup", ErrStore, true)
	}
	if j == nil || j.Stage == "restored" {
		r := initialReport(!m.recovery)
		if j != nil {
			r.Status = "restored"
			r.Detail = "本应用准备的组件和固定入站限制已恢复。"
		}
		return r, nil
	}
	r.HasBackup = true
	r.CanRestore = true
	r.CanPrepare = !m.recovery
	if !m.recovery && j.Owner != m.owner {
		return fail("owner", ErrConflict, true)
	}
	if j.Pending != "" {
		r.Status = "cleanup_unknown"
		r.Ready = false
		r.CanPrepare = false
		r.CanRestore = false
		r.Stage = j.Pending
		r.Detail = "上次系统操作没有完整提交读回；保留备份及入站限制，需管理员核对。"
		return r, nil
	}
	if j.Stage == "ready" {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		current, err := m.backend.snapshot(ctx)
		gr, ge := m.gate.Inspect(ctx)
		if err != nil || ge != nil || current != j.Expected || !verifiedGate(gr) {
			return fail("report_readback", ErrConflict, true)
		}
		pin, err := m.backend.certificate(ctx)
		if err != nil || pin != j.CertificateSHA256 {
			return fail("certificate", ErrReadback, true)
		}
		c, err := m.backend.unseal(ctx, j.CredentialBox, j.Owner, j.TransactionID, j.CertificateSHA256)
		if err != nil || c.Port != HTTPSPort || c.CertificateSHA256 != pin {
			return fail("credentials", ErrReadback, true)
		}
		if err = m.backend.verifyConnection(ctx, c); err != nil {
			return fail("connection", err, true)
		}
		r.Status = "ready"
		r.Stage = "complete"
		r.Ready = true
		r.Busy = false
		r.HasBackup = true
		r.CanRestore = true
		r.CanPrepare = !m.recovery
		r.CapabilityInstalled = true
		r.DeveloperModeEnabled = true
		r.ServiceRunning = true
		r.AuthenticationRequired = true
		r.LoopbackGateVerified = true
		r.CertificateVerified = true
		r.Detail = "门户的固定组件、认证、入站限制与证书已重新读回；位置覆盖另行核对。"
		return r, &c
	}
	r.Status = "managed_reconnect_required"
	r.Ready = false
	r.Detail = "已有本应用准备记录；重新准备可生成新内存凭据，或恢复原状态。"
	return r, nil
}
func (m *Manager) Prepare() Report { return m.begin(false) }
func (m *Manager) Restore() Report { return m.begin(true) }
func (m *Manager) begin(restore bool) Report {
	if m == nil {
		return Report{Status: "unsupported", HTTPPort: HTTPPort, HTTPSPort: HTTPSPort}
	}
	m.mu.Lock()
	if m.closed || m.report.Busy || (!restore && m.recovery) || (!restore && m.report.Ready) {
		r := m.report
		m.mu.Unlock()
		return r
	}
	var nonce [32]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		m.mu.Unlock()
		return m.reportFailure("operation", e, false)
	}
	r := m.report
	r.Status = "preparing"
	if restore {
		r.Status = "restoring"
	}
	r.Busy = true
	r.Ready = false
	r.CanPrepare = false
	r.CanRestore = false
	r.CertificateVerified = false
	r.OperationID = hex.EncodeToString(nonce[:])
	r.Stage = "backup"
	r.NativeCode = 0
	r.Detail = "正在核对备份及系统状态；尚未确认门户就绪。"
	m.connection = nil
	m.generation++
	m.report = r
	m.done = make(chan struct{})
	done := m.done
	m.mu.Unlock()
	go func() {
		defer close(done)
		m.inspectMu.Lock()
		defer m.inspectMu.Unlock()
		defer func() {
			if recover() != nil {
				m.reportFailure("operation", ErrReadback, true)
			}
		}()
		ctx, cancel := context.WithTimeout(m.ctx, 12*time.Minute)
		defer cancel()
		if restore {
			m.restore(ctx)
		} else {
			m.prepare(ctx, r.OperationID)
		}
	}()
	return r
}
func (m *Manager) reportFailure(stage string, e error, backup bool) Report {
	r := buildFailure(m.cached(), stage, e, backup, m.recovery)
	m.mu.Lock()
	m.connection = nil
	m.mu.Unlock()
	m.publish(r)
	return r
}
func buildFailure(r Report, stage string, e error, backup, recovery bool) Report {
	r.Busy = false
	r.Ready = false
	r.CanPrepare = !backup && !recovery
	r.CanRestore = backup
	r.HasBackup = r.HasBackup || backup
	r.CertificateVerified = false
	r.Stage = stage
	r.Status = "failed"
	r.Detail = "准备未完成；未将门户状态或位置效果标为成功。"
	var native syscall.Errno
	if errors.As(e, &native) {
		r.NativeCode = uint32(native)
	}
	if errors.Is(e, ErrConflict) {
		r.Status = "external_conflict"
		r.CanPrepare = false
		r.CanRestore = false
		r.Detail = "系统状态与本应用记录不同；保留外部修改和已有备份。"
	}
	if errors.Is(e, ErrStore) {
		r.Status = "cleanup_unknown"
		r.CanPrepare = false
		r.CanRestore = false
		r.Detail = "备份或提交未确认；保留现有入站限制，禁止继续变更。"
	}
	if errors.Is(e, ErrOverrideActive) {
		r.Status = "location_restore_required"
		r.CanPrepare = false
		r.CanRestore = false
		r.Detail = "请先连接门户并恢复位置覆盖；覆盖开启或无法读取时不会停止门户或卸除组件。"
	}
	return r
}
func verifiedGate(r locationportalgate.GateReport) bool {
	return r.Status == "filters_verified" && r.Verified && r.Installed && r.LoopbackGateVerified && r.ExpectedFilters == 4 && r.InstalledFilters == 4 && r.HTTPPort == HTTPPort && r.HTTPSPort == HTTPSPort
}
func absentGate(r locationportalgate.GateReport) bool {
	return r.Status == "absent_verified" && r.Verified && !r.Installed && !r.LoopbackGateVerified && r.InstalledFilters == 0 && r.ExpectedFilters == 4 && r.HTTPPort == HTTPPort && r.HTTPSPort == HTTPSPort
}
func cleanInitial(v systemState) bool {
	return (v.Capability == 0 || v.Capability == 3) && !v.Service.Present && !v.PortalKeyPresent && v.PortalConfigSHA256 == ""
}
func fixedConfigured(v systemState) bool {
	return v.Capability == 4 && v.DeveloperMode.Present && v.DeveloperMode.Value == 1 && v.PortalKeyPresent && v.DynamicPorts == registryValue{true, 0} && v.HTTPPort == registryValue{true, HTTPPort} && v.HTTPSPort == registryValue{true, HTTPSPort} && v.Authorizer == registryValue{true, 1} && v.Service.Present
}

func (m *Manager) step(ctx context.Context, s store, j *journal, name string, change func() error, accept func(systemState, systemState) bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fresh, err := m.backend.snapshot(ctx)
	if err != nil {
		return err
	}
	if fresh != j.Expected {
		return ErrConflict
	}
	j.Pending = name
	if err = s.save(j); err != nil {
		return ErrStore
	}
	m.phase(name, j)
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = change(); err != nil {
		return err
	}
	after, err := m.backend.snapshot(ctx)
	if err != nil {
		return err
	}
	if accept != nil && !accept(j.Expected, after) {
		return ErrReadback
	}
	j.Expected = after
	j.Pending = ""
	j.Stage = name
	if s.save(j) != nil {
		return ErrStore
	}
	return nil
}
func (m *Manager) prepare(ctx context.Context, nonce string) {
	s, e := m.open(true)
	if e != nil {
		m.reportFailure("backup", e, false)
		return
	}
	defer s.close()
	j, e := s.load()
	if e != nil {
		m.reportFailure("backup", ErrStore, true)
		return
	}
	if j == nil || j.Stage == "restored" {
		before, e := m.backend.snapshot(ctx)
		if e != nil {
			m.reportFailure("plan", e, false)
			return
		}
		if !cleanInitial(before) {
			m.reportFailure("existing_component", ErrConflict, false)
			return
		}
		j = &journal{Version: 1, Owner: m.owner, TransactionID: nonce, Stage: "backed_up", Before: before, Expected: before}
		if s.save(j) != nil {
			m.reportFailure("backup", ErrStore, false)
			return
		}
	} else if j.Owner != m.owner || j.Pending != "" {
		m.reportFailure("backup", ErrConflict, true)
		return
	}
	if j.PortalWasStarted {
		m.reportFailure("existing_preparation", ErrBusy, true)
		return
	}
	// A prior successful preparation can be reconnected after helper loss using
	// its machine-DPAPI box. Never stop a portal whose current override is on.
	if j.PortalWasStarted {
		c, err := m.backend.unseal(ctx, j.CredentialBox, j.Owner, j.TransactionID, j.CertificateSHA256)
		if err != nil || !validPin(j.CertificateSHA256) {
			m.reportFailure("credentials", ErrReadback, true)
			return
		}
		off, err := m.backend.overrideDisabled(ctx, c)
		if err != nil || !off {
			m.reportFailure("location_restore", ErrOverrideActive, true)
			return
		}
	}
	fail := func(stage string, err error) bool {
		if err != nil {
			if j.Pending != "" {
				err = errors.Join(ErrStore, err)
			}
			m.reportFailure(stage, err, true)
			return true
		}
		return false
	}
	// The durable original exists before the first WFP or system mutation.
	j.Pending = "gate"
	if fail("backup", s.save(j)) {
		return
	}
	gr, e := m.gate.Ensure(ctx)
	if e == nil && !verifiedGate(gr) {
		e = ErrReadback
	}
	if fail("gate", e) {
		return
	}
	j.Pending = ""
	j.Stage = "gate_verified"
	if fail("backup", s.save(j)) {
		return
	}
	current, e := m.backend.snapshot(ctx)
	if fail("plan_readback", e) {
		return
	}
	if current != j.Expected {
		fail("plan_readback", ErrConflict)
		return
	}
	if current.Capability != 4 {
		reboot := false
		e = m.step(ctx, s, j, "capability", func() (err error) { reboot, err = m.backend.addCapability(ctx); return err }, func(_, after systemState) bool { return after.Capability == 4 || reboot })
		if fail("capability", e) {
			return
		}
		if reboot {
			r := m.cached()
			r.Status = "reboot_required"
			r.Busy = false
			r.RebootRequired = true
			r.HasBackup = true
			r.CanRestore = true
			r.Detail = "Windows 组件要求重启；未启动门户，也未移除入站限制。"
			m.publish(r)
			return
		}
	}
	if fail("ssh_components", m.step(ctx, s, j, "ssh_components", func() error { return m.backend.secureNewSSH(ctx, j.Before) }, func(_, after systemState) bool { return safeNewSSH(j.Before, after) })) {
		return
	}
	if fail("developer_mode", m.step(ctx, s, j, "developer_mode", func() error { return m.backend.setDeveloperMode(ctx, registryValue{true, 1}) }, func(_, after systemState) bool { return after.DeveloperMode == registryValue{true, 1} })) {
		return
	}
	if fail("service_stop", m.step(ctx, s, j, "service_stop", func() error { return m.backend.stopDisabled(ctx) }, func(_, after systemState) bool {
		return after.Service.Present && after.Service.State == 1 && after.Service.StartType == 4
	})) {
		return
	}
	cred, e := newCredentials()
	if fail("authentication", e) {
		return
	}
	// Seal before writing authentication, so a committed credential never
	// depends solely on this helper's lifetime. Bind the final box to the pin
	// once the service has generated its actual HTTP.sys certificate.
	preConfig := locationportal.Config{Port: HTTPSPort, Username: cred.username, Password: cred.password}
	j.CredentialBox, e = m.backend.seal(ctx, preConfig, j.Owner, j.TransactionID, "")
	if fail("credentials", e) {
		return
	}
	j.CertificateSHA256 = ""
	if fail("backup", s.save(j)) {
		return
	}
	if fail("configuration", m.step(ctx, s, j, "configuration", func() error { return m.backend.configure(ctx, cred) }, func(_, after systemState) bool {
		return fixedConfigured(after) && after.Service.State == 1 && after.Service.StartType == 4
	})) {
		return
	}
	gr, e = m.gate.Inspect(ctx)
	if e == nil && !verifiedGate(gr) {
		e = ErrReadback
	}
	if fail("gate_readback", e) {
		return
	}
	if fail("service_start", m.step(ctx, s, j, "service_start", func() error { return m.backend.start(ctx) }, func(_, after systemState) bool {
		return fixedConfigured(after) && after.Service.State == 4 && safeNewSSH(j.Before, after)
	})) {
		return
	}
	j.PortalWasStarted = true
	j.RestoreAuthorized = false
	if fail("backup", s.save(j)) {
		return
	}
	m.phase("certificate", j)
	pin, e := m.backend.certificate(ctx)
	if fail("certificate", e) {
		return
	}
	if !validPin(pin) {
		fail("certificate", ErrReadback)
		return
	}
	c := locationportal.Config{Port: HTTPSPort, CertificateSHA256: pin, Username: cred.username, Password: cred.password}
	if fail("connection", m.backend.verifyConnection(ctx, c)) {
		return
	}
	gr, e = m.gate.Inspect(ctx)
	if e == nil && !verifiedGate(gr) {
		e = ErrReadback
	}
	if fail("gate_readback", e) {
		return
	}
	current, e = m.backend.snapshot(ctx)
	if fail("final_readback", e) {
		return
	}
	if current != j.Expected {
		fail("final_readback", ErrConflict)
		return
	}
	j.Stage = "ready"
	j.CertificateSHA256 = pin
	j.CredentialBox, e = m.backend.seal(ctx, c, j.Owner, j.TransactionID, pin)
	if fail("credentials", e) {
		return
	}
	if fail("backup", s.save(j)) {
		return
	}
	r := m.cached()
	r.Status = "ready"
	r.Stage = "complete"
	r.Busy = false
	r.Ready = true
	r.HasBackup = true
	r.CanRestore = true
	r.CanPrepare = true
	r.CapabilityInstalled = true
	r.DeveloperModeEnabled = true
	r.ServiceRunning = true
	r.AuthenticationRequired = true
	r.LoopbackGateVerified = true
	r.CertificateVerified = true
	r.Detail = "固定本机门户的组件、认证、入站限制与证书已读回；此状态不代表系统位置已改变。"
	m.mu.Lock()
	if !m.closed {
		m.connection = &c
	}
	m.mu.Unlock()
	m.publish(r)
}
func safeNewSSH(before, after systemState) bool {
	for _, pair := range [][2]serviceState{{before.SSHBroker, after.SSHBroker}, {before.SSHProxy, after.SSHProxy}} {
		if pair[0].Present {
			if pair[0] != pair[1] {
				return false
			}
		} else if pair[1].Present && (pair[1].State != 1 || pair[1].StartType != 4) {
			return false
		}
	}
	return true
}
func newCredentials() (credentials, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return credentials{}, err
	}
	return credentials{"ultralocation", hex.EncodeToString(data[:])}, nil
}

func (m *Manager) restore(ctx context.Context) {
	s, e := m.open(false)
	if errors.Is(e, os.ErrNotExist) {
		r := initialReport(!m.recovery)
		r.Status = "restored"
		r.Detail = "不存在本应用准备备份；未更改系统设置或他人规则。"
		m.publish(r)
		return
	}
	if e != nil {
		m.reportFailure("backup", e, true)
		return
	}
	defer s.close()
	j, e := s.load()
	if e != nil || j == nil {
		m.reportFailure("backup", ErrStore, true)
		return
	}
	if j.Stage == "restored" {
		r := initialReport(!m.recovery)
		r.Status = "restored"
		m.publish(r)
		return
	}
	if !m.recovery && j.Owner != m.owner || j.Pending != "" {
		m.reportFailure("backup", ErrConflict, true)
		return
	}
	current, e := m.backend.snapshot(ctx)
	if e != nil || current != j.Expected {
		m.reportFailure("restore_readback", ErrConflict, true)
		return
	}
	if !j.PortalWasStarted && current == j.Before {
		j.Stage = "restored"
		j.Pending = ""
		j.Expected = current
		j.CredentialBox = nil
		j.CertificateSHA256 = ""
		if s.save(j) != nil {
			m.reportFailure("backup", ErrStore, true)
			return
		}
		r := initialReport(!m.recovery)
		r.Status = "restored"
		r.Stage = "complete"
		r.Detail = "准备未执行系统变更；临时备份已安全关闭。"
		m.publish(r)
		return
	}
	if j.PortalWasStarted && !j.RestoreAuthorized {
		unsealPin := j.CertificateSHA256
		connectionPin := j.CertificateSHA256
		if unsealPin == "" {
			connectionPin, e = m.backend.certificate(ctx)
			if e != nil || !validPin(connectionPin) {
				m.reportFailure("certificate", ErrReadback, true)
				return
			}
		}
		c, err := m.backend.unseal(ctx, j.CredentialBox, j.Owner, j.TransactionID, unsealPin)
		if err != nil || !validPin(connectionPin) {
			m.reportFailure("credentials", ErrReadback, true)
			return
		}
		c.CertificateSHA256 = connectionPin
		off, err := m.backend.overrideDisabled(ctx, c)
		if err != nil || !off {
			m.reportFailure("location_restore", ErrOverrideActive, true)
			return
		}
		j.RestoreAuthorized = true
		if s.save(j) != nil {
			m.reportFailure("backup", ErrStore, true)
			return
		}
	}
	fail := func(stage string, e error) bool {
		if e != nil {
			if j.Pending != "" {
				e = errors.Join(ErrStore, e)
			}
			m.reportFailure(stage, e, true)
			return true
		}
		return false
	}
	if fail("restore_stop", m.step(ctx, s, j, "restore_stop", func() error { return m.backend.stopDisabled(ctx) }, func(_, after systemState) bool {
		return !after.Service.Present || after.Service.State == 1 && after.Service.StartType == 4
	})) {
		return
	}
	if j.Expected.Capability == 4 {
		reboot := false
		e = m.step(ctx, s, j, "restore_capability", func() (err error) { reboot, err = m.backend.removeCapability(ctx); return err }, func(before, after systemState) bool {
			return reboot || ((after.Capability == 0 || after.Capability == 3) && !after.Service.Present && after.DeveloperMode == before.DeveloperMode)
		})
		if fail("restore_capability", e) {
			return
		}
		if reboot {
			r := m.cached()
			r.Status = "reboot_required"
			r.Busy = false
			r.RebootRequired = true
			r.CanRestore = true
			r.Detail = "组件移除要求重启；保留备份与入站限制，重启后再恢复。"
			m.publish(r)
			return
		}
	}
	if fail("restore_settings", m.step(ctx, s, j, "restore_settings", func() error { return m.backend.restoreSettings(ctx, j.Before) }, func(_, after systemState) bool { return restoredState(j.Before, after) })) {
		return
	}
	// The portal is absent/disabled and stopped before any owned gate is retired.
	m.phase("restore_gate", j)
	gr, e := m.gate.Remove(ctx)
	if e == nil && !absentGate(gr) {
		e = ErrReadback
	}
	if fail("restore_gate", e) {
		return
	}
	current, e = m.backend.snapshot(ctx)
	if fail("restore_verify", e) {
		return
	}
	if !restoredState(j.Before, current) {
		fail("restore_verify", ErrConflict)
		return
	}
	j.Stage = "restored"
	j.Expected = current
	j.Pending = ""
	j.CertificateSHA256 = ""
	j.CredentialBox = nil
	if fail("backup", s.save(j)) {
		return
	}
	r := initialReport(!m.recovery)
	r.Status = "restored"
	r.Stage = "complete"
	r.Detail = "本应用安装的门户组件及固定入站限制已恢复，原开发模式值已读回。"
	m.publish(r)
}
func restoredState(before, after systemState) bool {
	// DISM can retain metadata as Removed after removing a previously absent
	// payload. This is reported as absent capability, not byte-identical OS image.
	before.Capability = after.Capability
	return (after.Capability == 0 || after.Capability == 3) && before == after
}
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.generation++
	m.cancel()
	m.connection = nil
	m.report.Ready = false
	m.report.CanPrepare = false
	m.report.CanRestore = false
	done := m.done
	m.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
	}
}
