package appfirewall

import (
	"errors"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/maintenance"
)

func TestBusyGatePreventsFirewallWritesButNotReport(t *testing.T) {
	r := temporaryRule(t)
	b := &fakeBackend{admin: true, items: fakeFilters(r)}
	m := &Manager{backend: b, gate: func() (*maintenance.Lease, error) { return nil, maintenance.ErrBusy }}
	if _, e := m.Apply(r); !errors.Is(e, maintenance.ErrBusy) {
		t.Fatal(e)
	}
	if _, e := m.Remove(r.ID); !errors.Is(e, maintenance.ErrBusy) {
		t.Fatal(e)
	}
	if b.mutations != 0 {
		t.Fatal("busy gate changed WFP")
	}
	if _, e := m.Report(); e != nil {
		t.Fatal("read was gated", e)
	}
}

func TestRemoveUnknownDeterministicOrUnrelatedKeysNeverCompletes(t *testing.T) {
	for _, key := range []string{"app-1:out-v4", "unrelated-key", ""} {
		r := temporaryRule(t)
		unknown := installed{Key: key, Unknown: true}
		b := &fakeBackend{admin: true, items: append(fakeFilters(r), unknown)}
		result, e := (&Manager{backend: b}).Remove(r.ID)
		if e == nil || !result.Applied || result.Verified || result.State != "unknown_remnants" {
			t.Fatal(result, e)
		}
		if len(b.items) != 1 || !b.items[0].Unknown {
			t.Fatal("unknown object deleted")
		}
	}
}

func TestRemovePostCommitReadErrorPreservesUnverifiedResult(t *testing.T) {
	r := temporaryRule(t)
	b := &fakeBackend{admin: true, items: fakeFilters(r), afterWriteReadError: true}
	result, e := (&Manager{backend: b}).Remove(r.ID)
	if e == nil || !result.Applied || result.Verified || result.State != "removed_unverified" {
		t.Fatal(result, e)
	}
	if len(b.items) != 0 {
		t.Fatal("fake commit missing")
	}
}
