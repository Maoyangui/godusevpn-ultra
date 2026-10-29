package privacyhost

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
)

type fakeBrowserGuard struct {
	calls   []string
	request browserengineguard.InstallRequest
	err     error
}

func (g *fakeBrowserGuard) Install(request browserengineguard.InstallRequest) (browserengineguard.Result, error) {
	g.calls = append(g.calls, "install")
	g.request = request
	return browserengineguard.Result{SessionID: request.SessionID, Installed: true, Verified: true, State: "protected"}, g.err
}
func (g *fakeBrowserGuard) Report(id string) (browserengineguard.Result, error) {
	g.calls = append(g.calls, "report")
	return browserengineguard.Result{SessionID: id, Installed: true, Verified: true, State: "protected"}, g.err
}
func (g *fakeBrowserGuard) Remove(id string) (browserengineguard.Result, error) {
	g.calls = append(g.calls, "remove")
	return browserengineguard.Result{SessionID: id, State: "removed"}, g.err
}
func (g *fakeBrowserGuard) Close() {}
func browserGuardFixture() browserengineguard.InstallRequest {
	return browserengineguard.InstallRequest{SessionID: strings.Repeat("a", 64), EngineID: "chrome-for-testing", Version: "130.0.6723.58", JobHandle: 42, MainPID: 1234, MainCreated: 5678, RelayPort: 31001}
}
func TestBrowserGuardOnlyFixedOperationsAndOriginContext(t *testing.T) {
	d, p, f := fixtureDispatcher()
	g := &fakeBrowserGuard{}
	d.browser = g
	request := browserGuardFixture()
	for _, method := range []string{"BrowserGuardInstall", "BrowserGuardReport", "BrowserGuardRemove"} {
		params := raw(request.SessionID)
		if method == "BrowserGuardInstall" {
			params = raw(request)
		}
		result, err := d.call(method, params)
		if err != nil || result.(browserengineguard.Result).SessionID != request.SessionID {
			t.Fatal("fixed guard operation failed")
		}
	}
	if !reflect.DeepEqual(g.calls, []string{"install", "report", "remove"}) || g.request != request || len(p.calls) != 0 || len(f.calls) != 0 {
		t.Fatal("guard operation crossed into machine policy or app firewall")
	}
	called := false
	if executeElevated("BrowserGuardInstall", request, nil, func() bool { return true }, func() (dispatcher, error) { called = true; return d, nil }) == nil || called {
		t.Fatal("CLI without originating GUI context got browser guard authority")
	}
	for _, method := range []string{"BrowserGuardApplyProvider", "BrowserGuardDeleteFilter", "Execute", "OpenURL"} {
		if _, err := d.call(method, raw(request)); err == nil {
			t.Fatal("arbitrary privileged method accepted")
		}
	}
}
func TestBrowserGuardRefusesIdentityPathAndClaimInjection(t *testing.T) {
	d, _, _ := fixtureDispatcher()
	g := &fakeBrowserGuard{}
	d.browser = g
	request := browserGuardFixture()
	data, _ := json.Marshal(request)
	for _, field := range []string{`"provider":"arbitrary"`, `"filterId":123`, `"path":"C:/arbitrary.exe"`, `"root":"C:/Users/Administrator"`, `"sid":"S-1-5-18"`, `"userName":"administrator"`, `"closed":true`, `"pids":[]`, `"flags":["no-sandbox"]`, `"SESSIONID":"` + request.SessionID + `"`} {
		input := append([]byte("{"+field+","), data[1:]...)
		if _, err := d.call("BrowserGuardInstall", input); err == nil {
			t.Fatal("privileged browser injection accepted")
		}
	}
	for _, mutate := range []func(*browserengineguard.InstallRequest){func(r *browserengineguard.InstallRequest) { r.SessionID = "../path" }, func(r *browserengineguard.InstallRequest) { r.JobHandle = 0 }, func(r *browserengineguard.InstallRequest) { r.MainPID = 0 }, func(r *browserengineguard.InstallRequest) { r.MainCreated = 0 }, func(r *browserengineguard.InstallRequest) { r.RelayPort = 0 }, func(r *browserengineguard.InstallRequest) { r.EngineID = "cmd" }} {
		bad := request
		mutate(&bad)
		if _, err := d.call("BrowserGuardInstall", raw(bad)); err == nil {
			t.Fatal("incomplete runtime authority accepted")
		}
	}
	for _, method := range []string{"BrowserGuardReport", "BrowserGuardRemove"} {
		for _, params := range []json.RawMessage{raw("unknown"), raw(map[string]any{"sessionId": request.SessionID, "closed": true}), raw(strings.ToUpper(request.SessionID))} {
			if _, err := d.call(method, params); err == nil {
				t.Fatal("untrusted cleanup evidence accepted")
			}
		}
	}
	if len(g.calls) != 0 {
		t.Fatal("invalid inputs reached native WFP/Job boundary")
	}
}
func TestBrowserGuardErrorsRemainFixedAndMaintenanceModeCannotMutate(t *testing.T) {
	d, _, _ := fixtureDispatcher()
	g := &fakeBrowserGuard{err: errors.New("private user folder or secret must not escape")}
	d.browser = g
	result, err := d.call("BrowserGuardInstall", raw(browserGuardFixture()))
	if err != errBrowserGuardRefused || result == nil {
		t.Fatal("unknown result evidence lost or raw error leaked")
	}
	d.maintenanceOnly = true
	for _, method := range []string{"BrowserGuardInstall", "BrowserGuardReport", "BrowserGuardRemove"} {
		if _, err := d.call(method, raw(browserGuardFixture())); err == nil {
			t.Fatal("maintenance-only helper acquired browser privileges")
		}
	}
	if len(g.calls) != 1 {
		t.Fatal("maintenance-only mode reached browser manager")
	}
	d.maintenanceOnly = false
	d.browser = nil
	if _, err := d.call("BrowserGuardReport", raw(browserGuardFixture().SessionID)); err != errBrowserGuardRefused {
		t.Fatal("absent native manager looked supported")
	}
}
func TestBrowserGuardClientCannotSendArbitraryMethod(t *testing.T) {
	client := newClient(nil, strings.Repeat("a", 64), nil)
	if client.Call(context.Background(), "BrowserGuardInstallArbitrary", nil, nil) == nil {
		t.Fatal("client accepted unknown capability")
	}
}
