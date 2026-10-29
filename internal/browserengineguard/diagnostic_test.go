package browserengineguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
)

func TestGuardDiagnosticKeepsOnlyKnownStageAndNativeNumber(t *testing.T) {
	private := fmt.Errorf("PRIVATE_PATH_AND_NONCE: %w", syscall.Errno(5))
	err := refusal("owner_token_open", private)
	// Outer scope must not erase the more precise native failure.
	err = refusal("manager_unavailable", err)
	d := DiagnosticFromError(err)
	if d == nil || d.Stage != "owner_token_open" || d.NativeCode != 5 || !errors.Is(err, ErrRefused) {
		t.Fatal("precise fixed diagnostic lost", d)
	}
	raw, _ := json.Marshal(d)
	if strings.Contains(string(raw)+err.Error(), "PRIVATE") || string(raw) != `{"stage":"owner_token_open","nativeCode":5}` {
		t.Fatal("private source error escaped fixed schema")
	}
	d.Stage = "mutated"
	if DiagnosticFromError(err).Stage != "owner_token_open" {
		t.Fatal("caller changed retained diagnostic")
	}
	if d := DiagnosticFromError(ErrorWithDiagnostic(Diagnostic{Stage: "PRIVATE_PATH"})); d == nil || d.Stage != "guard_transport" || d.NativeCode != 0 {
		t.Fatal("unknown stage was not normalized")
	}
}

func TestGuardBeforeBackendRefusalHasNarrowSessionProof(t *testing.T) {
	for _, mode := range []string{"open_failure", "partial_lease", "owner_dead", "closed_manager", "missing_backend"} {
		t.Run(mode, func(t *testing.T) {
			m, lease, _, calls := fakeManager()
			switch mode {
			case "open_failure", "partial_lease":
				m.open = func(InstallRequest) (sessionLease, error) {
					*calls = append(*calls, "open")
					var partial sessionLease
					if mode == "partial_lease" {
						partial = lease
					}
					return partial, refusal("lease_job_duplicate", syscall.Errno(5))
				}
			case "owner_dead":
				m.ownerAlive = func() bool { return false }
			case "closed_manager":
				m.closed = true
			case "missing_backend":
				m.backend = nil
			}
			r, err := m.Install(validRequest())
			if err == nil || !NoInstallWasAttempted(r, validRequest().SessionID) || len(m.records) != 0 {
				t.Fatal("pre-backend refusal lost exact no-attempt proof", r, err)
			}
			joined := strings.Join(*calls, ",")
			if strings.Contains(joined, "install") || strings.Contains(joined, "verify") || strings.Contains(joined, "remove") || lease.closed != (mode == "partial_lease") {
				t.Fatal("pre-install failure invoked backend or leaked partial lease", joined)
			}
		})
	}
}

func TestGuardNoAttemptProofCannotCoverExistingOrUnknownFilters(t *testing.T) {
	id := validRequest().SessionID
	good := refusedBeforeInstall(id, "owner_token_open", syscall.Errno(5))
	for name, mutate := range map[string]func(*Result){
		"different_session":  func(r *Result) { r.SessionID = strings.Repeat("b", 64) },
		"installed":          func(r *Result) { r.Installed = true },
		"unverified":         func(r *Result) { r.Verified = false },
		"generic_absence":    func(r *Result) { r.State = "removed_after_job_empty" },
		"backend_phase":      func(r *Result) { r.Diagnostic = &Diagnostic{Stage: "wfp_install", NativeCode: 5} },
		"unknown_stage":      func(r *Result) { r.Diagnostic = &Diagnostic{Stage: "arbitrary"} },
		"missing_diagnostic": func(r *Result) { r.Diagnostic = nil },
	} {
		t.Run(name, func(t *testing.T) {
			r := good
			mutate(&r)
			if NoInstallWasAttempted(r, id) {
				t.Fatal("untrusted absence claim accepted")
			}
		})
	}
	for _, mode := range []string{"install_failure", "verify_failure", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			m, _, backend, _ := fakeManager()
			if mode == "install_failure" {
				backend.installErr = syscall.Errno(5)
			}
			if mode == "verify_failure" {
				backend.verifyErr = syscall.Errno(5)
			}
			if mode == "duplicate" {
				_, _ = m.Install(validRequest())
				m.closed = true
			}
			r, err := m.Install(validRequest())
			if err == nil || NoInstallWasAttempted(r, id) || len(m.records) != 1 {
				t.Fatal("possible installed filters were downgraded to no-attempt", r, err)
			}
		})
	}
}

func TestGuardCapabilityValidatesAvailabilitySeparately(t *testing.T) {
	for _, tc := range []struct {
		value Capability
		valid bool
	}{
		{Capability{Available: true}, true}, {UnavailableCapability(nil), true},
		{UnavailableCapability(refusal("owner_token_open", syscall.Errno(5))), true},
		{Capability{}, false}, {Capability{Available: true, Diagnostic: &Diagnostic{Stage: "owner_token_open"}}, false},
		{Capability{Diagnostic: &Diagnostic{Stage: "wfp_remove"}}, false},
		{Capability{Diagnostic: &Diagnostic{Stage: "guard_transport"}}, true},
	} {
		if ValidCapability(tc.value) != tc.valid {
			t.Fatal("ambiguous availability", tc.value)
		}
	}
}
