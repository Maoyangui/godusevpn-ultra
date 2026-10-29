# qa.3 installation, real medium identity and uninstall acceptance

2026-10-19. **Preparation only: no installation, token access, product execution or remote operation was performed while writing this file. This document is not execution approval.** Test candidate `0.01-beta-qa.3`, Windows amd64, unsigned, PE numeric File/ProductVersion `0.1.0.0`. The source was dirty at build time; this is a fixed QA candidate, not a clean public release.

## Fixed artifacts

The complete candidate has **352 regular files: 351 manifest payload files plus SHA256SUMS**. The installer's message therefore reports 351 payload files. Do not add the new QA launcher to that tree: it is delivered and reviewed separately.

| Artifact | SHA256 |
|---|---|
| Full `godusevpn-ultra-0.01-beta-qa.3-windows-amd64.zip` | `50f9b657569da793ef7893cee554c2fc45ba807d8ed9ed040a9941f0419dad27` |
| `SHA256SUMS` | `bd0bcadae8965f3f782e63146d917f1fac82df8d3d7fd15eee3cd0a2cd5f61dd` |
| `godusevpn.exe` | `2293b29ed415e7ef972208e54f690437f61a6e195f35c9da0f4bdf9a7c7caa15` |
| `godusevpn-svc.exe` | `2ebd104afd943353f18662510e751acf9421e03fa7cca834d516444b98f5127e` |
| `godusevpn-cli.exe` | `3a17693ee9e80c865b9bee92ee34f403adeb75883a797b821e6123e9edabfae8` |
| Packaged `Install-Ultra.ps1` | `dcb217c4c3968c2f9c65fdd3c48e275c25642b23bc694f6ab169774bc5c0148c` |
| Packaged `Uninstall-Ultra.ps1` | `af6e7b98021beeb5f91850b52db7c4c11657b94c49278f002658cac934a51347` |
| Separate `Vps-Qa3MediumStatus.ps1` | `b651ef9607c2dd6d3124beba88ff7965d336d4adf5d2e340ee20144ce8abd298` |

Local package descriptor/audit and actual file hashes were read without running the binaries. The fixed full archive is an alternative to the separately reviewed delta/support reconstruction: either route must result in the exact same 352-file package. A local hash list does not authenticate its publisher by itself; its expected values must come from the reviewed record.

## Preconditions and paths

All checks must succeed before installation. An access error is unknown, never evidence of absence.

- Windows PowerShell 5.1 x64, true SYSTEM in session 0, no thread impersonation. The Azure Run Command recovery channel is available. Existing snapshot creation does not prove that restoration was rehearsed.
- Both fixed SCM names `godusevpn` and `godusevpn-ultra` are absent. Both original Program Files `godusevpn` and Ultra `GodUseVPN Ultra` targets are absent. An existing path/service stops this batch; no takeover, merge, repair or automatic deletion.
- The legacy archive batch is complete and independently recorded. ProgramData `godusevpn-ultra` contains only the approved new role-private `privacy-machine` directory, canonical empty journal and lock; managed policies are 0 and all four previously exercised policy values remain absent. No old settings/state/profiles/controller registration, network restore records or original/Ultra active filters exist. Do not execute the policy roundtrip script to check this baseline.
- No `GODUSEVPN_ULTRA_DATA` override is present for the installer/service. No original or Ultra GUI/helper is running. No user settings, timezone, browser protections, application firewall rules, subscription or connection will be added by this batch.
- The assembled `package-beta-qa3` has the fixed manifest hash, exact inventory, no extra/link/reparse members, fixed descriptor and three fixed EXE hashes/PE resources. The source and its root-first local private ACL/pins are independently verified. Do not replace packaged files with newer source files.
- The separate QA launcher has its reviewed hash and private local parent. Exactly one existing System32 `cmd.exe` belongs to the approved local `<test-account>`. There must be a real medium or limited linked token; do not create an account, collect a password, relax UAC or use restricted SYSTEM as a substitute.

These variables are placeholders for a private QA session; never publish real account, StageID, SID, process ID, address or resource identifiers:

