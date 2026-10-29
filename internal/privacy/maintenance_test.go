package privacy

import (
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"testing"
)

func TestMachinePolicyGateBusyStopsBeforeStoreOrRegistryMutation(t *testing.T) {
	m, f := fakeManager(t)
	calls := 0
	m.gate = func() (*maintenance.Lease, error) { calls++; return nil, maintenance.ErrBusy }
	if _, e := m.Apply("windows-advertising-id"); e == nil || f.writes != 0 || calls != 1 {
		t.Fatal("busy apply reached machine mutation")
	}
	if _, e := m.Restore("windows-advertising-id"); e == nil || f.writes != 0 || calls != 2 {
		t.Fatal("busy restore reached machine mutation")
	}
	if release, e := m.enterMaintenance(false); e != nil {
		t.Fatal("user policy coupled to machine gate")
	} else {
		release()
	}
	if calls != 2 {
		t.Fatal("user scope acquired machine gate")
	}
}
