# Read-only browser diagnostic v2

V1 and its failed VM evidence remain untouched. Its class-20 zero-buffer
GetTokenInformation call returned ERROR_BAD_LENGTH (24); the tool incorrectly
required ERROR_INSUFFICIENT_BUFFER (122) for all classes. This was a diagnostic
buffer assumption, not evidence that the product's token access failed.

V2 retains the exact fixed installed QA11 image hash, parent/helper/session/SID/
integrity/alive/creation checks, unmodified privilege probes and provider-limited
four-layer WFP count from v1. No production token permission, privilege, policy,
ACL, WFP rule, process, service or browser is changed. DLL imports are limited
to System32. The only new write is the fixed sanitized QA report described below.

TokenElevation now gets a real DWORD buffer (4 bytes). TokenStatistics gets its
x64 structure buffer (56 bytes), with AuthenticationId at offset 8. User,
Integrity and Privileges start with a real bounded 4096-byte buffer, avoiding
NULL/zero-buffer status assumptions entirely. An error-24/122 resize is allowed
once only for a recognized variable class, returned length above the allocated
size, and an at-most-1MiB bound. An actual failing query is never treated as
successful data. Successful returns must fit the allocation and minimum type
size; fixed types must return exactly their correct size. Class, actual code,
required length, allocation and fixed-size choice are in the sanitized report.

`PureChecks.cs` calls only numeric buffer-planning helpers: 42 checks cover
fixed sizes, 24/122, zero/short/oversized/unknown classes and return lengths.
It does not invoke the diagnostic entry point, Info, writer or any imported
token/process/filesystem/WFP function. The diagnostic itself was only compiled,
never run on the development machine. The production lifecycle change separately
rejects cancel/deadline errors even when joined with a matching typed receipt;
this tool does not modify that production code.

New fixed VM stage (never the v1 directory):
`C:\GodUseVPN-Ultra-Qa11-BrowserReadOnlyDiagV2`.
Before the first launch it must contain exactly these independently verified
two leaves: `BrowserReadOnlyDiagnosticV2.exe` and
`Invoke-BrowserReadOnlyDiagnosticV2.ps1`. SYSTEM stages them with protected
system/admin-owned ACLs and ordinary-user read/execute only. Do not elevate an
ordinary-user-writable candidate. No config, DLL, old report or other file is
accepted by the launcher. No stage directory is created or modified by it.

From ordinary PowerShell run the fixed staged launcher with ParentPid and
HelperPid; select the same `ultraadmin` account as the running helper in the
explicit UAC dialog. The tool still accepts only those two decimal PIDs plus
the exact fixed QA11 hash injected by the launcher. It never reads CommandLine.
Matching account/elevation/session is checked; a separate RunAs logon can have
a different AuthenticationId, which is reported rather than silently equated.

V2 emits full JSON to stdout and attempts CREATE_NEW of the fixed leaf
`diagnostic-v2-report.json` in that new stage. It checks its own fixed base
directory, local fixed-drive namespace, held no-reparse/no-delete-sharing
directory identities, protected system/admin ownership and absence of ordinary
write permissions before writing. The JSON contains only the same booleans,
bounded counts, fixed gate IDs and numeric API codes, with no SID, PID, paths,
nonce, credential, command line or endpoint. Admins need write access to this
QA stage if the full receipt is wanted; the tool never grants it. The file is
flushed, never overwritten or recovered automatically. A partial write or
existing report is preserved; stdout marks receiptWritten=false and records
the write exception HRESULT plus any fixed gate number. A report with unknown
write outcome is not declared current. The launcher requires a fresh stage, so
it refuses an automatic second run after a report exists. Preserve evidence
and request a separately reviewed new-stage plan for another iteration.

The console remains visible until Enter. Individual native failures and WFP
unknowns remain separate from completed identity/probe execution. WFP counts
are only the four known layers of the fixed browser provider, not a live
traffic test, filter ownership/authenticity proof, atomic global snapshot or
proof of absence on other layers. No non-browser provider is enumerated.

Primary references: [GetTokenInformation ReturnLength](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-gettokeninformation),
[TOKEN_ELEVATION](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-token_elevation),
[TOKEN_STATISTICS](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-token_statistics).
V1's README retains the other fixed process/token/WFP API references.
