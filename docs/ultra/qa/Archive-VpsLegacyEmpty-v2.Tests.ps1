# AST/fictional metadata/fake move callbacks only. Never load filesystem,
# service/process/registry checks, Directory.Move or the script main body.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Archive-VpsLegacyEmpty-v2.ps1'),[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'archive_parser_failed' }
$allowed=@('Get-ArchivePolicies','Get-ArchiveHash','Assert-ArchiveEnvelope','Assert-ArchiveSD','Assert-ArchiveEntries','Assert-ArchiveSnapshot','Assert-ArchiveSame','Invoke-ArchivePlan','New-ArchiveNative','Get-ArchiveComponents')
$functions=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
foreach ($name in $allowed) {
    $found=@($functions | Where-Object {$_.Name -ceq $name})
    if ($found.Count -ne 1) { throw 'pure_function_missing' }
    Invoke-Expression $found[0].Extent.Text
}
function Assert-Test([bool]$Condition,[string]$Code) { if (-not $Condition) { throw $Code } }
function Assert-Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Assert-Test $failed 'expected_rejection' }
$v1Path=Join-Path $PSScriptRoot 'Archive-VpsLegacyEmpty.ps1'
Assert-Test ((Get-ArchiveHash ([IO.File]::ReadAllBytes($v1Path))) -ceq 'f79e38f3765af9b42838b9cc4834380a87fa463c1bd659932628caf0cac845c8') 'frozen_v1_unchanged'
$oldTokens=$null;$oldErrors=$null
$oldAst=[Management.Automation.Language.Parser]::ParseFile($v1Path,[ref]$oldTokens,[ref]$oldErrors)
Assert-Test ($oldErrors.Count -eq 0) 'v1_parser'
$oldFunctions=@($oldAst.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
foreach ($old in $oldFunctions) {
    if ($old.Name -ceq 'Invoke-ArchivePlan') { continue }
    $current=@($functions | Where-Object {$_.Name -ceq $old.Name})
    Assert-Test ($current.Count -eq 1 -and $current[0].Extent.Text -ceq $old.Extent.Text) 'unchanged_native_acl_empty_policy_checks'
}
function Make-ArchiveEnvelope([string]$Payload) { return ,[Text.Encoding]::UTF8.GetBytes('{"payload":'+$Payload+',"sha256":"'+(Get-ArchiveHash ([Text.Encoding]::UTF8.GetBytes($Payload)))+'"}') }
$payload='{"version":1,"userKey":"machine-policy-store-v1","controls":{}}'
Assert-ArchiveEnvelope (Make-ArchiveEnvelope $payload)
Assert-Reject { Assert-ArchiveEnvelope (Make-ArchiveEnvelope $payload.Replace('machine-policy-store-v1','legacy-account')) }
Assert-Reject { Assert-ArchiveEnvelope (Make-ArchiveEnvelope $payload.Replace('"controls":{}','"controls":{"other":{}}')) }
Assert-Reject { Assert-ArchiveEnvelope (Make-ArchiveEnvelope $payload.Replace('"controls":{}','"controls":{},"controls":{}')) }
Assert-Reject { Assert-ArchiveEnvelope ([Text.Encoding]::UTF8.GetBytes('{"payload":'+$payload+',"sha256":"'+('0'*64)+'"}')) }
Assert-Reject { Assert-ArchiveEnvelope (New-Object byte[] 4194305) }
Assert-Test ((Get-ArchiveHash ([byte[]]@())) -ceq 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855') 'empty_lock_hash'
Assert-ArchiveEntries @('privacy-machine') @('privacy.lock','privacy-journal.json')
Assert-Reject { Assert-ArchiveEntries @('privacy-machine','logs') @('privacy.lock','privacy-journal.json') }
Assert-Reject { Assert-ArchiveEntries @('privacy-machine') @('privacy.lock','privacy-journal.json','other') }
Assert-Reject { Assert-ArchiveEntries @('Privacy-machine') @('privacy.lock','privacy-journal.json') }
$role='O:BAG:BAD:P(A;OICI;FA;;;BA)(A;OICI;FA;;;SY)'
$legacy='O:BAG:BAD:P(A;OICI;FA;;;BA)(A;OICI;FA;;;SY)(A;OICI;FA;;;S-1-5-21-1-2-3-1001)'
$null=Assert-ArchiveSD $role $false
Assert-Test ((Assert-ArchiveSD $legacy $true).legacySids.Count -eq 1) 'one_fictional_legacy_account'
Assert-Reject { Assert-ArchiveSD $legacy $false }
Assert-Reject { Assert-ArchiveSD 'O:BUG:BUD:P(A;;FA;;;SY)' $true }
Assert-Reject { Assert-ArchiveSD 'O:BAG:BAD:P(A;;FR;;;BU)' $true }
Assert-Reject { Assert-ArchiveSD ($legacy + '(A;;FA;;;S-1-5-21-4-5-6-1002)') $true }
Assert-Test (@(Get-ArchivePolicies).Count -eq 4) 'four_hklm_targets'
foreach ($bad in @('C:\','C:relative','\\invalid.example\share','C:\folder\..\file','C:\folder\CONOUT$.txt')) { Assert-Reject { Get-ArchiveComponents $bad } }
foreach ($point in @(0x00b9,0x00b2,0x00b3)) { $bad='C:\folder\COM'+[char]$point;Assert-Reject { Get-ArchiveComponents $bad } }
$native=New-ArchiveNative
$import=@($native.GetMethod('CreateFileW').GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
Assert-Test ($import.ExactSpelling -and $import.SetLastError -and $native.GetMethod('CreateFileW').ReturnType -eq [IntPtr]) 'native_metadata_only'

function New-FakeSnapshot {
    return [pscustomobject]@{empty=$true;fileCount=2;lockBytes=0;legacyCount=1;journalHash=('a'*64);lockHash=('b'*64);rootSddl='fictional-root';machineSddl='fictional-machine';journalSddl='fictional-journal';lockSddl='fictional-lock'}
}
$cases=0
foreach ($kind in @('success','target_exists','initial_policy','nonempty','extra_file','changed_hash','changed_acl','move_unknown','post_hash','source_present','post_policy','release_unknown','post_repin','post_acl','fresh_policy','fresh_preflight')) {
    $state=@{kind=$kind;moves=0;reads=0;policies=0;checks=0;releases=0;events=(New-Object 'Collections.Generic.List[string]')}
    $preflight={ $state.checks++;[void]$state.events.Add('preflight');if ($state.kind -ceq 'target_exists' -or ($state.kind -ceq 'fresh_preflight' -and $state.checks -eq 2)) { throw 'destination_exists' } }.GetNewClosure()
    $readSource={
        $state.reads++;[void]$state.events.Add('source');$s=New-FakeSnapshot
        if ($state.kind -ceq 'nonempty') { $s.empty=$false }
        if ($state.kind -ceq 'extra_file') { $s.fileCount=3 }
        if ($state.kind -ceq 'changed_hash' -and $state.reads -eq 2) { $s.journalHash='c'*64 }
        if ($state.kind -ceq 'changed_acl' -and $state.reads -eq 2) { $s.lockSddl='changed' }
        return $s
    }.GetNewClosure()
    $release={ $state.releases++;[void]$state.events.Add('release');if ($state.kind -ceq 'release_unknown') { throw 'synthetic_release' } }.GetNewClosure()
    $move={ $state.moves++;[void]$state.events.Add('move');if ($state.kind -ceq 'move_unknown') { throw 'synthetic_unknown' } }.GetNewClosure()
    $readTarget={ [void]$state.events.Add('target');if ($state.kind -ceq 'post_repin') { throw 'synthetic_repin' };$s=New-FakeSnapshot;if ($state.kind -ceq 'post_acl') { $s.rootSddl='changed-group' };if ($state.kind -ceq 'post_hash') { $s.lockHash='d'*64 };return $s }.GetNewClosure()
    $sourceAbsent={ return $state.kind -cne 'source_present' }.GetNewClosure()
    $policiesAbsent={ $state.policies++;[void]$state.events.Add('policies');return $state.kind -cne 'initial_policy' -and -not ($state.kind -ceq 'post_policy' -and $state.policies -eq 3) -and -not ($state.kind -ceq 'fresh_policy' -and $state.policies -eq 2) }.GetNewClosure()
    $result=Invoke-ArchivePlan $preflight $readSource $release $move $readTarget $sourceAbsent $policiesAbsent
    if ($kind -ceq 'success') { Assert-Test ($result.status -ceq 'completed' -and $state.moves -eq 1 -and $result.hashesMatch -and $result.aclMatch -and $result.sourceAbsent -and $result.policiesAbsent) 'complete_archive' }
    elseif ($kind -in @('move_unknown','post_hash','source_present','post_policy','post_repin','post_acl')) { Assert-Test ($result.status -ceq 'review_required' -and $state.moves -eq 1) 'no_retry_or_rollback' }
    else { Assert-Test ($result.status -ceq 'rejected' -and $state.moves -eq 0) 'refused_before_move' }
    if ($kind -ceq 'success') { Assert-Test ($state.releases -eq 1 -and $result.parentPinsReleased -and (($state.events -join ',') -ceq 'preflight,source,policies,preflight,source,policies,release,move,target,policies')) 'release_order_and_once' }
    if ($kind -ceq 'release_unknown') { Assert-Test ($state.releases -eq 1 -and -not $result.parentPinsReleased -and -not $result.moveAttempted) 'release_failure_no_move' }
    if ($state.moves -eq 0 -and $kind -cne 'release_unknown') { Assert-Test ($state.releases -eq 0 -and -not $result.parentPinsReleased) 'preflight_failure_keeps_pins' }
    if ($state.moves -eq 1) { Assert-Test ($state.releases -eq 1 -and $result.parentPinsReleased) 'move_requires_release' }
    $json=$result | ConvertTo-Json -Compress -Depth 5
    Assert-Test ($json.Length -lt 1500 -and $json -notmatch 'fictional|S-1-5|journalHash|rootSddl') 'bounded_nonraw_output'
    $cases++
}
[Console]::Out.WriteLine('{"scope":"AST_and_fake_only","parser":true,"nativeMetadata":true,"fixedPolicies":4,"archiveCases":'+$cases+',"emptyEnvelope":true,"legacySourceOnly":true,"actualMove":false,"productExecuted":false,"registryWrites":false}')
