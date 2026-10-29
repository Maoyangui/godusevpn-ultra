# AST and synthetic state-machine validation only. Never load the main body,
# process runner, filesystem/registry readers, a product, UAC or networking.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Vps-BetaPolicyRoundtrip.ps1'),[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'roundtrip_parser_failed' }
$allowed=@('Get-RoundPolicies','Get-RoundPayload','Get-RoundHash','Read-RoundEnvelope','Assert-RoundAction','Assert-RoundEmpty','Invoke-RoundBatch','New-RoundNative','Get-RoundComponents','Assert-RoundSD')
$functions=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
foreach ($name in $allowed) {
    $found=@($functions | Where-Object {$_.Name -ceq $name})
    if ($found.Count -ne 1) { throw 'pure_function_missing' }
    Invoke-Expression $found[0].Extent.Text
}
function Assert-Test([bool]$Condition,[string]$Code) { if (-not $Condition) { throw $Code } }
function Assert-Reject([scriptblock]$Body) { $rejected=$false;try { &$Body | Out-Null } catch { $rejected=$true };Assert-Test $rejected 'expected_rejection' }
function Make-Envelope([string]$Payload) {
    return ,[Text.Encoding]::UTF8.GetBytes('{"payload":'+$Payload+',"sha256":"'+(Get-RoundHash ([Text.Encoding]::UTF8.GetBytes($Payload)))+'"}')
}
$policies=@(Get-RoundPolicies)
Assert-Test ($policies.Count -eq 4) 'catalog_count'
Assert-Test (($policies.id -join '|') -ceq 'windows-advertising-id|windows-online-speech|windows-clipboard-history|windows-clipboard-cloud') 'catalog_ids'
Assert-Test (($policies.desired -join '|') -ceq '1|0|0|0') 'catalog_values'
$empty=Get-RoundPayload $null
Assert-Test ($empty -ceq '{"version":1,"userKey":"machine-policy-store-v1","controls":{}}') 'empty_writer_shape'
Assert-Test ((Read-RoundEnvelope (Make-Envelope $empty) $null) -ceq 'empty') 'empty_healthy'
foreach ($p in $policies) {
    $payload=Get-RoundPayload $p
    Assert-Test ((Read-RoundEnvelope (Make-Envelope $payload) $p) -ceq 'active') 'active_backup'
    Assert-Reject { Read-RoundEnvelope (Make-Envelope $payload) $null }
    Assert-Reject { Read-RoundEnvelope (Make-Envelope $payload.Replace('"active"','"applying"')) $p }
    Assert-Reject { Read-RoundEnvelope (Make-Envelope $payload.Replace('"exists":false','"exists":true')) $p }
}
$golden='{"version":1,"userKey":"machine-policy-store-v1","controls":{"windows-advertising-id":{"stage":"active","changes":[{"location":{"hive":"HKLM","path":"SOFTWARE\\Policies\\Microsoft\\Windows\\AdvertisingInfo","name":"DisabledByGroupPolicy"},"before":{"exists":false,"type":0},"desired":{"exists":true,"type":4,"data":"AQAAAA=="}}]}}}'
Assert-Test ((Get-RoundPayload $policies[0]) -ceq $golden) 'go_writer_golden'
Assert-Reject { Read-RoundEnvelope ([Text.Encoding]::UTF8.GetBytes('{"payload":'+$empty+',"sha256":"'+('0'*64)+'"}')) $null }
Assert-Reject { Read-RoundEnvelope (Make-Envelope $empty.Replace('machine-policy-store-v1','legacy-user')) $null }
Assert-Reject { Read-RoundEnvelope (Make-Envelope $empty.Replace('"controls":{}','"controls":{},"controls":{}')) $null }
Assert-Reject { Read-RoundEnvelope (New-Object byte[] 4194305) $null }

