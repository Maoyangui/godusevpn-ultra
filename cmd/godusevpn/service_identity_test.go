package main

import (
	"errors"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/ipc"
)

func TestUnconfirmedControlReadPreservesProtectionAsUnknownHistory(t *testing.T) {
	for _, tc := range []struct {
		err  error
		hint string
	}{{ipc.ErrUntrustedService, "identity"}, {ipc.ErrResponseMismatch, "response"}} {
		t.Run(tc.hint, func(t *testing.T) {
			before := UIState{Service: true, SvcState: "running", CanRegister: true, Up: 4, Down: 7, TotalUp: 10, TotalDown: 20}
			before.View.State.Status = "connected"
			before.View.State.Wanted = true
			before.View.Guard = "on"
			before.View.Mode = "global"
			before.View.Settings.NoDirect = true
			after := serviceReadState(before, ipc.StateView{}, tc.err, "running")
			if after.Service || after.SvcState != serviceIdentityUnknown || after.SvcHint != tc.hint || after.CanRegister || after.View.State.Status != "unknown" {
				t.Fatal("uncertainty was misreported")
			}
			if after.View.Guard != "on" || !after.View.State.Wanted || !after.View.Settings.NoDirect || after.View.Mode != "global" || before.View.State.Status != "connected" {
				t.Fatal("historical protection or source snapshot mutated")
			}
			if after.Up != 0 || after.Down != 0 || after.TotalUp != 10 || after.TotalDown != 20 {
				t.Fatal("stale rates or session totals misrepresented")
			}
			a := &App{state: after}
			// No Wails context: any attempted IPC, SCM or elevation path would
			// fail/panic rather than being quietly exercised by this test.
			if err := a.RepairService(); !errors.Is(err, tc.err) {
				t.Fatal("repair was not blocked before side effects", err)
			}
			observed := before.View
			confirmed := serviceReadState(after, observed, nil, "running")
			if !confirmed.Service || confirmed.SvcHint != "" || confirmed.SvcState != "running" || confirmed.View.State.Status != "connected" {
				t.Fatal("fresh confirmation did not replace uncertainty")
			}
		})
	}
}

func TestQuitUnconfirmedOrFailedDisconnectNeverStopsService(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		st                       UIState
		disconnectErr            error
		wantDisconnect, wantStop int
	}{
		{"identity_unknown", UIState{Service: true, SvcState: serviceIdentityUnknown}, nil, 0, 0},
		{"no_permission", UIState{Service: true, SvcState: "no-permission"}, nil, 0, 0},
		{"unreachable", UIState{Service: false, SvcState: "running"}, nil, 0, 0},
		{"auth_changed", UIState{Service: true}, ipc.ErrUntrustedService, 1, 0},
		{"response_mismatch", UIState{Service: true}, ipc.ErrResponseMismatch, 1, 0},
		{"result_lost", UIState{Service: true}, errors.New("fixture result unknown"), 1, 0},
		{"confirmed_quit", UIState{Service: true}, nil, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			disconnects, stops, closed := 0, 0, 0
			quitServiceActions(tc.st, func() error { disconnects++; return tc.disconnectErr }, func() { stops++ }, func() { closed++ })
			if disconnects != tc.wantDisconnect || stops != tc.wantStop || closed != 1 {
				t.Fatalf("disconnect=%d stop=%d close=%d", disconnects, stops, closed)
			}
		})
	}
}

func TestNetworkRecoveryUsesFreshReadAndOnlyDefiniteAbsenceAllowsOffline(t *testing.T) {
	for _, tc := range []struct {
		name            string
		err             error
		guard           string
		scm             string
		online, offline int
	}{
		{"untrusted", ipc.ErrUntrustedService, "", "stopped", 0, 0}, {"mismatch", ipc.ErrResponseMismatch, "", "stopped", 0, 0},
		{"permission", ipc.ErrNoPermission, "", "stopped", 0, 0}, {"unknown_error", errors.New("fixture unreadable"), "", "stopped", 0, 0},
		{"stopped", ipc.ErrNoService, "", "stopped", 0, 1}, {"absent", ipc.ErrNoService, "", "not-installed", 0, 1},
		{"wails_absent_bridge", serviceDownError{}, "", "not-installed", 0, 1},
		{"transport_unknown", ipc.ErrNoService, "", "unknown", 0, 0}, {"transport_busy_live", ipc.ErrNoService, "", "running", 0, 0},
		{"transport_starting", ipc.ErrNoService, "", "starting", 0, 0}, {"transport_stopping", ipc.ErrNoService, "", "stopping", 0, 0},
		{"verified_guard", nil, "on", "", 1, 0}, {"verified_no_guard", nil, "", "", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, scmReads, online, offline := 0, 0, 0, 0
			restoreNetworkActions(func() (ipc.StateView, error) { reads++; return ipc.StateView{Guard: tc.guard}, tc.err }, func() string { scmReads++; return tc.scm }, func(ipc.StateView) { online++ }, func() { offline++ })
			if reads != 1 || online != tc.online || offline != tc.offline {
				t.Fatalf("read=%d online=%d offline=%d", reads, online, offline)
			}
			if (scmReads == 1) != errors.Is(tc.err, ipc.ErrNoService) {
				t.Fatal("unexpected SCM query")
			}
		})
	}
}

func TestTransportFailureWhileServiceExistenceUnknownDoesNotInviteRepair(t *testing.T) {
	for _, scm := range []string{"unknown", "running", "starting", "stopping"} {
		next := serviceReadState(UIState{}, ipc.StateView{}, ipc.ErrNoService, scm)
		if next.SvcState != serviceIdentityUnknown || next.View.State.Status != "unknown" || next.CanRegister {
			t.Fatal("transport failure guessed a stopped service")
		}
	}
	for _, scm := range []string{"stopped", "not-installed"} {
		next := serviceReadState(UIState{}, ipc.StateView{}, ipc.ErrNoService, scm)
		if next.SvcState != scm || next.View.State.Status != "disconnected" {
			t.Fatal("definite SCM absence lost")
		}
	}
}
