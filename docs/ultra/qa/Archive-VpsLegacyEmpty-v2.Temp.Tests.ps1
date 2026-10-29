# Windows PowerShell 5.1 x64, elevated. Retained inert TEMP fixtures ONLY.
# Imports allowlisted AST functions/callbacks, never either archive script main.
# No ProgramData, registry, service/process inspection, product, network or UAC.
# Creates two independent fresh test trees, modifies only their fixture ACLs,
# runs one expected-failing v1 rename and one successful v2 rename, and retains
# every fixture. No retry, rollback or deletion is performed.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
$env:PSModulePath=[IO.Path]::Combine($PSHOME,'Modules')
function Assert-TempTest([bool]$Okay,[string]$Code) { if (-not $Okay) { throw $Code } }
if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'fixture_host_rejected' }
$identity=[Security.Principal.WindowsIdentity]::GetCurrent()
try {
    $principal=New-Object Security.Principal.WindowsPrincipal($identity)
    Assert-TempTest ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) 'fixture_requires_elevated_token'
} finally { $identity.Dispose() }
function Read-TempAst([string]$Name) {
    $tokens=$null;$errors=$null
    $parsed=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $Name),[ref]$tokens,[ref]$errors)
    Assert-TempTest ($errors.Count -eq 0) 'fixture_source_parser'
    return $parsed
}
$v1=Read-TempAst 'Archive-VpsLegacyEmpty.ps1'
$v2=Read-TempAst 'Archive-VpsLegacyEmpty-v2.ps1'
$safe=@('Get-ArchivePolicies','Get-ArchiveHash','Assert-ArchiveEnvelope','Assert-ArchiveSD','Assert-ArchiveEntries','Assert-ArchiveSnapshot','Assert-ArchiveSame','Invoke-ArchivePlan','New-ArchiveNative','Get-ArchiveComponents','Close-ArchiveHandles','Open-ArchiveNode','Get-ArchiveSddl','Open-ArchiveParents','Test-ArchiveAbsent','Read-ArchiveBytes','Read-ArchiveSnapshot')
$functions=@($v2.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
foreach ($name in $safe) {
    $matches=@($functions | Where-Object {$_.Name -ceq $name})
    Assert-TempTest ($matches.Count -eq 1) 'fixture_function_missing'
    Invoke-Expression $matches[0].Extent.Text
}
Assert-TempTest ((Get-ArchiveHash ([IO.File]::ReadAllBytes((Join-Path $PSScriptRoot 'Archive-VpsLegacyEmpty.ps1')))) -ceq 'f79e38f3765af9b42838b9cc4834380a87fa463c1bd659932628caf0cac845c8') 'fixture_frozen_v1'
$oldPlans=@($v1.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Invoke-ArchivePlan'},$true))
Assert-TempTest ($oldPlans.Count -eq 1) 'fixture_v1_plan'
Invoke-Expression ($oldPlans[0].Extent.Text.Replace('function Invoke-ArchivePlan(','function Invoke-ArchivePlanV1('))
function Get-TempCallbacks($Ast) {
    $commands=@($Ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -ceq 'Invoke-ArchivePlan'},$true))
    Assert-TempTest ($commands.Count -eq 1) 'fixture_main_callback_count'
    return ,$commands[0].CommandElements
}
$v1Callbacks=Get-TempCallbacks $v1;$v2Callbacks=Get-TempCallbacks $v2
Assert-TempTest ($v1Callbacks.Count -eq 7 -and $v2Callbacks.Count -eq 8) 'fixture_callback_shape'
$script:archiveNative=New-ArchiveNative
$tempBase=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$fixtureLeaf='ultra-archive-v2-temp-'+[guid]::NewGuid().ToString('N')
$fixtureRoot=[IO.Path]::GetFullPath([IO.Path]::Combine($tempBase,$fixtureLeaf))
Assert-TempTest ($fixtureRoot.StartsWith($tempBase+'\',[StringComparison]::OrdinalIgnoreCase) -and [IO.Path]::GetFileName($fixtureRoot) -cmatch '^ultra-archive-v2-temp-[a-f0-9]{32}$') 'fixture_root_boundary'
function Resolve-TempChild([string]$Relative) {
    $resolved=[IO.Path]::GetFullPath([IO.Path]::Combine($fixtureRoot,$Relative))
    Assert-TempTest ($resolved.StartsWith($fixtureRoot+'\',[StringComparison]::OrdinalIgnoreCase)) 'fixture_child_boundary'
    return $resolved
}
function Assert-TempPath([string]$Path) {
    $resolved=[IO.Path]::GetFullPath($Path)
    Assert-TempTest ($resolved.Equals($fixtureRoot,[StringComparison]::OrdinalIgnoreCase) -or $resolved.StartsWith($fixtureRoot+'\',[StringComparison]::OrdinalIgnoreCase)) 'fixture_io_boundary'
}
function Set-TempDescriptor([string]$Path,[bool]$Directory,[string]$Sddl) {
    Assert-TempPath $Path
    if ($Directory) { $acl=New-Object Security.AccessControl.DirectorySecurity;$acl.SetSecurityDescriptorSddlForm($Sddl);[IO.Directory]::SetAccessControl($Path,$acl) }
    else { $acl=New-Object Security.AccessControl.FileSecurity;$acl.SetSecurityDescriptorSddlForm($Sddl);[IO.File]::SetAccessControl($Path,$acl) }
}
$role='O:BAG:BAD:P(A;OICI;FA;;;BA)(A;OICI;FA;;;SY)'
$legacy=$role+'(A;OICI;FA;;;S-1-5-21-1-2-3-1001)'
$payload='{"version":1,"userKey":"machine-policy-store-v1","controls":{}}'
$envelope=[Text.Encoding]::UTF8.GetBytes('{"payload":'+$payload+',"sha256":"'+(Get-ArchiveHash ([Text.Encoding]::UTF8.GetBytes($payload)))+'"}')
$setupPins=New-Object 'Collections.Generic.List[IntPtr]'
$held=New-Object 'Collections.Generic.List[IntPtr]'
try {
    # Validate from the drive root before creating the fresh fixture root.
    Open-ArchiveParents $tempBase '' $setupPins
    Assert-TempTest (Test-ArchiveAbsent $fixtureRoot) 'fixture_already_exists'
    [void][IO.Directory]::CreateDirectory($fixtureRoot);Set-TempDescriptor $fixtureRoot $true $role
    foreach ($case in @('held-parent-case','released-parent-case')) {
        foreach ($relative in @($case,($case+'\source'),($case+'\source\privacy-machine'),($case+'\qa'),($case+'\qa\stage'))) {
            $resolved=Resolve-TempChild $relative;[void][IO.Directory]::CreateDirectory($resolved);Set-TempDescriptor $resolved $true $role
        }
        $caseSource=Resolve-TempChild ($case+'\source')
        $caseMachine=Resolve-TempChild ($case+'\source\privacy-machine')
        $journal=Resolve-TempChild ($case+'\source\privacy-machine\privacy-journal.json')
        $lock=Resolve-TempChild ($case+'\source\privacy-machine\privacy.lock')
        [IO.File]::WriteAllBytes($journal,$envelope);[IO.File]::WriteAllBytes($lock,[byte[]]@())
        Set-TempDescriptor $caseSource $true $legacy;Set-TempDescriptor $caseMachine $true $legacy
        Set-TempDescriptor $journal $false $legacy;Set-TempDescriptor $lock $false $legacy
    }
    # No setup handle may remain to contaminate the independent v2 rename.
    Close-ArchiveHandles $setupPins;$setupPins.Clear()
    $summaries=New-Object 'Collections.Generic.List[object]'
    foreach ($case in @('held-parent-case','released-parent-case')) {
        $programData=Resolve-TempChild $case
        $source=Resolve-TempChild ($case+'\source')
        $qa=Resolve-TempChild ($case+'\qa');$stage=Resolve-TempChild ($case+'\qa\stage')
        $script:archiveTarget=Resolve-TempChild ($case+'\qa\stage\legacy-empty-before-beta')
        $state=@{moves=0;hresult=0;policyReads=0}
        try {
            Open-ArchiveParents $programData '' $held;Open-ArchiveParents $stage $qa $held
            $before=Read-ArchiveSnapshot $source
            Assert-TempTest ($before.empty -and $before.fileCount -eq 2 -and $before.lockBytes -eq 0 -and $before.legacyCount -eq 1) 'fixture_not_exact_legacy_empty'
            $preflight={ Assert-TempTest (Test-ArchiveAbsent $script:archiveTarget) 'fixture_destination_exists' }
            # Every one of the real four policy names is checked in a fictional
            # empty map. The real HKLM policy reader is never imported or run.
            $fictionalValues=@{}
            $policies={ foreach ($policy in @(Get-ArchivePolicies)) { $state.policyReads++;if ($fictionalValues.ContainsKey($policy.path+'\'+$policy.name)) { return $false } };return $true }
            if ($case -ceq 'held-parent-case') {
                $rawMove=$v1Callbacks[3].ScriptBlock.GetScriptBlock()
                $move={ $state.moves++;try { &$rawMove } catch { $state.hresult=$_.Exception.GetBaseException().HResult;throw } }
                $r=Invoke-ArchivePlanV1 $preflight $v1Callbacks[2].ScriptBlock.GetScriptBlock() $move $v1Callbacks[4].ScriptBlock.GetScriptBlock() $v1Callbacks[5].ScriptBlock.GetScriptBlock() $policies
                Assert-TempTest ($r.status -ceq 'review_required' -and $r.moveAttempted -and $state.moves -eq 1 -and $state.hresult -eq -2147024864) 'fixture_v1_expected_sharing_violation'
                Assert-ArchiveSame $before (Read-ArchiveSnapshot $source)
                Assert-TempTest (Test-ArchiveAbsent $script:archiveTarget) 'fixture_v1_destination_absent'
                Assert-TempTest ($state.policyReads -eq 8) 'fixture_v1_two_four_policy_checks'
                [void]$summaries.Add([pscustomobject]@{case='v1_held_parents';status=$r.status;moveAttempts=$state.moves;win32=($state.hresult -band 0xffff);sourceRetained=$true;targetAbsent=$true;hashesAndAclUnchanged=$true})
            } else {
                $rawMove=$v2Callbacks[4].ScriptBlock.GetScriptBlock()
                $move={ $state.moves++;Assert-TempTest ($held.Count -eq 0) 'fixture_parent_pins_not_released';&$rawMove }
                $r=Invoke-ArchivePlan $preflight $v2Callbacks[2].ScriptBlock.GetScriptBlock() $v2Callbacks[3].ScriptBlock.GetScriptBlock() $move $v2Callbacks[5].ScriptBlock.GetScriptBlock() $v2Callbacks[6].ScriptBlock.GetScriptBlock() $policies
                Assert-TempTest ($r.status -ceq 'completed' -and $r.parentPinsReleased -and $r.moveAttempted -and $state.moves -eq 1 -and $held.Count -gt 0 -and $r.sourceAbsent -and $r.policiesAbsent) 'fixture_v2_expected_success'
                Assert-ArchiveSame $before (Read-ArchiveSnapshot $script:archiveTarget)
                Assert-TempTest ($state.policyReads -eq 12) 'fixture_v2_three_four_policy_checks'
                [void]$summaries.Add([pscustomobject]@{case='v2_release_then_repin';status=$r.status;moveAttempts=$state.moves;parentPinsReleased=$true;parentsRepinned=$true;sourceAbsent=$true;hashesAndOwnerGroupDaclUnchanged=$true;emptyControls=$true;fourPoliciesFictionalAbsent=$true})
            }
        } finally { Close-ArchiveHandles $held;$held.Clear() }
    }
    [pscustomobject]@{schema='ultra-archive-v2-temp-tests';scope='retained_inert_temp_only';fixtureLeaf=$fixtureLeaf;fixtureRetained=$true;cases=$summaries.ToArray();realRegistryRead=$false;productExecuted=$false;vpsAccess=$false;objectsDeleted=$false;productionMigration=$false;stableNamespaceAssumed=$true} | ConvertTo-Json -Compress -Depth 6
} finally {
    Close-ArchiveHandles $setupPins;Close-ArchiveHandles $held
    # Intentionally retain the new fixture, even if an assertion fails.
}