# Emitting metadata performs no native method invocation.
$native=New-RoundNative
$import=@($native.GetMethod('CreateFileW').GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
Assert-Test ($import.ExactSpelling -and $import.SetLastError -and $native.GetMethod('CreateFileW').ReturnType -eq [IntPtr]) 'native_metadata'
Assert-Test (@(Get-RoundComponents 'C:\ProgramData\GodUseVPN-Ultra-QA\fixture').Count -eq 4) 'path_components'
foreach ($bad in @('C:\','C:relative','\\invalid.example\share','\\?\C:\file','C:\folder\..\file','C:\folder\NUL.txt','C:\folder\name:stream')) { Assert-Reject { Get-RoundComponents $bad } }
foreach ($point in @(0x00b9,0x00b2,0x00b3)) { $alias='C:\folder\COM'+[char]$point+'.txt';Assert-Reject { Get-RoundComponents $alias } }
Assert-RoundSD 'O:BAG:BAD:P(A;OICI;FA;;;BA)(A;OICI;FA;;;SY)' $true
Assert-Reject { Assert-RoundSD 'O:BUG:BUD:P(A;;FA;;;BA)' $true }
Assert-Reject { Assert-RoundSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FR;;;BU)' $true }
Assert-Reject { Assert-RoundSD 'O:BAG:BAD:(A;;FA;;;BA)(A;;FA;;;SY)' $true }
Assert-Reject { Assert-RoundAction ([pscustomobject]@{id='other';status='configured';changed=$true}) $policies[0] 'apply' }
Assert-Reject { Assert-RoundAction ([pscustomobject]@{id=$policies[0].id;status='configured';changed='true'}) $policies[0] 'apply' }

$caseCount=0
foreach ($kind in @('success','initial_value','initial_journal','apply_timeout','apply_nochange','bad_backup','missing_lock','read_failure','external_conflict','restore_unknown')) {
    $state=@{kind=$kind;active='';values=@{};calls=(New-Object 'Collections.Generic.List[string]');readFailed=$false}
    foreach ($p in $policies) { $state.values[$p.id]='absent' }
    if ($kind -ceq 'initial_value') { $state.values[$policies[0].id]='external' }
    $readPolicy={param($p)
        if ($state.active -ceq $p.id -and $state.kind -ceq 'read_failure' -and -not $state.readFailed) { $state.readFailed=$true;throw 'synthetic_read_failure' }
        if ($state.active -ceq $p.id -and $state.kind -ceq 'external_conflict') { $state.values[$p.id]='external' }
        return [pscustomobject]@{exists=$state.values[$p.id] -cne 'absent';matches=$state.values[$p.id] -ceq 'desired'}
    }.GetNewClosure()
    $readJournal={param($p)
        if ($state.kind -ceq 'initial_journal') { return [pscustomobject]@{state='dirty';lockPresent=$false} }
        if ($state.active -and $state.kind -ceq 'bad_backup') { throw 'synthetic_wrong_backup' }
        if ($state.active) { return [pscustomobject]@{state='active';lockPresent=($state.kind -cne 'missing_lock')} }
        return [pscustomobject]@{state='empty';lockPresent=$false}
    }.GetNewClosure()
    $run={param($action,$p)
        [void]$state.calls.Add($action+':'+$p.id)
        if ($action -ceq 'apply') {
            if ($state.kind -ceq 'apply_nochange') { return [pscustomobject]@{id=$p.id;status='configured';changed=$false} }
            $state.active=$p.id;$state.values[$p.id]='desired'
            if ($state.kind -ceq 'apply_timeout') { $script:roundUnknown=$true;throw 'synthetic_timeout' }
            return [pscustomobject]@{id=$p.id;status='configured';changed=$true}
        }
        if ($state.kind -ceq 'restore_unknown') { $script:roundUnknown=$true;throw 'synthetic_restore_unknown' }
        $state.active='';$state.values[$p.id]='absent'
        return [pscustomobject]@{id=$p.id;status='restored';changed=$true}
    }.GetNewClosure()
    $script:roundUnknown=$false
    $summary=Invoke-RoundBatch $readPolicy $readJournal $run
    if ($kind -ceq 'success') {
        Assert-Test ($summary.status -ceq 'completed' -and $summary.completed -eq 4 -and $summary.initialAbsentCount -eq 4 -and $state.calls.Count -eq 8) 'four_roundtrips'
        Assert-Test (($summary | ConvertTo-Json -Compress -Depth 8).Length -lt 2600) 'summary_bound'
    } elseif ($kind -in @('initial_value','initial_journal')) { Assert-Test ($state.calls.Count -eq 0 -and $summary.status -ceq 'rejected') 'initial_refusal' }
    elseif ($kind -ceq 'read_failure') { Assert-Test ($state.calls.Count -eq 2 -and $summary.results[0].status -ceq 'recovered_after_failure' -and $state.active -ceq '') 'finally_owned_restore' }
    elseif ($kind -ceq 'restore_unknown') { Assert-Test ($state.calls.Count -eq 2 -and $summary.results[0].status -ceq 'recovery_pending') 'restore_not_retried' }
    else {
        Assert-Test ($state.calls.Count -eq 1 -and $summary.completed -eq 0) 'unknown_or_unowned_no_restore'
        if ($kind -ceq 'external_conflict') { Assert-Test ($state.values[$policies[0].id] -ceq 'external') 'external_value_preserved' }
    }
    foreach ($call in $state.calls) { Assert-Test ($call -notmatch 'restore-all|install|connect|start') 'only_fixed_actions' }
    $caseCount++
}
[Console]::Out.WriteLine('{"scope":"AST_and_synthetic_only","parser":true,"nativeMetadata":true,"fixedPolicies":4,"stateMachineCases":'+$caseCount+',"journalChecksumAndShape":true,"productExecuted":false,"registryWrites":false}')
