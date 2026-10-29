package privacyhost

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/privacy"
)

type fakePolicy struct {
	calls      []string
	report     privacy.Report
	writeError error
}

func (f *fakePolicy) Report() (privacy.Report, error) {
	f.calls = append(f.calls, "Report")
	return f.report, nil
}
func (f *fakePolicy) Apply(id string) (privacy.ActionResult, error) {
	f.calls = append(f.calls, "Apply:"+id)
	return privacy.ActionResult{ID: id, Status: "configured", Changed: true}, f.writeError
}
func (f *fakePolicy) Restore(id string) (privacy.ActionResult, error) {
	f.calls = append(f.calls, "Restore:"+id)
	return privacy.ActionResult{ID: id, Status: "restored"}, nil
}

type fakeFirewall struct{ calls []string }

func (f *fakeFirewall) Report() (appfirewall.Report, error) {
	f.calls = append(f.calls, "Report")
	return appfirewall.Report{Supported: true, Enforcement: "not_live_tested"}, nil
}
func (f *fakeFirewall) Apply(r appfirewall.Rule) (appfirewall.Result, error) {
	f.calls = append(f.calls, "Apply:"+r.Program)
	return appfirewall.Result{ID: r.ID, Applied: true, State: "filters_installed", Rule: &r}, nil
}
func (f *fakeFirewall) Remove(id string) (appfirewall.Result, error) {
	f.calls = append(f.calls, "Remove:"+id)
	return appfirewall.Result{ID: id, State: "removed"}, nil
}
func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func fixtureDispatcher() (dispatcher, *fakePolicy, *fakeFirewall) {
	p := &fakePolicy{report: privacy.Report{Admin: true, BackupHealthy: true, Controls: []privacy.ControlState{
		{ID: machineIDs[0], RequiresAdmin: true, Scope: "machine / Windows", Managed: true},
		{ID: "chrome-webrtc", RequiresAdmin: false, Scope: "current-user / Chrome", Managed: true},
		{ID: machineIDs[1], RequiresAdmin: false, Scope: "current-user", Managed: true},
		{ID: machineIDs[2], RequiresAdmin: true, Scope: "machine / Windows", Managed: false},
	}, Capabilities: []privacy.Capability{{ID: "other-user-fingerprint"}}}}
	f := &fakeFirewall{}
	return dispatcher{policy: p, firewall: f}, p, f
}

func TestDispatcherRejectsUnknownAndUserScopeWithoutCallingManagers(t *testing.T) {
	for _, tc := range []struct {
		method string
		params json.RawMessage
	}{
		{"Execute", raw("cmd.exe")}, {"OpenURL", raw("https://example.com")}, {"Apply", raw("chrome-webrtc")}, {"Restore", raw("edge-camera")},
		{"Apply", raw(map[string]any{"id": machineIDs[0], "path": "HKCU"})}, {"RestoreAll", raw(map[string]any{"scope": "user"})},
		{"FirewallApply", raw(map[string]any{"program": "test.exe", "mode": "block_all", "shell": "evil"})},
	} {
		t.Run(tc.method+string(tc.params), func(t *testing.T) {
			d, p, f := fixtureDispatcher()
			if _, err := d.call(tc.method, tc.params); err == nil {
				t.Fatal("accepted disallowed operation")
			}
			if len(p.calls) != 0 || len(f.calls) != 0 {
				t.Fatalf("called manager: %v %v", p.calls, f.calls)
			}
		})
	}
}

func TestMachineReportAndRestoreAllCannotExposeOrRestoreUserScope(t *testing.T) {
	d, p, _ := fixtureDispatcher()
	result, err := d.call("Report", raw(nil))
	if err != nil {
		t.Fatal(err)
	}
	report := result.(privacy.Report)
	if len(report.Controls) != 2 || len(report.Capabilities) != 0 {
		t.Fatalf("unfiltered helper report: %+v", report)
	}
	p.calls = nil
	result, err = d.call("RestoreAll", raw(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.calls, []string{"Report", "Restore:" + machineIDs[0]}) {
		t.Fatal(p.calls)
	}
	if len(result.([]privacy.ActionResult)) != 1 {
		t.Fatal(result)
	}
}

func TestDamagedBackupCannotLookLikeNothingToRestore(t *testing.T) {
	d, p, _ := fixtureDispatcher()
	p.report.BackupHealthy = false
	p.report.Controls = nil
	if _, err := d.call("RestoreAll", raw(nil)); err == nil {
		t.Fatal("damaged backup was treated as clean no-op")
	}
	if !reflect.DeepEqual(p.calls, []string{"Report"}) {
		t.Fatal("attempted restoring untrusted backup", p.calls)
	}
}

func TestFramingRejectsOversizeTruncationUnknownFieldsAndTrailingJSON(t *testing.T) {
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], maxRequest+1)
	if err := readFrame(bytes.NewReader(h[:]), new(request), maxRequest); err == nil {
		t.Fatal("accepted oversize frame")
	}
	if err := strictDecode([]byte(`{"version":1,"arbitraryCommand":"bad"}`), new(request)); err == nil {
		t.Fatal("accepted unknown request field")
	}
	if err := strictDecode([]byte(`{} {}`), new(request)); err == nil {
		t.Fatal("accepted trailing JSON")
	}
	var b bytes.Buffer
	if err := writeFrame(&b, request{Version: 1, ID: 1}, maxRequest); err != nil {
		t.Fatal(err)
	}
	truncated := b.Bytes()[:b.Len()-1]
	if err := readFrame(bytes.NewReader(truncated), new(request), maxRequest); err == nil {
		t.Fatal("accepted truncated frame")
	}
}

