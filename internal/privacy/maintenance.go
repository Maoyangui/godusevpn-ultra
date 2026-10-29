package privacy

import "github.com/Maoyangui/godusevpn/internal/maintenance"

func (m *Manager) WithLease(l *maintenance.Lease) *Manager {
	return &Manager{dir: m.dir, backend: m.backend, persist: m.persist, machineOnly: m.machineOnly, lockStorage: m.lockStorage, checkStore: m.checkStore, openJournal: m.openJournal, gate: m.gate, lease: l}
}
func (m *Manager) enterMaintenance(machine bool) (func(), error) {
	if !machine {
		return func() {}, nil
	}
	if m.lease != nil {
		return func() {}, m.lease.Check()
	}
	if m.gate == nil {
		return func() {}, nil
	}
	l, e := m.gate()
	if e != nil {
		return nil, e
	}
	return func() { _ = l.Close() }, nil
}
