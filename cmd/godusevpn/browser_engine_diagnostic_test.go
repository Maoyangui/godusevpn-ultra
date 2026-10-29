package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
)

func TestEngineControllerRetainsBoundedFactoryDiagnosticAndCopiesHistory(t *testing.T) {
	c, _, starts := engineFixture(t)
	c.deps.guard = func() (browserenginehost.Guard, error) {
		return nil, browserengineguard.ErrorWithDiagnostic(browserengineguard.Diagnostic{Stage: "owner_token_open", NativeCode: 5})
	}
	result, err := c.launch(engineConfigFixture, engineProxyFixture)
	if err == nil || result.Code != "helper_required" || result.Status.State != "start_failed" || result.Status.Diagnostic == nil || result.Status.Diagnostic.Stage != "owner_token_open" || result.Status.Diagnostic.NativeCode != 5 || starts.Load() != 0 {
		t.Fatal(result, err)
	}
	result.Status.Diagnostic.Stage = "SECRET_RETURN_MUTATION"
	if c.status().Status.Diagnostic.Stage != "owner_token_open" {
		t.Fatal("history diagnostic storage escaped")
	}
}
func TestEngineControllerKeepsStartupDiagnosticOverLaterCleanup(t *testing.T) {
	c, session, _ := engineFixture(t)
	session.status.State = "closed_filters_unknown"
	session.status.Ready = false
	session.status.CanNavigate = false
	session.status.Diagnostic = &browserengineguard.Diagnostic{Stage: "wfp_remove", NativeCode: 6}
	session.closeErr = errors.New("synthetic cleanup unknown")
	c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
		return session, browserengineguard.ErrorWithDiagnostic(browserengineguard.Diagnostic{Stage: "lease_receipt", NativeCode: 5})
	}
	result, err := c.launch(engineConfigFixture, engineProxyFixture)
	if err == nil || result.Status.State != "closed_filters_unknown" || result.Status.Diagnostic == nil || result.Status.Diagnostic.Stage != "lease_receipt" || c.current == nil {
		t.Fatal(result, err)
	}
	result.Status.Diagnostic.NativeCode = 999
	if c.status().Status.Diagnostic.NativeCode != 5 {
		t.Fatal("active status exposed diagnostic storage")
	}
}
func TestEngineBrowserActionsInvalidateDetectionEvidenceBeforeWork(t *testing.T) {
	for _, action := range []struct {
		name string
		call func(*App)
	}{
		{"launch", func(a *App) { _, _ = a.BrowserEngineProfileLaunch(engineConfigFixture, engineProxyFixture) }},
		{"begin", func(a *App) { _, _ = a.BrowserEngineProfileBeginLaunch(engineConfigFixture, engineProxyFixture) }},
		{"cancel", func(a *App) { _, _ = a.BrowserEngineProfileCancelStart("invalid") }},
		{"close", func(a *App) { _ = a.BrowserEngineProfileClose() }},
	} {
		t.Run(action.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			a := &App{ctx: ctx, privacyEvidenceEpoch: 7, privacySnapshot: &privacyDetectionSnapshot{}, privacyExport: &privacyExportSnapshot{}}
			action.call(a)
			if a.privacyEvidenceEpoch != 8 || a.privacySnapshot != nil || a.privacyExport != nil || a.browserEngine != nil {
				t.Fatal("stale detection evidence survived browser action")
			}
		})
	}
}
