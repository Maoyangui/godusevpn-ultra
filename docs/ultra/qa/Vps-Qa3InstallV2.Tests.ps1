# AST / metadata / memory / fake control flow only. No official installer,
# wrapper main, native filesystem/token/SCM/registry, process or product runs.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Vps-Qa3InstallV2.ps1'),[ref]$tokens,[ref]$errors)
if ($errors.Count) { throw 'parser_failed' }
$functions=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw ('assertion_failed_'+$script:checks) };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }
Check (@($functions | Group-Object Name | Where-Object {$_.Count -ne 1}).Count -eq 0)
Check (-not [regex]::IsMatch($ast.Extent.Text,'[^\x00-\x7f]'))
$allow=@('Get-AssembleHash','Assert-AssembleID','Get-AssembleComponents','Assert-AssemblePrivateSD','Assert-AssembleName','Read-AssembleManifest','New-InstallNative','Get-InstallPolicies','Assert-InstallEmptyJournal','Assert-InstallWhatIfOutput','Invoke-InstallPreviewPlan')
foreach ($name in $allow) { $f=@($functions | Where-Object {$_.Name -ceq $name});Check ($f.Count -eq 1);. ([scriptblock]::Create($f[0].Extent.Text)) }
foreach ($statement in $ast.EndBlock.Statements) {
    if ($statement -is [Management.Automation.Language.AssignmentStatementAst] -and $statement.Left -is [Management.Automation.Language.VariableExpressionAst] -and $statement.Left.VariablePath.UserPath -cin @('script:assembleManifestHash','script:assembleHashes','script:installLengths')) { . ([scriptblock]::Create($statement.Extent.Text)) }
}
$base=Join-Path $PSScriptRoot 'Assemble-VpsQa3Package.ps1'
Check ((Get-AssembleHash ([IO.File]::ReadAllBytes($base))) -ceq '00393b3d79d7d64ecda93ff06d16ea6233a25aeb2ef8b0b879d08315c8153834')
$baseTokens=$null;$baseErrors=$null;$baseAst=[Management.Automation.Language.Parser]::ParseFile($base,[ref]$baseTokens,[ref]$baseErrors)
$copied=@('Get-AssembleHash','Assert-AssembleID','Get-AssembleComponents','Assert-AssemblePrivateSD','Assert-AssemblePrivatePath','Get-AssembleAttributes','Lock-AssembleDirectories','Read-AssembleFile','Assert-AssembleName','Read-AssembleManifest')
foreach ($name in $copied) {
    $before=@($baseAst.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
    $after=@($functions | Where-Object {$_.Name -ceq $name})
    Check ($before.Count -eq 1 -and $after.Count -eq 1 -and $before[0].Extent.Text.Replace("`r`n","`n") -ceq $after[0].Extent.Text.Replace("`r`n","`n"))
}
Check (@($ast.ParamBlock.Parameters).Count -eq 1 -and $ast.ParamBlock.Parameters[0].Name.VariablePath.UserPath -ceq 'CandidateStageID')
$native=New-InstallNative
foreach ($name in @('CreateFileW','GetFileInformationByHandleEx','OpenSCManagerW','OpenServiceW')) {
    $method=$native.GetMethod($name);$attribute=@($method.GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
    Check ($attribute.SetLastError -and $attribute.ExactSpelling -and $attribute.CharSet -eq [Runtime.InteropServices.CharSet]::Unicode)
}
foreach ($forbidden in @('CreateDirectoryW','CreateServiceW','StartServiceW','TerminateProcess','SetTokenInformation')) { Check ($null -eq $native.GetMethod($forbidden)) }
$policies=@(Get-InstallPolicies);Check ($policies.Count -eq 4)
Check (($policies | ForEach-Object {$_[1]}) -join '|' -ceq 'DisabledByGroupPolicy|AllowInputPersonalization|AllowClipboardHistory|AllowCrossDeviceClipboard')
$payload='{"version":1,"userKey":"machine-policy-store-v1","controls":{}}'
function Envelope([string]$Value) { return ,[Text.Encoding]::UTF8.GetBytes('{"payload":'+$Value+',"sha256":"'+(Get-AssembleHash ([Text.Encoding]::UTF8.GetBytes($Value)))+'"}') }
Assert-InstallEmptyJournal (Envelope $payload);$checks++
foreach ($bad in @($payload.Replace('machine-policy-store-v1','fictional-user'),$payload.Replace('"controls":{}','"controls":{"managed":{}}'),$payload.Replace('"controls":{}','"controls":{},"controls":{}'))) { Reject { Assert-InstallEmptyJournal (Envelope $bad) } }
Reject { Assert-InstallEmptyJournal ([byte[]]@()) }
Reject { Assert-InstallEmptyJournal ([Text.Encoding]::UTF8.GetBytes('{"payload":'+$payload+',"sha256":"'+('0'*64)+'"}')) }
Assert-AssemblePrivateSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)';$checks++
Reject { Assert-AssemblePrivateSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FR;;;BU)' }
$target='C:\Fixture\GodUseVPN Ultra'
$preview='Verified unsigned Ultra 0.01-beta-qa.3, 351 payload files.'+"`r`n"+'Target: '+$target+"`r`n"+'What if: Create protected directory, copy verified payload, install/start godusevpn-ultra service'
Assert-InstallWhatIfOutput $preview $target;$checks++
Assert-InstallWhatIfOutput ($preview.Replace('copy verified payload,',("copy verified`r`npayload,"))) $target;$checks++
foreach ($bad in @($preview.Replace('351','350'),$preview.Replace('qa.3','qa.2'),$preview.Replace('Create protected directory','Other action'),($preview+"`r`nInstalled Ultra at C:\Fixture"))) { Reject { Assert-InstallWhatIfOutput $bad $target } }
Reject { Assert-InstallWhatIfOutput $preview 'C:\Other' }
foreach ($scenario in @('success','service_exists','SCM_unknown','installation_exists','policy_present','machine_not_empty','data_override_present','timeout','bad_receipt','post_unknown','post_changed')) {
    $state=@{scenario=$scenario;pre=0;calls=0;events=(New-Object 'Collections.Generic.List[string]')}
    $preflight={
        $state.pre++;[void]$state.events.Add('preflight')
        if ($state.scenario -in @('service_exists','SCM_unknown','installation_exists','policy_present','machine_not_empty','data_override_present')) { throw $state.scenario }
        if ($state.scenario -ceq 'post_unknown' -and $state.pre -eq 2) { throw 'fictional_unknown_do_not_export' }
        if ($state.scenario -ceq 'post_changed' -and $state.pre -eq 2) { return 'changed' }
        return 'empty-journal-fingerprint'
    }.GetNewClosure()
    $whatif={ $state.calls++;[void]$state.events.Add('whatif');if ($state.scenario -ceq 'timeout') { throw 'whatif_timeout' };if ($state.scenario -ceq 'bad_receipt') { throw 'whatif_receipt_rejected' } }.GetNewClosure()
    $r=Invoke-InstallPreviewPlan $preflight $whatif
    if ($scenario -ceq 'success') { Check ($r.status -ceq 'completed' -and $r.unchanged -and ($state.events -join ',') -ceq 'preflight,whatif,preflight') }
    elseif ($scenario -in @('timeout','bad_receipt','post_unknown','post_changed')) { Check ($r.status -ceq 'review_required' -and $state.calls -eq 1) }
    else { Check ($r.status -ceq 'rejected' -and $state.calls -eq 0) }
    $json=$r | ConvertTo-Json -Compress;Check ($json.Length -lt 1000 -and $json -notmatch 'fingerprint|fictional|C:|S-1-5')
}
$raw=$ast.Extent.Text
$scm=$native.GetMethod('OpenSCManagerW')
Check ($scm.GetParameters()[0].ParameterType -eq [IntPtr] -and $scm.GetParameters()[1].ParameterType -eq [IntPtr])
Check ($raw.Contains('OpenSCManagerW([IntPtr]::Zero,[IntPtr]::Zero,1)') -and $raw -notmatch 'OpenSCManagerW\(\$null,\$null')
Check ($raw.Contains('ultra-qa3-install-preview-v2'))
Check ($raw.Contains(' -WhatIf') -and $raw.Contains('60000') -and $raw.Contains('65536') -and $raw.Contains('-ne 1060'))
Check ($raw.Contains('actualInstallSupported=$false') -and $raw.Contains('effectiveSystemCaller=') -and $raw.Contains("'package-beta-qa3'"))
Check (@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -match 'Start-Process|Set-Acl|Remove-Item|New-Item|Invoke-Expression'},$true)).Count -eq 0)
[Console]::Out.WriteLine(('{"scope":"AST_metadata_memory_fake_only","checks":'+$checks+',"previewCases":11,"mainExecuted":false,"nativeCalled":false,"officialWhatIfExecuted":false,"productExecuted":false,"actualInstallSupported":false}'))