```powershell
$ErrorActionPreference = 'Stop'
$candidateStageId = '<candidate-stage-id>'  # approved 32hex UUID4
$qaStageId = '<qa-script-stage-id>'         # separately approved 32hex UUID4
$testAccount = '<test-account>'             # conservative local username
$programData = [Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData)
$programFiles = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles)
$qaRoot = Join-Path $programData 'GodUseVPN-Ultra-QA'
$package = Join-Path (Join-Path $qaRoot $candidateStageId) 'package-beta-qa3'
$mediumLauncher = Join-Path (Join-Path $qaRoot $qaStageId) 'Vps-Qa3MediumStatus.ps1'
$installed = Join-Path $programFiles 'GodUseVPN Ultra'
$cli = Join-Path $installed 'godusevpn-cli.exe'
$svc = Join-Path $installed 'godusevpn-svc.exe'
$controllerName = [Environment]::MachineName + '\' + $testAccount
```

Assignment and `Test-Path` alone do not validate these paths. Apply the reviewed no-follow/private ACL/pin checks before reading or invoking any staged script. Query SCM by exact name and a minimal property subset; retain the distinction between not found and query failure:

```powershell
$original = @(Get-CimInstance Win32_Service -Filter "Name='godusevpn'" -Property Name)
$ultra = @(Get-CimInstance Win32_Service -Filter "Name='godusevpn-ultra'" -Property Name)
if ($original.Count -ne 0 -or $ultra.Count -ne 0) { throw 'service_baseline_not_empty' }
if (Test-Path -LiteralPath $installed) { throw 'target_exists' }
if (Test-Path -LiteralPath (Join-Path $programFiles 'godusevpn')) { throw 'original_target_exists' }
if ($env:GODUSEVPN_ULTRA_DATA) { throw 'data_override_present' }
```

## Ordered execution batches

The commands below specify operations; they are **not** one unconditional paste-and-run batch. Use the reviewed bounded private runner for each approved operation. Raw installer output contains paths and `register-controller` prints SIDs; do not stream or save it in public QA records. Publish only fixed booleans, bounded exit codes/counts and version/hash matches. The old five-command readonly runner does not authorize new install/register/status/uninstall commands.

| Step | Exact operation | Effects, required evidence and stop rule |
|---|---|---|
| 1. Readonly preflight | `& (Join-Path $package 'Install-Ultra.ps1') -WhatIf` | Reads package/manifest/path/ACL/SCM, no product EXE. Require 351 payload/352 inventory, fresh fixed target and version `0.01-beta-qa.3`; a normal script return is not proof that installation occurred. |
| 2. First installation | `& (Join-Path $package 'Install-Ultra.ps1')` as session-0 SYSTEM | **Requires explicit first installation/service-start approval.** Creates the fixed Program Files tree, copies/protects all 352 files, runs the unsigned fixed service EXE with `install`, writes controller registration, creates automatic-start SCM service and starts SYSTEM daemon. Normal daemon startup writes logs/rule-set/data directories and may reconcile existing network restore state; the clean baseline is therefore mandatory. Success requires independently rechecking all 352 bytes/manifest, SCM Running/Own Process/canonical LocalSystem/exact quoted installed `service` command, held process actual image/SYSTEM identity and fixed hashes. A partial failure retains files/state; stop without rerun/repair or deletion. |
| 3. Unregistered real medium negative control | `& $mediumLauncher -TestAccount $testAccount` | **Requires first helper/real-user-token and unsigned CLI-status approval.** Fresh session-0 SYSTEM install should register SYSTEM only, but verify that the test account is actually absent from the controller list before calling this a negative control. Launcher must validate real medium child and execute only installed CLI `status` once. Expected known nonzero status exit: `status='rejected'`, `error='status_failed'`, `statusExitZero=false`, `resultUnknown=false`, `childExited=true`, real-user/medium/hash/child validation true. A timeout, unknown or failure before child execution is untested, not an authorization-negative pass. Unexpected success stops the batch for review. |
| 4. Register one controller | `& $svc 'register-controller' $controllerName '--restart'` | **Requires explicit controller privilege expansion and service restart approval.** Persists the approved local account SID, stops/starts Ultra service. This grants the complete main IPC control surface, including connect/disconnect/settings; it is not a readonly grant. Source exit codes 2/3 can mean registration persisted despite restart failure. Any nonzero/unknown stops; do not retry or treat it as rolled back. Independently recheck SCM, actual fixed service image/identity and current instance. |
| 5. Real medium positive control | `& $mediumLauncher -TestAccount $testAccount` | The same separately hashed helper runs only the fixed installed CLI `status` under the same real local medium identity. Require `status='completed'`, `realUserMatched/medium/installedHashesMatched/childValidated/statusCommandExecuted/statusExitZero/postServiceUnchanged/cliAuthenticationPassed=true`, `resultUnknown=false`, no termination attempt. This proves this candidate's ordinary-token authenticated `MGetState` path; it does not prove GUI/UAC or a tunnel. Failure remains C01 unverified, with service/protection unchanged. Never alter process/token/pipe DACLs, elevate all UI or swap SYSTEM for the user to obtain a pass. |
| 6. Official uninstall preflight | `& (Join-Path $installed 'Uninstall-Ultra.ps1') -WhatIf` | Readonly installed inventory/code/service/no-GUI checks. Do not use `-ValidationPackage` for actual uninstall and do not invoke an uninstalled source copy. |
| 7. Official uninstall | `& (Join-Path $installed 'Uninstall-Ultra.ps1')` as SYSTEM | **Requires explicit cleanup/service and possible network-restoration approval.** Runs fixed CLI `privacy uninstall-machine` under one machine-maintenance lease: verify fixed package/service, restore managed machine policies, remove/verify Ultra application rules, then matching service uninstall. Service uninstall stops the service, clears Ultra guard, attempts IPv6/DNS/route restoration and deletes the SCM object. Even with an expected empty baseline, this is a real cleanup operation. Existing per-user/browser/timezone protections are not restored by SYSTEM. Failure retains tools/backups and stops; no broad restore-all, manual clear, forced stop or directory deletion. |
| 8. Independent terminal checks | Exact-name SCM queries; `& $cli 'guard' 'status'`; `& $cli 'privacy' 'firewall-list'`; reviewed readonly machine-journal snapshot and four-value absence checks | After approved bounded private execution, require original/Ultra SCM absent, Ultra active guard filters 0, Ultra application rules 0 and unknown filters 0, canonical managed journal 0 and all four policy values still absent. Guard/firewall reads use installed CLI and do not require the removed service. Do not run cleanup to manufacture a good result. Files/manifest/controller record/logs/empty journal and lock remain. Any read failure is unknown; preserve recovery tools and investigate. |

