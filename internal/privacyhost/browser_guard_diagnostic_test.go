package privacyhost

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
)

type diagnosticBrowserGuard struct {
	result browserengineguard.Result
	calls  int
}

func (g *diagnosticBrowserGuard) Install(browserengineguard.InstallRequest) (browserengineguard.Result, error) {
	g.calls++
	return g.result, errors.New("PRIVATE_PATH_AND_NONCE")
}
func (g *diagnosticBrowserGuard) Report(string) (browserengineguard.Result, error) {
	panic("unexpected Report")
}
func (g *diagnosticBrowserGuard) Remove(string) (browserengineguard.Result, error) {
	panic("unexpected Remove")
}
func (g *diagnosticBrowserGuard) Close() {}

func TestBrowserGuardFactoryFailureIsReadOnlyDistinctCapability(t *testing.T) {
	d, p, f := fixtureDispatcher()
	d.browserDiagnostic = &browserengineguard.Diagnostic{Stage: "owner_token_open", NativeCode: 5}
	d.gate = func() (*maintenance.Lease, error) { t.Fatal("status acquired mutation gate"); return nil, nil }
	value, err := d.call("BrowserGuardStatus", raw(nil))
	capability, ok := value.(browserengineguard.Capability)
	if err != nil || !ok || capability.Available || capability.Diagnostic.Stage != "owner_token_open" || capability.Diagnostic.NativeCode != 5 || len(p.calls)+len(f.calls) != 0 {
		t.Fatal("missing factory failure", value, err)
	}
	if _, err = d.call("BrowserGuardStatus", raw(map[string]any{"path": "private"})); err == nil {
		t.Fatal("status accepted arbitrary params")
	}
	d.maintenanceOnly = true
	if _, err = d.call("BrowserGuardStatus", raw(nil)); err == nil {
		t.Fatal("maintenance-only helper acquired GUI browser status authority")
	}
	if executeElevated("BrowserGuardStatus", nil, nil, func() bool { return true }, func() (dispatcher, error) { t.Fatal("CLI acquired GUI capability"); return d, nil }) == nil {
		t.Fatal("CLI status allowed")
	}
	d.maintenanceOnly = false
	d.gate = nil
	request := browserGuardFixture()
	value, err = d.call("BrowserGuardInstall", raw(request))
	if err != nil || !browserengineguard.NoInstallWasAttempted(value.(browserengineguard.Result), request.SessionID) {
		t.Fatal("factory refusal was converted to unknown filters", value, err)
	}
	value, err = d.call("BrowserGuardRemove", raw(request.SessionID))
	if err == nil || value.(browserengineguard.Result).Verified {
		t.Fatal("missing manager claimed filter cleanup")
	}
}

func TestBrowserGuardFramedKnownInstallFailuresKeepDiagnosticNotRawError(t *testing.T) {
	for _, state := range []string{"install_not_attempted", "unknown_filters_retained", "refused"} {
		t.Run(state, func(t *testing.T) {
			d, _, _ := fixtureDispatcher()
			stage := "wfp_install"
			if state == "install_not_attempted" {
				stage = "lease_job_duplicate"
			}
			if state == "refused" {
				stage = "existing_session"
			}
			g := &diagnosticBrowserGuard{result: browserengineguard.Result{SessionID: browserGuardFixture().SessionID, Verified: state == "install_not_attempted", State: state, Diagnostic: &browserengineguard.Diagnostic{Stage: stage, NativeCode: 5}}}
			d.browser = g
			ui, host := net.Pipe()
			done := make(chan error, 1)
			go func() { done <- serve(host, strings.Repeat("b", 64), d, func() error { return nil }) }()
			client := newClient(ui, strings.Repeat("b", 64), nil)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var result browserengineguard.Result
			err := client.Call(ctx, "BrowserGuardInstall", browserGuardFixture(), &result)
			client.Close()
			<-done
			raw, _ := json.Marshal(result)
			if err != nil || result.Diagnostic == nil || *result.Diagnostic != *g.result.Diagnostic || result.SessionID != g.result.SessionID || g.calls != 1 || strings.Contains(string(raw), "PRIVATE") {
				t.Fatal("fixed failure evidence lost at frame boundary", result, err)
			}
			if browserengineguard.NoInstallWasAttempted(result, browserGuardFixture().SessionID) != (state == "install_not_attempted") {
				t.Fatal("unknown result gained absence proof")
			}
		})
	}
}
