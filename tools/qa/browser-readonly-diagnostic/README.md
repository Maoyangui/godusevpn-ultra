# VM-only read-only browser failure diagnostic

This standalone QA tool is not included in the product and does not launch it.
Build compiles C# only; do not run the diagnostic on the development machine.
No privilege is enabled, no impersonation is performed, and no ACL, token,
process, service, firewall filter or configuration is changed. Its only output
is a JSON report on stdout and an interactive Enter-to-close prompt. It creates
no report files and reads no command lines, environment, subscription, proxy,
endpoint, profile, UA, GPU or browser data. SID strings are used only internally.

`Build.ps1` writes only the ignored `.codex/tmp/browser-readonly-diagnostic`
candidate. Its EXE manifest is explicitly asInvoker. Compilation is not runtime
acceptance. Pin the resulting EXE SHA256 before any separately authorized VM
transfer; do not load a neighboring `.config`, DLL, script or modified build.

The EXE accepts exactly three positional arguments: ordinary GUI PID, elevated
helper PID, and the fixed QA11 GUI SHA256
`d93205d060ecd86d651d72a4ac63aedbb9572d83bc875c3edafa201bc5afae05`.
No executable path, provider, privilege or arbitrary process target is accepted.
Leading-zero/nondecimal PIDs, extra args and other hashes refuse.

Before process probes, the tool obtains ProgramFiles with DONT_VERIFY and
accepts a canonical local fixed-drive namespace only. It pins every directory
from the volume root and the exact `GodUseVPN Ultra\godusevpn.exe` leaf without
write/delete sharing, rejects reparses and multiply-linked EXEs, verifies final
path and hashes the same retained file handle. Both process images must match
that fixed installed path. Held process handles must remain alive with unchanged
creation times before and after. Helper parent PID must match the GUI and its
creation time must be later. Sessions must match the diagnostic's nonzero session.

The diagnostic must be launched using the same administrator account as the
helper, with High integrity and elevation. GUI must be Medium, not elevated and
not a service account. Owner SIDs are compared without printing them. The report
also records `sameHelperAuthenticationId`: a separate RunAs can have a different
logon token even for the same account. The tool and helper's SeDebug present and
enabled flags are read independently; nothing assumes UAC automatically enabled
the privilege, and nothing attempts to enable it.

After fixed image/parent/session checks it independently probes GUI OpenProcess
QUERY_LIMITED_INFORMATION|SYNCHRONIZE versus that mask plus DUP_HANDLE, then
TOKEN_QUERY versus QUERY|IMPERSONATE on each successful handle. Failed calls
preserve their actual unsigned Win32 codes. A denied GUI token query prevents
full GUI identity verification; the failure report remains useful evidence,
but the tool does not bypass the check or claim completion. LocalAppData is
queried only after GUI identity is verified, with DONT_VERIFY and each successful
token, never opening that returned directory. Raw HRESULT and, only for Win32
facility errors, the low-word Win32 code are distinct fields.

WFP uses a non-null fixed browser provider template and the guard's four fixed
ALE connect/receive layers. Only an aggregate count is emitted. Disabled and
boot-time entries are included; no filter conditions, application IDs, names,
endpoints or another provider's entries are inspected. Paging is bounded to
32768 filters per layer. `wfpCountComplete` is false on errors or a page bound;
no zero count is fabricated on failure. This is a read snapshot of the four
known layers, not proof of absence at other layers, rule ownership, enabled
enforcement, or atomic state across concurrent changes. No WFP transaction,
add/delete/update API or service operation is imported.

## VM-only launch design

The root operator must first stage the independently hash-verified tool into a
new approved directory and confirm there is no sibling config/DLL. From the
ordinary user's PowerShell, explicitly RunAs that fixed staged executable with
the current verified GUI/helper PIDs and the fixed QA11 hash. For example, after
approval to use this new VM-only location (this repository does not create it):

```powershell
$exe = 'C:\GodUseVPN-Ultra-Qa11-BrowserReadOnlyDiag\BrowserReadOnlyDiagnostic.exe'
# Compare Get-FileHash $exe with the separately reviewed compiler receipt first.
Start-Process -FilePath $exe -Verb RunAs -Wait -ArgumentList @('7716','2200','d93205d060ecd86d651d72a4ac63aedbb9572d83bc875c3edafa201bc5afae05')
```

The checked `Invoke-BrowserReadOnlyDiagnostic.ps1` additionally requires exactly
the tool and launcher in that fixed stage, a protected system/admin-owned DACL
with no ordinary-user write/delete/ACL rights, and the frozen tool bytes. It
does not create or change the directory. Its only parameters are ParentPid and
HelperPid; after independent SYSTEM staging, invoke that fixed launcher as the
ordinary user. The example above describes the equivalent explicit RunAs step;
use the checked launcher for the actual test.

Select the same `ultraadmin` UAC account as the existing helper. Do not use Azure
RunCommand/SYSTEM as a substitute; the owner/session checks reject that context.
The PIDs in the example are transient and must still pass the fixed runtime
checks. The report stays visible until Enter. Exit 0 means the allowed probes
finished with stable verified identities; individual access or WFP failure codes
can still be nonzero. Exit 1 means identity/bounds/argument checks did not finish.
`refusedStage` is a fixed internal gate number, not a native error code.

## Primary API references

- [OpenProcessToken access and cross-account remarks](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-openprocesstoken)
- [SHGetKnownFolderPath token rights, HRESULT and resource cleanup](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-shgetknownfolderpath)
- [FWPM_FILTER_ENUM_TEMPLATE0 provider and action filters](https://learn.microsoft.com/en-us/windows/win32/api/fwpmtypes/ns-fwpmtypes-fwpm_filter_enum_template0)
- [FwpmFilterCreateEnumHandle0 snapshot/access limits](https://learn.microsoft.com/en-us/windows/win32/api/fwpmu/nf-fwpmu-fwpmfiltercreateenumhandle0)
- [Microsoft SDK structure definitions](https://github.com/microsoft/win32metadata/blob/main/generation/WinSDK/RecompiledIdlHeaders/shared/fwpmtypes.h)
