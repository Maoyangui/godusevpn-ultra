# Real medium-token CLI status: preparation only

`Vps-Qa2MediumStatus.ps1` is a QA draft with the frozen qa.2 CLI/service hashes. It has **not been run**. The new C01 implementation is newer than qa.2; the root reviewer must make a separately reviewed qa.3 copy with its actual fixed hashes before any acceptance run. The existing qa.2 token-query probe remains unchanged. No product source is changed by this work.

The only product operation permitted by this launcher is the installed `godusevpn-cli.exe status`. In the audited source, `status` exits zero only after the authenticated `MGetState` call succeeds. The launcher reads only that exit code and fixed booleans; it does not collect the status text, node names, subscriptions, account/SID, session number, process ID, environment or machine paths. It does not demonstrate GUI startup, UAC usability or tunnel operation.

## Preconditions and fixed scope

- Separate approval to execute this reviewed helper and the unsigned installed CLI. Use `-TestAccount '<test-account>'`, a conservative local username, never a password or domain account.
- Windows PowerShell 5.1 x64, an actual SYSTEM primary caller with no impersonation token; the installed code is under the fixed Program Files product directory. Trusted local administrators control the OS ancestors and installation tree.
- The fixed service is running as canonical `LocalSystem`, Own Process, noninteractive, with the exact quoted installed service command. A held service process has the expected image and actual SYSTEM token. Installed CLI/service bytes must match the fixed candidate hashes; held no-follow read handles pin the code paths and reject ordinary-user write ACLs.
- Exactly one existing System32 `cmd.exe` belongs to the approved real local account. `Win32_Process` is queried only for `ProcessId,ExecutablePath,Handle`; `GetOwnerSid` is checked against the held process token independently. The source may be medium already or have a genuine limited linked token. A restricted SYSTEM token is never a substitute.
- The selected token and its primary duplicate must have the real account SID, exact Medium integrity, no enabled administrators group, no elevation, and the original source session. No token DACL, privileges, user, policy, service, controller registration or profile is changed by the helper.

## Child lifetime

The launcher creates a user environment with `CreateEnvironmentBlock(..., false)` and calls `CreateProcessAsUserW` with one fixed application, a fixed quoted `status` command line, the installed working directory, `CREATE_SUSPENDED | CREATE_NO_WINDOW | CREATE_UNICODE_ENVIRONMENT`, and `bInheritHandles=false`. Zeroed x64 STARTUPINFO has no standard handles or `STARTF_USESTDHANDLES`; no output pipe or console is opened. No `LoadUserProfile`, `SetTokenInformation`, `AdjustTokenPrivileges`, impersonation or secondary shell is used. The API may temporarily enable privileges already held by SYSTEM as documented; the helper does not add privileges.

Before resuming the newly created thread, it checks the held child's actual primary token, user, Medium integrity, elevation, administrators-group state, session and executable image. It rechecks the held source/service and fresh SCM state. Any rejection terminates only the still-owned child handle; there is no PID-based kill or retry. A successful `ResumeThread` must report exactly one previous suspension.

After resume, the wait is bounded to 15 seconds. A timeout, unexpected wait, resume failure or unreadable exit code remains unknown. The helper attempts one termination of its own child, waits at most one further second to confirm exit, and always closes the owned process/thread handles. It never kills a preexisting process. A known nonzero CLI exit is a failed status check; it does not trigger install, repair, service restart, guard changes, disconnect or other automatic action. Fresh SCM/held-service checks must still agree after a zero exit before `cliAuthenticationPassed=true` is reported.

`productExecuted`/`statusCommandExecuted` record a successful expected resume. A failed or raced resume is marked `resultUnknown`; a false execution flag in that case is not proof that no instruction could have executed. This controlled QA assumes the approved account and local administrators are not actively injecting into or resuming the test child. It does not claim process isolation from an adversarial same-account actor.

## Verification performed

Windows PowerShell 5.1 ran `Vps-Qa2MediumStatus.Tests.ps1`: **71 checks passed**. Tests parse the whole file but load only named pure functions and the Reflection.Emit declaration factory. They inspect native signatures without invoking them, verify zeroed x64 STARTUPINFO/PROCESS_INFORMATION buffers in memory, and exercise the production lifecycle with fake handles/operations: successful exit, creation failure, identity rejection before resume, failed resume, 15-second timeout, failed termination, nonzero/unknown exit and cleanup failure. They verify no retries, no termination after confirmed exit, and both handles closed even when the first close fails.

No main body, CIM, native method, actual token, child process, product command, service operation, registry operation or network call was executed. Real `CreateProcessAsUserW`, the source user's token rights/environment, cross-session behavior, the ordinary-user pipe/SCM/process-query permissions and successful new-candidate `MGetState` remain unverified until a separately authorized qa.3 run. No C01 acceptance item is closed by these pure tests.

Official API boundaries: [CreateProcessAsUserW](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessasuserw), [CreateEnvironmentBlock](https://learn.microsoft.com/en-us/windows/win32/api/userenv/nf-userenv-createenvironmentblock), [DuplicateTokenEx](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-duplicatetokenex).
