package browserenginehost

import (
	"context"
	"errors"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
)

func copyGuardDiagnostic(d *browserengineguard.Diagnostic) *browserengineguard.Diagnostic {
	if !browserengineguard.ValidDiagnostic(d) {
		return nil
	}
	copy := *d
	return &copy
}

// Keep the first bounded failure, not a later cleanup error that can mask it.
// A typed transport failure takes precedence over any accompanying stale data.
func (l *lifetime) rememberGuardDiagnostic(r browserengineguard.Result, err error) {
	diagnostic := browserengineguard.DiagnosticFromError(err)
	if diagnostic == nil && r.SessionID == l.session.id {
		diagnostic = copyGuardDiagnostic(r.Diagnostic)
	}
	if diagnostic == nil {
		return
	}
	l.session.mu.Lock()
	if l.session.status.Diagnostic == nil {
		l.session.status.Diagnostic = diagnostic
	}
	l.session.mu.Unlock()
}

// Only a complete authenticated before-backend receipt can discharge the
// attempted-install cleanup obligation. Production RPC strips this proof on a
// transport/decode/context/epoch error. Reject inconsistent errors here too.
func (l *lifetime) recordInstallResponse(r browserengineguard.Result, err error) {
	l.rememberGuardDiagnostic(r, err)
	if !browserengineguard.NoInstallWasAttempted(r, l.session.id) {
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	if err != nil {
		diagnostic := browserengineguard.DiagnosticFromError(err)
		if diagnostic == nil || *diagnostic != *r.Diagnostic {
			return
		}
	}
	l.guardAttempted = false
}

func startupErrorWithDiagnostic(err error, session *Session) error {
	if session == nil {
		return err
	}
	diagnostic := session.Status().Diagnostic
	if diagnostic == nil {
		return err
	}
	if err == nil {
		err = ErrRefused
	}
	return errors.Join(err, browserengineguard.ErrorWithDiagnostic(*diagnostic))
}
