package browserengineguard

import "sync"

type sessionLease interface {
	Programs() []string
	Active() (uint32, error)
	Terminate() error
	Close()
}
type firewall interface {
	install(InstallRequest, []string) error
	verify(InstallRequest, []string) (bool, error)
	remove(InstallRequest, []string) error
}
type record struct {
	request InstallRequest
	lease   sessionLease
}
type Manager struct {
	mu         sync.Mutex
	open       func(InstallRequest) (sessionLease, error)
	ownerAlive func() bool
	backend    firewall
	records    map[string]*record
	closed     bool
	closeOwner func()
}

func (m *Manager) Install(request InstallRequest) (Result, error) {
	result := Result{SessionID: request.SessionID, State: "refused"}
	if validate(request) != nil {
		result.Diagnostic = failureDiagnostic("request_validation", nil)
		return result, ErrRefused
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.records[request.SessionID] != nil {
		result.Diagnostic = failureDiagnostic("existing_session", nil)
		return result, ErrRefused
	}
	if m.closed || m.open == nil || m.backend == nil || m.ownerAlive == nil {
		return refusedBeforeInstall(request.SessionID, "manager_unavailable", nil), ErrRefused
	}
	if !m.ownerAlive() {
		return refusedBeforeInstall(request.SessionID, "owner_unavailable", nil), ErrRefused
	}
	lease, err := m.open(request)
	if err != nil || lease == nil {
		if lease != nil {
			lease.Close()
		}
		return refusedBeforeInstall(request.SessionID, "lease_code_pin", err), ErrRefused
	}
	if m.records == nil {
		m.records = map[string]*record{}
	}
	entry := &record{request, lease}
	m.records[request.SessionID] = entry
	if err = m.backend.install(request, lease.Programs()); err != nil {
		result.Diagnostic = failureDiagnostic("wfp_install", err)
		lease.Terminate()
		result.State = "unknown_filters_retained"
		return result, ErrUnknown
	}
	present, err := m.backend.verify(request, lease.Programs())
	if err != nil || !present {
		result.Diagnostic = failureDiagnostic("wfp_verify", err)
		lease.Terminate()
		result.State = "unknown_filters_retained"
		return result, ErrUnknown
	}
	result.Installed = true
	result.Verified = true
	result.State = "filters_verified"
	return result, nil
}
func (m *Manager) Report(id string) (Result, error) {
	result := Result{SessionID: id, State: "unknown_filters_retained"}
	if !sessionPattern.MatchString(id) {
		return result, ErrRefused
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.records[id]
	if m.closed || entry == nil || m.ownerAlive == nil || !m.ownerAlive() {
		result.Diagnostic = failureDiagnostic("guard_report", nil)
		return result, ErrUnknown
	}
	present, err := m.backend.verify(entry.request, entry.lease.Programs())
	if err != nil || !present {
		result.Diagnostic = failureDiagnostic("wfp_verify", err)
		entry.lease.Terminate()
		return result, ErrUnknown
	}
	result.Installed = true
	result.Verified = true
	result.State = "filters_verified"
	return result, nil
}
func (m *Manager) Remove(id string) (Result, error) {
	result := Result{SessionID: id, State: "unknown_filters_retained"}
	if !sessionPattern.MatchString(id) {
		return result, ErrRefused
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeLocked(id)
}
func (m *Manager) removeLocked(id string) (Result, error) {
	result := Result{SessionID: id, State: "unknown_filters_retained"}
	entry := m.records[id]
	if entry == nil {
		result.Diagnostic = failureDiagnostic("guard_report", nil)
		return result, ErrUnknown
	}
	active, err := entry.lease.Active()
	if err != nil {
		result.Diagnostic = failureDiagnostic("job_query", err)
		return result, ErrUnknown
	}
	if active != 0 {
		result.Diagnostic = failureDiagnostic("job_active", nil)
		result.State = "job_active_filters_retained"
		return result, ErrActive
	}
	if err = m.backend.remove(entry.request, entry.lease.Programs()); err != nil {
		result.Diagnostic = failureDiagnostic("wfp_remove", err)
		return result, ErrUnknown
	}
	present, err := m.backend.verify(entry.request, entry.lease.Programs())
	if err != nil || present {
		result.Diagnostic = failureDiagnostic("wfp_verify", err)
		return result, ErrUnknown
	}
	entry.lease.Close()
	delete(m.records, id)
	result.Verified = true
	result.State = "removed_after_job_empty"
	return result, nil
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	for id, entry := range m.records {
		entry.lease.Terminate()
		m.removeLocked(id)
	}
	// Unknown jobs retain persistent WFP filters. Closing the helper's duplicate
	// Job handle is not reported as proof that another holder's job has exited.
	for _, entry := range m.records {
		entry.lease.Close()
	}
	if m.closeOwner != nil {
		m.closeOwner()
	}
}
