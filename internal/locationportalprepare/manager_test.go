package locationportalprepare

import (
	"context"
	"errors"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"github.com/Maoyangui/godusevpn/internal/locationportalgate"
	"strings"
	"testing"
	"time"
)

type fakePrepareStore struct{ j *journal }

func (s *fakePrepareStore) load() (*journal, error) {
	if s.j == nil {
		return nil, nil
	}
	v := *s.j
	return &v, nil
}
func (s *fakePrepareStore) save(j *journal) error { v := *j; s.j = &v; return nil }
func (s *fakePrepareStore) close() error          { return nil }

type fakePrepareGate struct {
	report    locationportalgate.GateReport
	removeErr error
}

func (g *fakePrepareGate) Ensure(context.Context) (locationportalgate.GateReport, error) {
	return g.report, nil
}
func (g *fakePrepareGate) Inspect(context.Context) (locationportalgate.GateReport, error) {
	return g.report, nil
}
func (g *fakePrepareGate) Remove(context.Context) (locationportalgate.GateReport, error) {
	return g.report, g.removeErr
}

type fakePrepareBackend struct {
	state     systemState
	pin       string
	cfg       locationportal.Config
	override  bool
	stopped   bool
	sealCount int
}

func (b *fakePrepareBackend) snapshot(_ context.Context) (systemState, error) { return b.state, nil }
func (b *fakePrepareBackend) addCapability(_ context.Context) (bool, error) {
	b.state.Capability = 4
	b.state.Service = serviceState{true, 3, 1}
	return false, nil
}
func (b *fakePrepareBackend) removeCapability(_ context.Context) (bool, error) {
	b.state.Capability = 3
	b.state.Service = serviceState{}
	return false, nil
}
func (b *fakePrepareBackend) setDeveloperMode(_ context.Context, v registryValue) error {
	b.state.DeveloperMode = v
	return nil
}
func (b *fakePrepareBackend) configure(_ context.Context, c credentials) error {
	b.state.PortalKeyPresent = true
	b.state.DynamicPorts = registryValue{true, 0}
	b.state.HTTPPort = registryValue{true, HTTPPort}
	b.state.HTTPSPort = registryValue{true, HTTPSPort}
	b.state.Authorizer = registryValue{true, 1}
	b.state.Service = serviceState{true, 4, 1}
	return nil
}
func (b *fakePrepareBackend) start(_ context.Context) error { b.state.Service.State = 4; return nil }
func (b *fakePrepareBackend) stopDisabled(_ context.Context) error {
	b.state.Service.StartType = 4
	b.state.Service.State = 1
	b.stopped = true
	return nil
}
func (b *fakePrepareBackend) secureNewSSH(_ context.Context, _ systemState) error { return nil }
func (b *fakePrepareBackend) restoreSettings(_ context.Context, v systemState) error {
	b.state = v
	return nil
}
func (b *fakePrepareBackend) certificate(_ context.Context) (string, error) { return b.pin, nil }
func (b *fakePrepareBackend) verifyConnection(_ context.Context, _ locationportal.Config) error {
	return nil
}
func (b *fakePrepareBackend) overrideDisabled(_ context.Context, _ locationportal.Config) (bool, error) {
	return !b.override, nil
}
func (b *fakePrepareBackend) seal(_ context.Context, c locationportal.Config, o, t, p string) ([]byte, error) {
	b.sealCount++
	return []byte("box:" + p), nil
}
func (b *fakePrepareBackend) unseal(_ context.Context, box []byte, o, t, p string) (locationportal.Config, error) {
	if !strings.HasPrefix(string(box), "box:") || strings.TrimPrefix(string(box), "box:") != p {
		return locationportal.Config{}, ErrReadback
	}
	c := b.cfg
	c.CertificateSHA256 = p
	return c, nil
}
func fakeInitialState() systemState {
	return systemState{Capability: 0, DeveloperMode: registryValue{}, PortalKeyPresent: false, DynamicPorts: registryValue{}, HTTPPort: registryValue{}, HTTPSPort: registryValue{}, Authorizer: registryValue{}, Service: serviceState{}, SSHBroker: serviceState{}, SSHProxy: serviceState{}}
}
func fakeVerifiedGate() locationportalgate.GateReport {
	return locationportalgate.GateReport{Status: "filters_verified", Installed: true, Verified: true, LoopbackGateVerified: true, ExpectedFilters: 4, InstalledFilters: 4, HTTPPort: HTTPPort, HTTPSPort: HTTPSPort}
}
func waitReport(t *testing.T, m *Manager, status string) Report {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r := m.Report()
		if r.Status == status && !r.Busy {
			return r
		}
		time.Sleep(time.Millisecond)
	}
	return m.Report()
}

