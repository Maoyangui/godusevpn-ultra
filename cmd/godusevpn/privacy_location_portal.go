package main

import (
	"context"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"time"
)

type locationPortalBackend interface {
	Connect(context.Context, locationportal.Config) (locationportal.Report, error)
	Read(context.Context) (locationportal.Report, error)
	Apply(context.Context, locationportal.ApplyRequest) (locationportal.Report, error)
	Observe(context.Context) (locationportal.Report, error)
	Restore(context.Context) (locationportal.Report, error)
	Stop()
	CancelObservation()
	Close()
}

var newLocationPortalBackend = func() locationPortalBackend { return locationportal.New(runLocationConsumer) }

func (a *App) locationPortalManager() (locationPortalBackend, error) {
	a.locationPortalMu.Lock()
	defer a.locationPortalMu.Unlock()
	if a.locationPortalClosed {
		return nil, locationportal.ErrConnection
	}
	if a.locationPortal == nil {
		a.locationPortal = newLocationPortalBackend()
	}
	if a.locationPortal == nil {
		return nil, locationportal.ErrUnsupported
	}
	return a.locationPortal, nil
}
func (a *App) locationPortalContext() (context.Context, context.CancelFunc) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, 20*time.Second)
}
func (a *App) PrivacyLocationPortalReport() (locationportal.Report, error) {
	m, e := a.locationPortalManager()
	if e != nil {
		return locationportal.Report{}, e
	}
	ctx, cancel := a.locationPortalContext()
	defer cancel()
	return m.Read(ctx)
}
func (a *App) PrivacyLocationPortalConnect(configJSON string) (locationportal.Report, error) {
	c, e := locationportal.ParseConfig(configJSON)
	if e != nil {
		return locationportal.Report{}, e
	}
	return a.acceptPreparedLocationPortal(c)
}

// Only trusted backend preparation hands credentials to this function. Its
// returned Report contains no credentials and is safe for the ordinary UI.
func (a *App) acceptPreparedLocationPortal(c locationportal.Config) (locationportal.Report, error) {
	m, e := a.locationPortalManager()
	if e != nil {
		return locationportal.Report{}, e
	}
	ctx, cancel := a.locationPortalContext()
	defer cancel()
	return m.Connect(ctx, c)
}
func (a *App) PrivacyLocationPortalApply(requestJSON string) (locationportal.Report, error) {
	r, e := locationportal.ParseApply(requestJSON)
	if e != nil {
		return locationportal.Report{}, e
	}
	m, e := a.locationPortalManager()
	if e != nil {
		return locationportal.Report{}, e
	}
	a.invalidatePrivacyEvidence()
	defer a.invalidatePrivacyEvidence()
	ctx, cancel := a.locationPortalContext()
	defer cancel()
	return m.Apply(ctx, r)
}
func (a *App) PrivacyLocationPortalObserve() (locationportal.Report, error) {
	a.locationObserveMu.Lock()
	if a.locationObserveCancelPending {
		a.locationObserveCancelPending = false
		a.locationObserveMu.Unlock()
		return locationportal.Report{}, locationportal.ErrReadback
	}
	a.locationObserveActive = true
	a.locationObserveMu.Unlock()
	defer func() { a.locationObserveMu.Lock(); a.locationObserveActive = false; a.locationObserveMu.Unlock() }()
	m, e := a.locationPortalManager()
	if e != nil {
		return locationportal.Report{}, e
	}
	base := a.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 110*time.Second)
	defer cancel()
	return m.Observe(ctx)
}
func (a *App) PrivacyLocationPortalCancelObservation() {
	a.locationObserveMu.Lock()
	if a.locationObserveActive {
		a.locationObserveCancelPending = true
	}
	a.locationObserveMu.Unlock()
	a.locationPortalMu.Lock()
	m := a.locationPortal
	a.locationPortalMu.Unlock()
	if m != nil {
		m.CancelObservation()
	}
}
func (a *App) PrivacyLocationPortalRestore() (locationportal.Report, error) {
	m, e := a.locationPortalManager()
	if e != nil {
		return locationportal.Report{}, e
	}
	a.invalidatePrivacyEvidence()
	defer a.invalidatePrivacyEvidence()
	ctx, cancel := a.locationPortalContext()
	defer cancel()
	return m.Restore(ctx)
}
func (a *App) PrivacyLocationPortalStopTrajectory() (locationportal.Report, error) {
	m, e := a.locationPortalManager()
	if e != nil {
		return locationportal.Report{}, e
	}
	m.Stop()
	ctx, cancel := a.locationPortalContext()
	defer cancel()
	return m.Read(ctx)
}
func (a *App) shutdownLocationPortal() {
	a.locationPortalMu.Lock()
	a.locationPortalClosed = true
	m := a.locationPortal
	a.locationPortal = nil
	a.locationPortalMu.Unlock()
	if m != nil {
		m.Close()
	}
}
