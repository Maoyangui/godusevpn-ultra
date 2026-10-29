package main

import (
	"errors"

	"github.com/Maoyangui/godusevpn/internal/ipc"
)

const serviceIdentityUnknown = "identity-unknown"

// Keep the existing exact Wails error code while preserving the transport
// sentinel for local recovery decisions. Text alone must never pick a route.
type serviceDownError struct{}

func (serviceDownError) Error() string { return "SERVICE_DOWN" }
func (serviceDownError) Unwrap() error { return ipc.ErrNoService }

// This changes only the UI's evidence snapshot, never the daemon's connection,
// desired state, guard or adapters. Historical configuration remains available.
func serviceReadState(previous UIState, observed ipc.StateView, readErr error, scmState string) UIState {
	next := previous
	next.Service, next.SvcState = readErr == nil, scmState
	next.SvcHint, next.CanRegister = "", false
	if readErr == nil {
		next.View = observed
		return next
	}
	next.Up, next.Down = 0, 0
	transportUnknown := errors.Is(readErr, ipc.ErrNoService) && scmState != "stopped" && scmState != "not-installed"
	if errors.Is(readErr, ipc.ErrUntrustedService) || errors.Is(readErr, ipc.ErrResponseMismatch) || transportUnknown {
		next.SvcState, next.SvcHint = serviceIdentityUnknown, "identity"
		if errors.Is(readErr, ipc.ErrResponseMismatch) {
			next.SvcHint = "response"
		}
		next.View.State.Status = "unknown"
		return next
	}
	next.View.State.Status = "disconnected"
	return next
}

func (a *App) markServiceUnconfirmed(reason error) {
	a.mu.Lock()
	a.state = serviceReadState(a.state, ipc.StateView{}, reason, "unknown")
	a.mu.Unlock()
}

func unconfirmedServiceError(st UIState) error {
	if st.SvcHint == "response" {
		return ipc.ErrResponseMismatch
	}
	return ipc.ErrUntrustedService
}

func restoreNetworkActions(read func() (ipc.StateView, error), queryService func() string, online func(ipc.StateView), offline func()) {
	view, err := read()
	if err == nil {
		if view.Guard != "" || view.GuardError != "" {
			online(view)
		}
		return
	}
	if errors.Is(err, ipc.ErrNoService) {
		// Transport timeouts/busy pipes also produce ErrNoService. Only a
		// fresh, definite SCM result may select explicit offline recovery.
		if status := queryService(); status == "stopped" || status == "not-installed" {
			offline()
		}
	}
}

// An unconfirmed/unreachable service cannot be stopped merely because the user
// closes this UI. A failed disconnect has an unknown result, so no second
// service mutation follows it. Closing the UI never means restoring a guard.
func quitServiceActions(st UIState, disconnect func() error, stop, closeUI func()) {
	if !st.Service || st.SvcState == "no-permission" || st.SvcState == serviceIdentityUnknown {
		closeUI()
		return
	}
	if err := disconnect(); err != nil {
		closeUI()
		return
	}
	stop()
	closeUI()
}