func TestPreparationPreparePublishesOnlyAfterFullReadbackAndPrivateConfig(t *testing.T) {
	b := &fakePrepareBackend{state: fakeInitialState(), pin: strings.Repeat("a", 64)}
	b.cfg = locationportal.Config{Port: HTTPSPort, Username: portalSecretUser, Password: strings.Repeat("b", 64)}
	s := &fakePrepareStore{}
	g := &fakePrepareGate{report: fakeVerifiedGate()}
	m := newManager("S-1-5-21-42", false, b, g, func(bool) (store, error) { return s, nil })
	start := m.Prepare()
	if start.Status != "preparing" || !start.Busy || start.Ready {
		t.Fatal(start)
	}
	r := waitReport(t, m, "ready")
	if !r.Ready || r.Busy || !r.CertificateVerified || r.HTTPSPort != HTTPSPort {
		t.Fatal(r)
	}
	if m.Connection() == nil || m.Connection().Password == "" {
		t.Fatal("private config unavailable to helper")
	}
	raw := r.Detail + " " + r.Status
	if strings.Contains(raw, b.cfg.Password) {
		t.Fatal("secret in report")
	}
	if b.sealCount < 2 {
		t.Fatal("DPAPI final box not written")
	}
}
func TestPreparationReportAndRestoreRefuseActiveLocationBeforeStopping(t *testing.T) {
	b := &fakePrepareBackend{state: fakeInitialState(), pin: strings.Repeat("a", 64), override: true}
	b.cfg = locationportal.Config{Port: HTTPSPort, Username: portalSecretUser, Password: strings.Repeat("b", 64)}
	s := &fakePrepareStore{j: &journal{Version: 1, Owner: "S-1-5-21-42", TransactionID: strings.Repeat("1", 64), Stage: "ready", Before: fakeInitialState(), Expected: fakeInitialState(), CertificateSHA256: strings.Repeat("a", 64), CredentialBox: []byte("box:" + strings.Repeat("a", 64)), PortalWasStarted: true}}
	s.j.Expected.Capability = 4
	s.j.Expected.DeveloperMode = registryValue{true, 1}
	s.j.Expected.PortalKeyPresent = true
	s.j.Expected.DynamicPorts = registryValue{true, 0}
	s.j.Expected.HTTPPort = registryValue{true, HTTPPort}
	s.j.Expected.HTTPSPort = registryValue{true, HTTPSPort}
	s.j.Expected.Authorizer = registryValue{true, 1}
	s.j.Expected.Service = serviceState{true, 4, 4}
	b.state = s.j.Expected
	g := &fakePrepareGate{report: fakeVerifiedGate()}
	m := newManager("S-1-5-21-42", true, b, g, func(bool) (store, error) { return s, nil })
	r := m.Restore()
	r = waitReport(t, m, "location_restore_required")
	if r.Status != "location_restore_required" || b.stopped {
		t.Fatal(r)
	}
}

func TestPreparationReconnectsReadyJournalWithoutReconfiguringPortal(t *testing.T) {
	pin := strings.Repeat("a", 64)
	b := &fakePrepareBackend{state: fakeInitialState(), pin: pin}
	b.state.Capability = 4
	b.state.DeveloperMode = registryValue{true, 1}
	b.state.PortalKeyPresent = true
	b.state.DynamicPorts = registryValue{true, 0}
	b.state.HTTPPort = registryValue{true, HTTPPort}
	b.state.HTTPSPort = registryValue{true, HTTPSPort}
	b.state.Authorizer = registryValue{true, 1}
	b.state.Service = serviceState{true, 4, 4}
	b.cfg = locationportal.Config{Port: HTTPSPort, Username: portalSecretUser, Password: strings.Repeat("b", 64)}
	s := &fakePrepareStore{j: &journal{Version: 1, Owner: "S-1-5-21-42", TransactionID: strings.Repeat("1", 64), Stage: "ready", Before: fakeInitialState(), Expected: b.state, CertificateSHA256: pin, CredentialBox: []byte("box:" + pin), PortalWasStarted: true}}
	g := &fakePrepareGate{report: fakeVerifiedGate()}
	m := newManager("S-1-5-21-42", false, b, g, func(bool) (store, error) { return s, nil })
	r := waitReport(t, m, "ready")
	if r.Status != "ready" || !r.Ready || m.Connection() == nil || b.stopped || b.sealCount != 0 {
		t.Fatalf("ready journal was reconfigured instead of reconnected: report=%+v stopped=%v seals=%d", r, b.stopped, b.sealCount)
	}
}

func TestPreparationCloseCancelsLateOperationAndReportNeverClaimsReady(t *testing.T) {
	b := &fakePrepareBackend{state: fakeInitialState(), pin: strings.Repeat("a", 64)}
	s := &fakePrepareStore{}
	g := &fakePrepareGate{report: fakeVerifiedGate()}
	m := newManager("S-1-5-21-42", false, b, g, func(bool) (store, error) { return s, nil })
	m.Prepare()
	m.Close()
	r := m.Report()
	if r.Ready || r.CanPrepare || r.CanRestore {
		t.Fatal(r)
	}
	if !errors.Is(m.ctx.Err(), context.Canceled) {
		t.Fatal("manager context remained active")
	}
}
