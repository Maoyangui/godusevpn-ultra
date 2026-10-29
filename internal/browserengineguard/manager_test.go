package browserengineguard

import (
	"errors"
	"strings"
	"testing"
)

type fakeLease struct {
	calls              *[]string
	active             uint32
	activeErr          error
	terminated, closed bool
}

func (f *fakeLease) Programs() []string {
	return []string{`C:\fixture\chrome.exe`, `C:\fixture\helper.exe`}
}
func (f *fakeLease) Active() (uint32, error) {
	*f.calls = append(*f.calls, "active")
	return f.active, f.activeErr
}
func (f *fakeLease) Terminate() error {
	*f.calls = append(*f.calls, "terminate")
	f.terminated = true
	return nil
}
func (f *fakeLease) Close() { *f.calls = append(*f.calls, "close"); f.closed = true }

type fakeFirewall struct {
	calls                            *[]string
	installed                        bool
	installErr, verifyErr, removeErr error
	programs                         int
}

func (f *fakeFirewall) install(_ InstallRequest, p []string) error {
	*f.calls = append(*f.calls, "install")
	f.programs = len(p)
	f.installed = true
	return f.installErr
}
func (f *fakeFirewall) verify(_ InstallRequest, _ []string) (bool, error) {
	*f.calls = append(*f.calls, "verify")
	return f.installed, f.verifyErr
}
func (f *fakeFirewall) remove(_ InstallRequest, _ []string) error {
	*f.calls = append(*f.calls, "remove")
	if f.removeErr == nil {
		f.installed = false
	}
	return f.removeErr
}
func fakeManager() (*Manager, *fakeLease, *fakeFirewall, *[]string) {
	calls := []string{}
	lease := &fakeLease{calls: &calls, active: 1}
	backend := &fakeFirewall{calls: &calls}
	m := &Manager{records: map[string]*record{}, ownerAlive: func() bool { return true }, backend: backend, open: func(InstallRequest) (sessionLease, error) { calls = append(calls, "open"); return lease, nil }}
	return m, lease, backend, &calls
}
func validRequest() InstallRequest {
	return InstallRequest{SessionID: strings.Repeat("a", 64), EngineID: "chrome-for-testing", Version: "156.0.8078.47", JobHandle: 123, MainPID: 456, MainCreated: 789, RelayPort: 49152}
}

func TestGuardLifecycleRequiresIndependentEmptyJobThenAbsence(t *testing.T) {
	m, lease, backend, calls := fakeManager()
	request := validRequest()
	result, err := m.Install(request)
	if err != nil || !result.Verified || !result.Installed || backend.programs != 2 || strings.Join(*calls, ",") != "open,install,verify" {
		t.Fatal("install did not verify complete program set")
	}
	if _, err = m.Install(request); err == nil {
		t.Fatal("duplicate mutation permitted")
	}
	*calls = nil
	if _, err = m.Remove(request.SessionID); !errors.Is(err, ErrActive) || backend.installed == false || strings.Join(*calls, ",") != "active" {
		t.Fatal("active Job lost its guard")
	}
	lease.active = 0
	*calls = nil
	result, err = m.Remove(request.SessionID)
	if err != nil || !result.Verified || result.Installed || result.State != "removed_after_job_empty" || strings.Join(*calls, ",") != "active,remove,verify,close" {
		t.Fatal("remove ordering/readback wrong")
	}
}
func TestGuardUnknownAndPeerLossKeepFiltersUntilZero(t *testing.T) {
	for _, stage := range []string{"install", "verify"} {
		m, lease, backend, calls := fakeManager()
		if stage == "install" {
			backend.installErr = ErrUnknown
		} else {
			backend.verifyErr = ErrUnknown
		}
		if _, err := m.Install(validRequest()); !errors.Is(err, ErrUnknown) || !lease.terminated || lease.closed || !backend.installed {
			t.Fatal("unknown install failed open")
		}
		*calls = nil
		m.Close()
		if strings.Join(*calls, ",") != "terminate,active,close" || !backend.installed {
			t.Fatal("peer loss removed active filters")
		}
	}
	m, lease, backend, calls := fakeManager()
	request := validRequest()
	_, _ = m.Install(request)
	backend.verifyErr = ErrUnknown
	*calls = nil
	if _, err := m.Report(request.SessionID); !errors.Is(err, ErrUnknown) || !lease.terminated || strings.Join(*calls, ",") != "verify,terminate" {
		t.Fatal("lost guard did not revoke Job")
	}
	backend.verifyErr = nil
	lease.active = 0
	*calls = nil
	m.Close()
	if strings.Join(*calls, ",") != "terminate,active,remove,verify,close" || backend.installed {
		t.Fatal("peer loss cleanup did not require zero and absence")
	}
	m, lease, backend, _ = fakeManager()
	_, _ = m.Install(request)
	lease.active = 0
	lease.activeErr = ErrUnknown
	m.Close()
	if !backend.installed || !lease.closed {
		t.Fatal("unknown active count must retain persistent rules")
	}
}
