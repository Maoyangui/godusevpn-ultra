# AST/fictional metadata/fake move callbacks only. Never load filesystem,
# service/process/registry checks, Directory.Move or the script main body.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Archive-VpsLegacyEmpty.ps1'),[ref]$tokens,[ref]$parseErrors)
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
foreach ($kind in @('success','target_exists','initial_policy','nonempty','extra_file','changed_hash','changed_acl','move_unknown','post_hash','source_present','post_policy')) {
    $state=@{kind=$kind;moves=0;reads=0;policies=0;checks=0}
    $preflight={ $state.checks++;if ($state.kind -ceq 'target_exists') { throw 'destination_exists' } }.GetNewClosure()
    $readSource={
        $state.reads++;$s=New-FakeSnapshot
        if ($state.kind -ceq 'nonempty') { $s.empty=$false }
        if ($state.kind -ceq 'extra_file') { $s.fileCount=3 }
        if ($state.kind -ceq 'changed_hash' -and $state.reads -eq 2) { $s.journalHash='c'*64 }
        if ($state.kind -ceq 'changed_acl' -and $state.reads -eq 2) { $s.lockSddl='changed' }
        return $s
    }.GetNewClosure()
    $move={ $state.moves++;if ($state.kind -ceq 'move_unknown') { throw 'synthetic_unknown' } }.GetNewClosure()
    $readTarget={ $s=New-FakeSnapshot;if ($state.kind -ceq 'post_hash') { $s.lockHash='d'*64 };return $s }.GetNewClosure()
    $sourceAbsent={ return $state.kind -cne 'source_present' }.GetNewClosure()
    $policiesAbsent={ $state.policies++;return $state.kind -cne 'initial_policy' -and -not ($state.kind -ceq 'post_policy' -and $state.policies -eq 3) }.GetNewClosure()
    $result=Invoke-ArchivePlan $preflight $readSource $move $readTarget $sourceAbsent $policiesAbsent
    if ($kind -ceq 'success') { Assert-Test ($result.status -ceq 'completed' -and $state.moves -eq 1 -and $result.hashesMatch -and $result.aclMatch -and $result.sourceAbsent -and $result.policiesAbsent) 'complete_archive' }
    elseif ($kind -in @('move_unknown','post_hash','source_present','post_policy')) { Assert-Test ($result.status -ceq 'review_required' -and $state.moves -eq 1) 'no_retry_or_rollback' }
    else { Assert-Test ($result.status -ceq 'rejected' -and $state.moves -eq 0) 'refused_before_move' }
    $json=$result | ConvertTo-Json -Compress -Depth 5
    Assert-Test ($json.Length -lt 1500 -and $json -notmatch 'fictional|S-1-5|journalHash|rootSddl') 'bounded_nonraw_output'
    $cases++
}
[Console]::Out.WriteLine('{"scope":"AST_and_fake_only","parser":true,"nativeMetadata":true,"fixedPolicies":4,"archiveCases":'+$cases+',"emptyEnvelope":true,"legacySourceOnly":true,"actualMove":false,"productExecuted":false,"registryWrites":false}')
