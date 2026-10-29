package browserengineguard

import (
	"errors"
	"syscall"
)

// Diagnostic is bounded local failure evidence. It never contains an error
// string, identity, nonce, proxy, command line, URL or filesystem path.
// NativeCode==0 means no native error number was available, not success.
type Diagnostic struct {
	Stage      string `json:"stage"`
	NativeCode uint32 `json:"nativeCode"`
}

type Capability struct {
	Available  bool        `json:"available"`
	Diagnostic *Diagnostic `json:"diagnostic,omitempty"`
}

var preInstallStages = map[string]bool{
	"manager_unavailable": true, "owner_unavailable": true,
	"owner_process_id": true, "owner_process_times": true, "owner_process_open": true, "owner_process_identity": true,
	"owner_token_open": true, "owner_token_identity": true, "owner_token_elevation": true, "owner_known_folder": true, "owner_local_drive": true,
	"owner_debug_thread": true, "owner_debug_token": true, "owner_debug_duplicate": true, "owner_debug_identity": true,
	"owner_debug_enable": true, "owner_debug_attach": true, "owner_debug_open": true, "owner_debug_revert": true, "owner_debug_retire": true,
	"lease_job_duplicate": true, "lease_job_limits": true, "lease_job_processes": true, "lease_process_open": true,
	"lease_process_identity": true, "lease_directory": true, "lease_image": true, "lease_receipt": true, "lease_code_pin": true, "lease_code_seal": true,
}

func ValidDiagnostic(d *Diagnostic) bool {
	if d == nil {
		return false
	}
	if preInstallStages[d.Stage] {
		return true
	}
	switch d.Stage {
	case "request_validation", "existing_session", "wfp_install", "wfp_verify", "wfp_remove", "job_query", "job_active", "guard_report", "guard_transport":
		return true
	}
	return false
}

type diagnosticError struct{ value Diagnostic }

func (e *diagnosticError) Error() string { return ErrRefused.Error() }
func (e *diagnosticError) Unwrap() error { return ErrRefused }

func ErrorWithDiagnostic(d Diagnostic) error {
	if !ValidDiagnostic(&d) {
		d = Diagnostic{Stage: "guard_transport"}
	}
	return &diagnosticError{d}
}

func refusal(stage string, err error) error {
	if d := DiagnosticFromError(err); d != nil {
		return ErrorWithDiagnostic(*d)
	}
	d := Diagnostic{Stage: stage}
	var code syscall.Errno
	if errors.As(err, &code) {
		d.NativeCode = uint32(code)
	}
	return ErrorWithDiagnostic(d)
}

func DiagnosticFromError(err error) *Diagnostic {
	var reported *diagnosticError
	if errors.As(err, &reported) && reported != nil && ValidDiagnostic(&reported.value) {
		copy := reported.value
		return &copy
	}
	return nil
}

func failureDiagnostic(stage string, err error) *Diagnostic {
	if d := DiagnosticFromError(err); d != nil {
		return d
	}
	return DiagnosticFromError(refusal(stage, err))
}

// NoInstallWasAttempted proves only that this authenticated session's install
// was refused before any WFP backend invocation. It does not prove that every
// filter on the machine is absent. Unknown replies cannot satisfy this proof.
func NoInstallWasAttempted(r Result, id string) bool {
	return sessionPattern.MatchString(id) && r.SessionID == id && !r.Installed && r.Verified && r.State == "install_not_attempted" && ValidDiagnostic(r.Diagnostic) && preInstallStages[r.Diagnostic.Stage]
}

func refusedBeforeInstall(id, stage string, err error) Result {
	return Result{SessionID: id, Verified: true, State: "install_not_attempted", Diagnostic: failureDiagnostic(stage, err)}
}

func UnavailableCapability(err error) Capability {
	return Capability{Diagnostic: failureDiagnostic("manager_unavailable", err)}
}

func ValidCapability(c Capability) bool {
	return c.Available && c.Diagnostic == nil || !c.Available && ValidDiagnostic(c.Diagnostic) && (preInstallStages[c.Diagnostic.Stage] || c.Diagnostic.Stage == "guard_transport")
}
