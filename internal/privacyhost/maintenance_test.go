package privacyhost

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/maintenance"
)

func TestMaintenanceOnlyDispatcherHasNoPolicyOrFirewallChannel(t *testing.T) {
	d, p, f := fixtureDispatcher()
	d.maintenanceOnly = true
	for _, method := range []string{"Report", "Apply", "Restore", "RestoreAll", "FirewallReport", "FirewallApply", "FirewallRemove", "Execute"} {
		if _, e := d.call(method, raw(nil)); e == nil {
			t.Fatal("bootstrap accepted", method)
		}
	}
	if _, e := d.call("MaintenanceReady", raw(map[string]string{"directory": "arbitrary"})); e == nil {
		t.Fatal("bootstrap accepted parameters")
	}
	r, e := d.call("MaintenanceReady", raw(nil))
	b, _ := json.Marshal(r)
	if e != nil || string(b) != `{"ready":true,"scope":"synchronization_only"}` {
		t.Fatal(string(b), e)
	}
	if len(p.calls) != 0 || len(f.calls) != 0 {
		t.Fatal("bootstrap accessed managers", p.calls, f.calls)
	}
	d.maintenanceOnly = false
	if _, e = d.call("MaintenanceReady", raw(nil)); e == nil {
		t.Fatal("normal helper accepted bootstrap channel")
	}
}

func TestDispatcherBusyMaintenancePrecedesAllWriteManagers(t *testing.T) {
	for _, method := range []string{"Apply", "Restore", "RestoreAll", "FirewallApply", "FirewallRemove"} {
		d, p, f := fixtureDispatcher()
		d.gate = func() (*maintenance.Lease, error) { return nil, maintenance.ErrBusy }
		if _, e := d.call(method, raw(nil)); !errors.Is(e, maintenance.ErrBusy) {
			t.Fatal(method, e)
		}
		if len(p.calls) != 0 || len(f.calls) != 0 {
			t.Fatal("busy entered manager", method)
		}
	}
	d, _, _ := fixtureDispatcher()
	d.gate = func() (*maintenance.Lease, error) { t.Fatal("read acquired mutation gate"); return nil, nil }
	for _, method := range []string{"Report", "FirewallReport"} {
		if _, e := d.call(method, raw(nil)); e != nil {
			t.Fatal(e)
		}
	}
}
