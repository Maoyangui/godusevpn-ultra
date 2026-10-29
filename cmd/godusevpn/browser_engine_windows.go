//go:build windows

package main

import (
	"context"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
)

type browserEngineGuardBridge struct {
	app    *App
	helper privacyHelper
	epoch  uint64
}

func (a *App) fixedBrowserEngineGuard() (browserenginehost.Guard, error) {
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	if a.ctx == nil || a.ctx.Err() != nil || a.privacyHost == nil || a.privacyHostState != "connected" {
		return nil, errBrowserEngineRefused
	}
	if a.privacyBrowserGuard != nil && !a.privacyBrowserGuard.Available {
		diagnostic := a.privacyBrowserGuard.Diagnostic
		if !browserengineguard.ValidDiagnostic(diagnostic) {
			diagnostic = &browserengineguard.Diagnostic{Stage: "guard_transport"}
		}
		return nil, browserengineguard.ErrorWithDiagnostic(*diagnostic)
	}
	return &browserEngineGuardBridge{app: a, helper: a.privacyHost, epoch: a.privacyHostEpoch}, nil
}
func (g *browserEngineGuardBridge) current() bool {
	g.app.privacyMu.Lock()
	defer g.app.privacyMu.Unlock()
	return g.app.ctx != nil && g.app.ctx.Err() == nil && g.app.privacyHost == g.helper && g.app.privacyHostEpoch == g.epoch && g.app.privacyHostState == "connected"
}
func (g *browserEngineGuardBridge) call(ctx context.Context, method string, params any) (browserengineguard.Result, error) {
	var result browserengineguard.Result
	if ctx == nil || ctx.Err() != nil || !g.current() {
		return result, errBrowserEngineRefused
	}
	bounded, stop := context.WithTimeout(ctx, 25*time.Second)
	defer stop()
	err := g.helper.Call(bounded, method, params, &result)
	if err != nil {
		// Only a successfully framed known refusal may certify no mutation.
		// A local decode/transport error can leave a partially decoded result.
		result.Verified = false
		if result.State == "install_not_attempted" {
			result.State = "unknown_filters_retained"
		}
		result.Diagnostic = &browserengineguard.Diagnostic{Stage: "guard_transport"}
		// Never retry an install/remove after an unknown result. The helper's
		// peer-loss path terminates its held Job and removes persistent rules
		// only after it independently observes an empty process tree.
		if closed, ok := g.helper.(interface{ Closed() bool }); ok && closed.Closed() {
			g.app.privacyMu.Lock()
			if g.app.privacyHost == g.helper && g.app.privacyHostEpoch == g.epoch {
				g.app.privacyHost = nil
				g.app.privacyHostState = "lost"
				g.app.privacyHostError = "管理员助手失联；本次浏览器结果未知，没有重试或撤销VPN保护。"
				g.app.privacyEvidenceEpoch++
				g.app.privacySnapshot, g.app.privacyExport = nil, nil
			}
			g.app.privacyMu.Unlock()
		}
		return result, errBrowserEngineRefused
	}
	if !g.current() || bounded.Err() != nil {
		result.Verified = false
		if result.State == "install_not_attempted" {
			result.State = "unknown_filters_retained"
		}
		result.Diagnostic = &browserengineguard.Diagnostic{Stage: "guard_transport"}
		return result, errBrowserEngineRefused
	}
	return result, nil
}
func (g *browserEngineGuardBridge) Install(ctx context.Context, request browserengineguard.InstallRequest) (browserengineguard.Result, error) {
	result, err := g.call(ctx, "BrowserGuardInstall", request)
	if err == nil && result.SessionID == request.SessionID && browserengineguard.ValidDiagnostic(result.Diagnostic) && (browserengineguard.NoInstallWasAttempted(result, request.SessionID) || !result.Verified) {
		return result, browserengineguard.ErrorWithDiagnostic(*result.Diagnostic)
	}
	if err != nil || result.SessionID != request.SessionID || !result.Installed || !result.Verified || result.State != "filters_verified" {
		return result, errBrowserEngineRefused
	}
	return result, nil
}
func (g *browserEngineGuardBridge) Report(ctx context.Context, id string) (browserengineguard.Result, error) {
	result, err := g.call(ctx, "BrowserGuardReport", id)
	if err != nil || result.SessionID != id || !result.Installed || !result.Verified || result.State != "filters_verified" {
		return result, errBrowserEngineRefused
	}
	return result, nil
}
func (g *browserEngineGuardBridge) Remove(ctx context.Context, id string) (browserengineguard.Result, error) {
	result, err := g.call(ctx, "BrowserGuardRemove", id)
	if err != nil || result.SessionID != id || result.Installed || !result.Verified || result.State != "removed_after_job_empty" {
		return result, errBrowserEngineRefused
	}
	return result, nil
}
func nativeBrowserEngineDependencies(app *App) browserEngineDependencies {
	return browserEngineDependencies{manager: newBrowserEngineManager, guard: app.fixedBrowserEngineGuard, start: func(ctx context.Context, manager *browserengine.Manager, config browserenginehost.Config, guard browserenginehost.Guard) (browserEngineSession, error) {
		session, err := browserenginehost.Start(ctx, manager, config, guard)
		if session == nil {
			return nil, err
		}
		return session, err
	}}
}