func TestClientServerProtocolAndCommittedResultWithError(t *testing.T) {
	d, p, f := fixtureDispatcher()
	p.writeError = errors.New("readback failed after commit")
	ui, host := net.Pipe()
	session := strings.Repeat("a", 64)
	done := make(chan error, 1)
	go func() { done <- serve(host, session, d, func() error { return nil }) }()
	c := newClient(ui, session, nil)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var result privacy.ActionResult
	err := c.Call(ctx, "Apply", machineIDs[0], &result)
	if err == nil || !result.Changed || result.ID != machineIDs[0] {
		t.Fatalf("lost committed evidence: %+v %v", result, err)
	}
	var report appfirewall.Report
	if err = c.Call(ctx, "FirewallReport", nil, &report); err != nil || report.Enforcement != "not_live_tested" {
		t.Fatal(report, err)
	}
	if len(f.calls) != 1 {
		t.Fatal(f.calls)
	}
	c.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("helper did not stop when pipe closed")
	}
}

func TestEveryRequestChecksParentAndRejectsSessionOrReplay(t *testing.T) {
	for _, kind := range []string{"session", "replay", "parent"} {
		t.Run(kind, func(t *testing.T) {
			d, p, _ := fixtureDispatcher()
			ui, host := net.Pipe()
			defer ui.Close()
			session := strings.Repeat("b", 64)
			checks := 0
			done := make(chan error, 1)
			go func() {
				done <- serve(host, session, d, func() error {
					checks++
					if kind == "parent" && checks == 2 {
						return errors.New("parent exited")
					}
					return nil
				})
			}()
			_ = ui.SetDeadline(time.Now().Add(time.Second))
			first := request{Version: 1, Session: session, ID: 1, Method: "Report", Params: raw(nil)}
			if err := writeFrame(ui, first, maxRequest); err != nil {
				t.Fatal(err)
			}
			var res response
			if err := readFrame(ui, &res, maxResponse); err != nil {
				t.Fatal(err)
			}
			second := first
			second.ID = 2
			second.Method = "Apply"
			second.Params = raw(machineIDs[0])
			if kind == "session" {
				second.Session = strings.Repeat("c", 64)
			}
			if kind == "replay" {
				second.ID = 1
			}
			if err := writeFrame(ui, second, maxRequest); err != nil {
				t.Fatal(err)
			}
			if err := readFrame(ui, &res, maxResponse); err == nil {
				t.Fatal("unauthorized request received response")
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("missing identity error")
				}
			case <-time.After(time.Second):
				t.Fatal("server stuck")
			}
			if !reflect.DeepEqual(p.calls, []string{"Report"}) {
				t.Fatal(p.calls)
			}
		})
	}
}

func TestCanceledClientCallAndUnknownMethodNeverSendRequests(t *testing.T) {
	ui, host := net.Pipe()
	defer host.Close()
	c := newClient(ui, strings.Repeat("d", 64), nil)
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Call(ctx, "Apply", machineIDs[0], nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := c.Call(context.Background(), "Execute", "anything", nil); err == nil {
		t.Fatal("accepted unknown method")
	}
}

func TestTimeoutClosesSessionAndDoesNotAutoRetryWrite(t *testing.T) {
	ui, host := net.Pipe()
	defer host.Close()
	c := newClient(ui, strings.Repeat("e", 64), nil)
	defer c.Close()
	go func() { var req request; _ = readFrame(host, &req, maxRequest); time.Sleep(80 * time.Millisecond) }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.Call(ctx, "Apply", machineIDs[0], nil); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("did not mark indeterminate result: %v", err)
	}
	if err := c.Call(context.Background(), "Apply", machineIDs[0], nil); !errors.Is(err, net.ErrClosed) {
		t.Fatal("session stayed usable after unknown write result", err)
	}
}

func TestDirectExecutionRequiresElevationAndSameFixedDispatcher(t *testing.T) {
	for _, tc := range []struct {
		method      string
		admin       bool
		wantFactory bool
	}{
		{"Execute", true, false}, {"MaintenanceReady", true, false}, {"RestoreAll", false, false}, {"Apply", true, true},
	} {
		t.Run(tc.method, func(t *testing.T) {
			d, p, f := fixtureDispatcher()
			factoryCalls := 0
			create := func() (dispatcher, error) { factoryCalls++; return d, nil }
			err := executeElevated(tc.method, "chrome-webrtc", nil, func() bool { return tc.admin }, create)
			if err == nil {
				t.Fatal("accepted unauthorized direct operation")
			}
			if (factoryCalls > 0) != tc.wantFactory {
				t.Fatal("unexpected privileged initialization", factoryCalls)
			}
			if len(p.calls) != 0 || len(f.calls) != 0 {
				t.Fatal("unauthorized manager access", p.calls, f.calls)
			}
		})
	}
	d, p, _ := fixtureDispatcher()
	p.writeError = errors.New("verification failed after change")
	var result privacy.ActionResult
	err := executeElevated("Apply", machineIDs[0], &result, func() bool { return true }, func() (dispatcher, error) { return d, nil })
	if err == nil || !result.Changed || result.ID != machineIDs[0] {
		t.Fatal("direct command lost committed result", result, err)
	}
	p.calls = nil
	var restored []privacy.ActionResult
	if err = executeElevated("RestoreAll", nil, &restored, func() bool { return true }, func() (dispatcher, error) { return d, nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.calls, []string{"Report", "Restore:" + machineIDs[0]}) {
		t.Fatal("direct command bypassed machine scope", p.calls)
	}
}
