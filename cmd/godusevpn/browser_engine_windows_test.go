//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
)

type engineGuardFakeHelper struct {
	calls  []string
	result browserengineguard.Result
	err    error
	closed bool
	during func()
}

func (h *engineGuardFakeHelper) Call(_ context.Context, method string, _ any, out any) error {
	h.calls = append(h.calls, method)
	if h.during != nil {
		h.during()
	}
	data, _ := json.Marshal(h.result)
	json.Unmarshal(data, out)
	return h.err
}
func (h *engineGuardFakeHelper) Close() error { h.closed = true; return nil }
func (h *engineGuardFakeHelper) Closed() bool { return h.closed }
func engineGuardBridgeFixture(t *testing.T) (*App, *engineGuardFakeHelper, *browserEngineGuardBridge) {
	t.Helper()
	h := &engineGuardFakeHelper{result: browserengineguard.Result{SessionID: strings.Repeat("a", 64), Installed: true, Verified: true, State: "filters_verified"}}
	a := &App{ctx: context.Background(), privacyHost: h, privacyHostEpoch: 1, privacyHostState: "connected"}
	guard, err := a.fixedBrowserEngineGuard()
	if err != nil {
		t.Fatal(err)
	}
	return a, h, guard.(*browserEngineGuardBridge)
}
func TestEngineGuardBridgeChecksFixedActionAndNativeResult(t *testing.T) {
	_, h, guard := engineGuardBridgeFixture(t)
	request := browserengineguard.InstallRequest{SessionID: h.result.SessionID, EngineID: "chrome-for-testing", Version: "130.0.6723.58", JobHandle: 42, MainPID: 1234, MainCreated: 5678, RelayPort: 31000}
	if _, err := guard.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.Report(context.Background(), request.SessionID); err != nil {
		t.Fatal(err)
	}
	h.result = browserengineguard.Result{SessionID: request.SessionID, Verified: true, State: "removed_after_job_empty"}
	if _, err := guard.Remove(context.Background(), request.SessionID); err != nil {
		t.Fatal("native job-empty cleanup result refused")
	}
	if strings.Join(h.calls, ",") != "BrowserGuardInstall,BrowserGuardReport,BrowserGuardRemove" {
		t.Fatal("bridge exposed a generalized privileged operation")
	}
	for _, bad := range []browserengineguard.Result{{SessionID: request.SessionID, Verified: true, State: "removed"}, {SessionID: request.SessionID, State: "removed_after_job_empty"}, {SessionID: request.SessionID, Installed: true, Verified: true, State: "removed_after_job_empty"}, {SessionID: strings.Repeat("b", 64), Verified: true, State: "removed_after_job_empty"}} {
		h.result = bad
		if _, err := guard.Remove(context.Background(), request.SessionID); err != errBrowserEngineRefused {
			t.Fatal("unconfirmed cleanup reported successful")
		}
	}
}
func TestEngineGuardBridgeRefusesStaleHelperAuthorityBeforeAndAfterCall(t *testing.T) {
	a, h, guard := engineGuardBridgeFixture(t)
	a.privacyHostEpoch++
	if _, err := guard.Report(context.Background(), h.result.SessionID); err != errBrowserEngineRefused || len(h.calls) != 0 {
		t.Fatal("stale helper lease used")
	}
	a, h, guard = engineGuardBridgeFixture(t)
	h.during = func() { a.privacyHostEpoch++ }
	if _, err := guard.Report(context.Background(), h.result.SessionID); err != errBrowserEngineRefused || len(h.calls) != 1 {
		t.Fatal("result from replaced helper accepted")
	}
	a.privacyHost = nil
	if _, err := a.fixedBrowserEngineGuard(); err != errBrowserEngineRefused {
		t.Fatal("missing helper triggered alternate elevation or authority")
	}
}
func TestEngineGuardBridgeUnknownResultNeverRetriesAndMarksDeadSession(t *testing.T) {
	a, h, guard := engineGuardBridgeFixture(t)
	h.err = errors.New("SYNTHETIC_PRIVATE_PATH_OR_SECRET")
	h.closed = true
	if _, err := guard.Remove(context.Background(), h.result.SessionID); err != errBrowserEngineRefused || strings.Contains(err.Error(), "SYNTHETIC") {
		t.Fatal("private error leaked")
	}
	if len(h.calls) != 1 || a.privacyHost != nil || a.privacyHostState != "lost" {
		t.Fatal("unknown mutation retried or dead authority retained")
	}
	_, h, guard = engineGuardBridgeFixture(t)
	h.err = errors.New("fixed policy refusal")
	if _, err := guard.Report(context.Background(), h.result.SessionID); err != errBrowserEngineRefused || !guard.current() {
		t.Fatal("browser policy refusal incorrectly destroyed live helper")
	}
}
func TestEngineGuardBridgeCancelledActionDoesNotCallHelper(t *testing.T) {
	_, h, guard := engineGuardBridgeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := guard.Report(ctx, h.result.SessionID); err != errBrowserEngineRefused || len(h.calls) != 0 {
		t.Fatal("cancelled operation reached privileged helper")
	}
}