If a SYSTEM CLI countercheck is needed, request that exact additional operation and use a bounded in-memory/discarding `status` runner; do not stream the raw status. It is not required to substitute for step 5 and cannot validate ordinary-user C01.

The negative control deliberately does not collect native error text. A known nonzero exit alone cannot distinguish a controller-list denial from another authenticated-status failure. Record it as the expected unregistered end-to-end failure, not proof of one particular access-control error; only the later positive control can establish success under the approved medium identity.

## Launcher evidence and limitations

`Vps-Qa3MediumStatus.ps1` is a constrained copy of frozen qa.2 launcher SHA `4136a5b9cb087e8b8ce296abd31ddeba7fa0dfe1c2878da38c9f7f14cf06e2b0`. Only candidate hashes/version, comments and dynamic assembly QA3 naming changed. `Vps-Qa3MediumStatus.Tests.ps1` passed **72 PS5.1 pure checks**, including exact normalized equality to that frozen logic, native signatures, zeroed x64 structures and fake lifecycle. No main/native/token/CIM/child/product was run.

The helper queries only `ProcessId,ExecutablePath,Handle` for candidate cmd processes, then independently verifies the held actual process token. It duplicates a genuine user's primary medium token, preserves session, uses `CreateEnvironmentBlock` without loading a profile and `CreateProcessAsUserW` with suspended/no-window/Unicode environment and no inherited handles. Actual child SID/Medium/elevation/BA/session/image and fresh SCM are checked before resume. Only the created process handle can be terminated; no PID-based kill or retries. Wait is at most 15 seconds plus a one-second confirmation of one own-child termination attempt. Unreadable final status remains unknown. No stdout/stderr, account/SID, process ID, environment or status content is collected.

A successful expected resume sets the execution booleans; failed/raced resume remains unknown, so a false execution flag in an unknown result is not proof of zero executed instructions. The controlled test assumes the approved account and local administrators are not actively manipulating/injecting into the child. Actual user token rights, profile/session suitability, SCM/pipe/query-limited permissions, native process creation and end-to-end C01 remain unverified until the authorized run. No native GUI/UAC test has occurred; native CUA is unavailable.

Stop on every mismatch, timeout, query error or unknown. Retain the private records and tools. Cleanup may proceed only within separately approved official uninstall scope, never an automatic reaction to authentication uncertainty. No subscription, tunnel/guard-disconnect experiment, reboot, new account, extra paid cloud resource or unrelated security setting is part of this batch. Final zero active Ultra filters does not assert that all system WFP providers/sublayers are absent; inspect fixed Ultra objects separately if that additional evidence is required. Retained files/data mean uninstall is not byte-identical machine rollback.
