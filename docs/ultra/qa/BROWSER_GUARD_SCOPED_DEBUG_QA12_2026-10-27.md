# Authenticated browser-parent access: scoped retry

This is a QA12 source change. The frozen QA11 package is unchanged. No local
test invoked a process/token API, enabled a privilege, started a browser or
changed WFP. The new native path still requires an isolated VM run.

## Reproduced boundary

The authorized read-only VM diagnostic verified the installed GUI/helper image,
ordinary GUI identity, elevated helper identity and process lifetime. Opening
the GUI with QUERY_LIMITED_INFORMATION and SYNCHRONIZE succeeded. Adding
PROCESS_DUP_HANDLE failed with Win32 5. Both token QUERY and QUERY|IMPERSONATE,
and the GUI-token LocalAppData lookup, succeeded. SeDebug was present but not
enabled in both diagnostic and helper tokens. The fixed browser provider had
zero filters in the four enumerated ALE layers. These are separate observations;
the initial factory refusal was not a WFP installation or removal failure.

## Source contract

`New` still accepts only the authenticated parent process HANDLE already held
by the fixed privileged helper. No external PID, path, token, access mask or
privilege selector is accepted. A normal exact-mask open remains first. Only
its ERROR_ACCESS_DENIED result permits one retry with precisely the same
QUERY_LIMITED_INFORMATION|DUP_HANDLE|SYNCHRONIZE mask and non-inheritance.

Before and after acquiring the new handle, the held parent and candidate must
agree on live PID, creation time, canonical image, SID, session, token identity,
authentication identity, integrity, elevation and token type. The GUI must be
ordinary Medium, non-elevated, non-system, and its image must equal the current
authenticated helper image. The original held handle is never closed here.

The retry runs in a private goroutine locked to one OS thread. An initial
thread token or uncertain thread-token query is refused. Only the helper's own
primary token is opened for QUERY|DUPLICATE. A distinct impersonation copy is
created for QUERY|ADJUST_PRIVILEGES|IMPERSONATE. Its identity, authentication,
session, High integrity, elevation, impersonation level and full privilege set
must match the expected copy contract. Only existing SeDebug is enabled on
that copy. The raw AdjustTokenPrivileges call checks BOOL and its same-call
last-error value, including ERROR_NOT_ALL_ASSIGNED, then independently reads
back that no other privilege changed. The helper's primary token is never
adjusted and is independently checked again during cleanup.

After attachment, the actual thread token is checked against the verified copy.
The only privileged target operation is the fixed same-mask OpenProcess retry.
Cleanup is registered before attachment is attempted. Every return, partial
attachment and recovered panic runs cleanup. RevertToSelf success plus an
explicit ERROR_NO_TOKEN readback are both required before UnlockOSThread or
delivery of a parent handle. The process candidate is withheld on every failure.

An uncertain or failed revert leaves the goroutine locked through exit. The
caller then waits at most five seconds on a separately held synchronization
HANDLE for that exact thread. Only WAIT_OBJECT_0 confirms retirement. Timeout,
WAIT_FAILED or another result refuses the capability and retains the thread
HANDLE as local unknown-resource evidence. The code does not terminate the
VPN, helper process or unrelated threads, and does not return an uncertain
thread to the Go scheduler. Even confirmed retirement does not turn a failed
operation into success. No process-global token, target DACL or access mask is
changed.

Fixed diagnostics are `owner_debug_thread`, `owner_debug_token`,
`owner_debug_duplicate`, `owner_debug_identity`, `owner_debug_enable`,
`owner_debug_attach`, `owner_debug_open`, `owner_debug_revert`, and
`owner_debug_retire`. They contain only a stage and numeric native code.

## Validation and remaining acceptance

Pure fake tests cover normal ordering, first-open error classification and
single retry, changed parent/candidate identity, absent/not-assigned SeDebug,
copy/active-token mismatches, partial attachment, original-token changes,
revert and token-query failures, finite retirement timeout/failed waits,
notification after body/cleanup panics, handle cleanup, and 48 concurrent
independent attempts. Raw BOOL/error interpretation and the 56-byte native
TokenStatistics layout are checked without native calls. The guard, helper
and native-host test suites and vet pass.

VM acceptance must repeat the same alternate-account authorization and browser
startup through the production helper, establish that the parent access now
succeeds, and confirm helper SeDebug remains disabled afterward. Further
Job/WFP/protocol/runtime refusals remain independent stages; this source fix
does not establish browser readiness, proxy routing or cleanup completion.

## Primary contracts

- [OpenProcess](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-openprocess): requested access and SeDebug behavior.
- [DuplicateTokenEx](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-duplicatetokenex): distinct impersonation-token creation.
- [AdjustTokenPrivileges](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-adjusttokenprivileges): success BOOL is insufficient and cannot add an absent privilege.
- [SetThreadToken](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-setthreadtoken): current-thread attachment and required token access.
- [RevertToSelf](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-reverttoself): failed revert leaves impersonation active; its general recommendation is process shutdown. This implementation confines that exceptional state to a locked thread and refuses all capability delivery, with independently observed thread retirement when available.
- [Go LockOSThread/UnlockOSThread](https://pkg.go.dev/runtime#LockOSThread): no other goroutine uses a locked thread, and exiting without unlocking terminates it; unsuitable thread state must not be unlocked.
