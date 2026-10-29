package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/identity"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	prep "github.com/Maoyangui/godusevpn/internal/locationportalprepare"
)

type fakePortalPreparationHelper struct {
	calls  int
	report prep.Report
	config *locationportal.Config
	onCall func()
	closed bool
}

func (*fakePortalPreparationHelper) Call(context.Context, string, any, any) error { return nil }
func (f *fakePortalPreparationHelper) Close() error                               { f.closed = true; return nil }
func (f *fakePortalPreparationHelper) LocationPortalPreparation(context.Context, string) (prep.Report, *locationportal.Config, error) {
	f.calls++
	if f.onCall != nil {
		f.onCall()
	}
	return f.report, f.config, nil
}
func TestLocationPortalPreparationSecretsStayBackendOnly(t *testing.T) {
	backend := &fakeLocationPortal{}
	config := &locationportal.Config{Port: 58443, CertificateSHA256: strings.Repeat("a", 64), Username: "private-user-canary", Password: "private-password-canary"}
	helper := &fakePortalPreparationHelper{report: prep.Report{Status: "ready", Ready: true, AuthenticationRequired: true, LoopbackGateVerified: true, CertificateVerified: true, ServiceRunning: true, HTTPSPort: 58443}, config: config}
	a := &App{locationPortal: backend, privacyHost: helper, privacyHostState: "connected"}
	r, e := a.PrivacyLocationPortalPreparationReport()
	if e != nil || !r.Ready || backend.connects != 1 {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(r)
	for _, canary := range []string{config.Username, config.Password, config.CertificateSHA256} {
		if strings.Contains(string(raw), canary) {
			t.Fatal("private connection leaked")
		}
	}
	helper.report.LoopbackGateVerified = false
	if _, e = a.PrivacyLocationPortalPreparationReport(); e == nil || backend.connects != 1 {
		t.Fatal("unverified prepared connection accepted")
	}
	helper.report.LoopbackGateVerified = true
	helper.onCall = func() { a.privacyMu.Lock(); a.privacyHostEpoch++; a.privacyMu.Unlock() }
	if _, e = a.PrivacyLocationPortalPreparationReport(); e != prep.ErrClosed || backend.connects != 1 {
		t.Fatal("stale helper connection accepted")
	}
}
func TestLocationPortalPreparationReportNeverAuthorizes(t *testing.T) {
	a := &App{}
	if r, e := a.PrivacyLocationPortalPreparationReport(); e == nil || r.Status != "helper_required" || a.privacyHost != nil {
		t.Fatal("readonly report unexpectedly authorized")
	}
}

func TestLocationPortalPrepareReauthorizesStaleHelper(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra helper is Windows-only")
	}
	old := launchPrivacyHelper
	t.Cleanup(func() { launchPrivacyHelper = old })
	stale := &fakePortalPreparationHelper{}
	config := &locationportal.Config{Port: 58443, CertificateSHA256: strings.Repeat("a", 64), Username: "private-user", Password: "private-password"}
	fresh := &fakePortalPreparationHelper{report: prep.Report{Status: "ready", Ready: true, AuthenticationRequired: true, LoopbackGateVerified: true, CertificateVerified: true, ServiceRunning: true, HTTPSPort: 58443}, config: config}
	launches := 0
	launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) {
		launches++
		return fresh, nil
	}
	a := &App{ctx: context.Background(), privacyHost: stale, privacyHostState: "lost", locationPortal: &fakeLocationPortal{}}
	if r, err := a.PrivacyLocationPortalPrepare(); err != nil || r.Status != "ready" {
		t.Fatalf("stale helper blocked preparation: report=%+v err=%v", r, err)
	}
	if launches != 1 || !stale.closed || a.privacyHost != fresh || a.privacyHostState != "connected" {
		t.Fatalf("stale helper was not replaced: launches=%d staleClosed=%v host=%T state=%s", launches, stale.closed, a.privacyHost, a.privacyHostState)
	}
}
