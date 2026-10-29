package main

import (
	"context"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	prep "github.com/Maoyangui/godusevpn/internal/locationportalprepare"
)

type locationPreparationHelper interface {
	LocationPortalPreparation(context.Context, string) (prep.Report, *locationportal.Config, error)
}

func (a *App) locationPortalPreparation(method string) (prep.Report, error) {
	switch method {
	case "LocationPortalPreparationReport", "LocationPortalPreparationPrepare", "LocationPortalPreparationRestore":
	default:
		return prep.Report{}, prep.ErrUnsupported
	}
	a.privacyMu.Lock()
	host, epoch, state := a.privacyHost, a.privacyHostEpoch, a.privacyHostState
	a.privacyMu.Unlock()
	if host == nil || state != "connected" {
		return prep.Report{Status: "helper_required", Detail: "准备系统组件需要先明确授权管理员助手。"}, prep.ErrUnsupported
	}
	helper, ok := host.(locationPreparationHelper)
	if !ok {
		return prep.Report{}, prep.ErrUnsupported
	}
	if method == "LocationPortalPreparationRestore" {
		m, e := a.locationPortalManager()
		if e != nil {
			return prep.Report{}, e
		}
		ctx, cancel := a.locationPortalContext()
		r, e := m.Read(ctx)
		cancel()
		if e != nil || r.Managed {
			return prep.Report{}, prep.ErrConflict
		}
	}
	ctx, cancel := a.locationPortalContext()
	defer cancel()
	r, c, e := helper.LocationPortalPreparation(ctx, method)
	a.privacyMu.Lock()
	valid := a.privacyHostEpoch == epoch && a.privacyHostState == "connected" && ctx.Err() == nil
	a.privacyMu.Unlock()
	if !valid {
		return prep.Report{}, prep.ErrClosed
	}
	if e != nil {
		return r, prep.ErrReadback
	}
	if c != nil {
		if !r.Ready || !r.AuthenticationRequired || !r.LoopbackGateVerified || !r.CertificateVerified || !r.ServiceRunning || r.HTTPSPort != prep.HTTPSPort || c.Port != prep.HTTPSPort {
			return prep.Report{}, prep.ErrReadback
		}
		if _, e = a.acceptPreparedLocationPortal(*c); e != nil {
			return r, e
		}
	}
	if r.Status == "restored" {
		m, e := a.locationPortalManager()
		if e == nil {
			m.Close()
		}
	}
	return r, nil
}
func (a *App) PrivacyLocationPortalPreparationReport() (prep.Report, error) {
	return a.locationPortalPreparation("LocationPortalPreparationReport")
}
func (a *App) PrivacyLocationPortalPrepare() (prep.Report, error) {
	a.invalidatePrivacyEvidence()
	defer a.invalidatePrivacyEvidence()
	a.privacyMu.Lock()
	// Reauthorize whenever the current helper is absent or its session state is
	// no longer connected. A stale helper pointer can survive a transport loss;
	// checking only the nil case would make the preparation button stay stuck
	// at helper_required until the application restarts.
	needs := a.privacyHost == nil || a.privacyHostState != "connected"
	a.privacyMu.Unlock()
	if needs {
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if e := a.authorizePrivacyHost(ctx, true); e != nil {
			return prep.Report{Status: "helper_required", Detail: "管理员授权未完成；没有自动重试。"}, prep.ErrUnsupported
		}
	}
	return a.locationPortalPreparation("LocationPortalPreparationPrepare")
}
func (a *App) PrivacyLocationPortalRestorePreparation() (prep.Report, error) {
	a.invalidatePrivacyEvidence()
	defer a.invalidatePrivacyEvidence()
	return a.locationPortalPreparation("LocationPortalPreparationRestore")
}
