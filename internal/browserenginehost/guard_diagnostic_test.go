package browserenginehost

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
)

func TestInstallNotAttemptedDischargesCleanupOnlyWithCompleteMatchingProof(t *testing.T) {
	for _, mode := range []string{"complete", "complete-no-error", "transport-error", "cancelled", "joined-cancel", "joined-deadline", "wrong-session", "not-verified", "already-installed", "wrong-state", "wfp-stage", "invalid-stage", "different-typed-error"} {
		t.Run(mode, func(t *testing.T) {
			var calls []string
			life := fixtureLifetime(&calls, nil, ErrRefused)
			diagnostic := browserengineguard.Diagnostic{Stage: "lease_directory", NativeCode: 5}
			reply := browserengineguard.Result{SessionID: life.session.id, State: "install_not_attempted", Verified: true, Diagnostic: &diagnostic}
			err := browserengineguard.ErrorWithDiagnostic(diagnostic)
			proven := mode == "complete" || mode == "complete-no-error"
			switch mode {
			case "complete-no-error":
				err = nil
			case "transport-error":
				err = errors.New("fixed transport failure")
			case "cancelled":
				err = context.Canceled
			case "joined-cancel":
				err = errors.Join(context.Canceled, err)
			case "joined-deadline":
				err = errors.Join(context.DeadlineExceeded, err)
			case "wrong-session":
				reply.SessionID = strings.Repeat("b", 64)
			case "not-verified":
				reply.Verified = false
			case "already-installed":
				reply.Installed = true
			case "wrong-state":
				reply.State = "unknown_filters_retained"
			case "wfp-stage":
				diagnostic.Stage = "wfp_install"
				err = browserengineguard.ErrorWithDiagnostic(diagnostic)
			case "invalid-stage":
				diagnostic.Stage = "SECRET_PATH_OR_STAGE"
				err = ErrRefused
			case "different-typed-error":
				err = browserengineguard.ErrorWithDiagnostic(browserengineguard.Diagnostic{Stage: "guard_transport", NativeCode: 109})
			}
			life.recordInstallResponse(reply, err)
			if life.guardAttempted == proven {
				t.Fatal("install obligation discharged without matching proof", mode)
			}
			closeErr := life.close()
			removed := strings.Contains(strings.Join(calls, ","), "remove-and-verify-absent")
			if proven {
				if closeErr != nil || removed || life.session.Status().State != "closed" || !strings.Contains(life.session.Status().Detail, "未调用撤销") {
					t.Fatal(calls, closeErr, life.session.Status())
				}
				if !strings.Contains(strings.Join(calls, ","), "code-pins") {
					t.Fatal("confirmed no-attempt did not release code usage")
				}
			} else {
				if closeErr == nil || !removed || life.session.Status().State != "closed_filters_unknown" || strings.Contains(strings.Join(calls, ","), "code-pins") {
					t.Fatal("unknown install/cleanup lost its obligation", calls, closeErr)
				}
				unresolvedLifetimes.Lock()
				delete(unresolvedLifetimes.items, life.session.id)
				unresolvedLifetimes.Unlock()
			}
		})
	}
}

func TestGuardDiagnosticKeepsOriginalCauseAndCopiesStatus(t *testing.T) {
	var calls []string
	life := fixtureLifetime(&calls, nil, nil)
	first := browserengineguard.Diagnostic{Stage: "lease_code_seal", NativeCode: 5}
	life.rememberGuardDiagnostic(browserengineguard.Result{SessionID: life.session.id, Diagnostic: &first}, nil)
	first.Stage = "SECRET_MUTATED_STAGE"
	life.rememberGuardDiagnostic(browserengineguard.Result{SessionID: life.session.id, Diagnostic: &browserengineguard.Diagnostic{Stage: "wfp_remove", NativeCode: 6}}, ErrRefused)
	status := life.session.Status()
	if status.Diagnostic == nil || status.Diagnostic.Stage != "lease_code_seal" || status.Diagnostic.NativeCode != 5 {
		t.Fatal(status)
	}
	status.Diagnostic.Stage = "SECRET_RETURN_MUTATION"
	if life.session.Status().Diagnostic.Stage != "lease_code_seal" {
		t.Fatal("Status exposed diagnostic storage")
	}
	wrapped := startupErrorWithDiagnostic(ErrGraphicsReadback, life.session)
	if !errors.Is(wrapped, ErrGraphicsReadback) || browserengineguard.DiagnosticFromError(wrapped).Stage != "lease_code_seal" {
		t.Fatal(wrapped)
	}
	if life.close() != nil {
		t.Fatal("fixture cleanup")
	}
}

func TestInvalidOrForeignGuardDiagnosticNeverPublished(t *testing.T) {
	for _, diagnostic := range []browserengineguard.Diagnostic{{Stage: "SECRET_PATH", NativeCode: 5}, {Stage: "lease_directory", NativeCode: 5}} {
		var calls []string
		life := fixtureLifetime(&calls, nil, nil)
		life.rememberGuardDiagnostic(browserengineguard.Result{SessionID: strings.Repeat("b", 64), Diagnostic: &diagnostic}, nil)
		if life.session.Status().Diagnostic != nil {
			t.Fatal("foreign or invalid result published")
		}
		if life.close() != nil {
			t.Fatal("fixture cleanup")
		}
	}
}
