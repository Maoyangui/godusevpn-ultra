package timeprivacy

import (
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"testing"
)

func TestTimezoneGateBusyPreventsSetAndRestore(t *testing.T) {
	m, f := fakeManager(t)
	m.gate = func() (*maintenance.Lease, error) { return nil, maintenance.ErrBusy }
	m.ready = func() (bool, error) { return false, maintenance.ErrUnavailable }
	if _, e := m.Apply("China Standard Time"); e == nil || f.sets != 0 {
		t.Fatal("busy clock write")
	}
	if _, e := m.Restore(); e == nil || f.sets != 0 {
		t.Fatal("busy clock restore")
	}
	r, e := m.Report()
	if e != nil || r.CanApply {
		t.Fatal("missing gate advertised apply permission")
	}
}
