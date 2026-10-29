# AST, metadata, immutable package manifest, and fake control flow only.
# Never invokes wrapper main, native APIs, official installer, or product binaries.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$path=Join-Path $PSScriptRoot 'Vps-Qa4InstallV2.ps1'
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$errors)
if ($errors.Count) { throw 'parser_failed' }
$functions=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw ('assertion_failed_'+$script:checks) };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }

Check (@($functions|Group-Object Name|Where-Object {$_.Count -ne 1}).Count -eq 0)
Check (-not [regex]::IsMatch($ast.Extent.Text,'[^\x00-\x7f]'))
$allow=@('Get-AssembleHash','Assert-AssembleID','Get-AssembleComponents','Assert-AssemblePrivateSD','Assert-AssemblePrivatePath','Get-AssembleAttributes','Read-AssembleManifest','Assert-AssembleName','New-InstallNative','Test-InstallAbsent','Assert-InstallNoServices','Get-InstallPolicies','Assert-InstallPoliciesAbsent','Assert-InstallEmptyJournal','Assert-InstallWhatIfOutput','Invoke-InstallPreviewPlan')
foreach ($name in $allow) { $f=@($functions|Where-Object {$_.Name -ceq $name});Check ($f.Count -eq 1);. ([scriptblock]::Create($f[0].Extent.Text)) }
foreach ($statement in $ast.EndBlock.Statements) {
    if ($statement -is [Management.Automation.Language.AssignmentStatementAst] -and $statement.Left -is [Management.Automation.Language.VariableExpressionAst] -and $statement.Left.VariablePath.UserPath -cin @('script:assembleManifestHash','script:assembleHashes','script:installLengths')) { . ([scriptblock]::Create($statement.Extent.Text)) }
}

$raw=$ast.Extent.Text
$package=Join-Path $PSScriptRoot '..\..\..\dist\godusevpn-ultra-0.01-beta-qa.4-windows-amd64'
$package=[IO.Path]::GetFullPath($package)
Check (Test-Path -LiteralPath $package -PathType Container)
$manifestPath=Join-Path $package 'SHA256SUMS'
$manifest=[IO.File]::ReadAllBytes($manifestPath)
$manifestHash=Get-AssembleHash $manifest
Check ($manifestHash -ceq $script:assembleManifestHash)
$hashes=Read-AssembleManifest $manifest
Check ($hashes.Count -eq 406)
Check ((Get-ChildItem -LiteralPath $package -Recurse -File).Count -eq 407)
$descriptor=Get-Content -LiteralPath (Join-Path $package 'ULTRA_PACKAGE.json') -Raw | ConvertFrom-Json
Check ($descriptor.product -ceq 'GodUseVPN Ultra' -and $descriptor.namespace -ceq 'godusevpn-ultra' -and $descriptor.version -ceq '0.01-beta-qa.4' -and $descriptor.architecture -ceq 'amd64' -and $descriptor.unsigned -is [bool] -and $descriptor.unsigned)
Check ($raw.Contains("'0.01-beta-qa.4'") -and $raw.Contains('406 payload files.') -and $raw.Contains('found.Count -ne 407'))
Check ($raw.Contains("'package-beta-qa4'") -and $raw.Contains('ultra-qa4-install-preview-v2'))
Check ($raw.Contains(('$script:assembleManifestHash=' + "'" + $manifestHash + "'")))

$expectedExe=@{
    'godusevpn.exe'='c321a62437521c6c8bfcb5b290c78396dd5fb9b7a2464c2c626d064794b8bafc'
    'godusevpn-cli.exe'='53107fddbbbea4b11030d1aaede6beae6839b27ea7b4412d0e6fadbd9d4044be'
    'godusevpn-svc.exe'='b2ede7f86ebe73b4d8e5b3010507357b6f9eb27af4585f2e0f760189cad083f8'
}
foreach ($name in $expectedExe.Keys) {
    Check ($script:assembleHashes[$name] -ceq $expectedExe[$name])
    Check ($hashes[$name] -ceq $expectedExe[$name])
    Check ((Get-AssembleHash ([IO.File]::ReadAllBytes((Join-Path $package $name)))) -ceq $expectedExe[$name])
    Check ([Diagnostics.FileVersionInfo]::GetVersionInfo((Join-Path $package $name)).ProductVersion -ceq '0.01-beta-qa.4')
}
Check ($script:installLengths['godusevpn.exe'] -eq 16122368 -and $script:installLengths['godusevpn-cli.exe'] -eq 8723456 -and $script:installLengths['godusevpn-svc.exe'] -eq 43166720)

# The installer uses a fixed identical payload validator; only qa version/count/hash constants change.
$qa3Path=Join-Path $PSScriptRoot 'Vps-Qa3InstallV2.ps1'
$qa3Tokens=$null;$qa3Errors=$null;$qa3Ast=[Management.Automation.Language.Parser]::ParseFile($qa3Path,[ref]$qa3Tokens,[ref]$qa3Errors)
Check ($qa3Errors.Count -eq 0)
foreach ($name in @('Get-AssembleHash','Assert-AssembleID','Get-AssembleComponents','Assert-AssemblePrivateSD','Assert-AssemblePrivatePath','Get-AssembleAttributes','Lock-AssembleDirectories','Read-AssembleFile','Assert-AssembleName','Read-AssembleManifest','Get-InstallPolicies','Assert-InstallPoliciesAbsent','Assert-InstallEmptyJournal','Test-InstallAbsent','Assert-InstallNoServices','Assert-InstallEnvironment','Assert-InstallWhatIfOutput','Invoke-InstallWhatIf','Invoke-InstallPreviewPlan')) {
    $old=@($qa3Ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
    $new=@($functions|Where-Object {$_.Name -ceq $name})
    $oldText=$old[0].Extent.Text.Replace("`r`n","`n");$newText=$new[0].Extent.Text.Replace("`r`n","`n")
    if ($name -ceq 'Read-AssembleFile') { $oldText=$oldText.Replace('qa.3','qa.4') }
    if ($name -ceq 'Read-AssembleManifest') { $oldText=$oldText.Replace('351','406') }
    if ($name -ceq 'Assert-InstallWhatIfOutput') { $oldText=$oldText.Replace('qa.3','qa.4').Replace('351','406') }
    Check ($old.Count -eq 1 -and $new.Count -eq 1 -and $oldText -ceq $newText)
}

$native=New-InstallNative
foreach ($name in @('CreateFileW','GetFileInformationByHandleEx','OpenSCManagerW','OpenServiceW','CloseServiceHandle')) {
    $method=$native.GetMethod($name);$attribute=@($method.GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
    Check ($attribute.SetLastError -and $attribute.ExactSpelling -and $attribute.CharSet -eq [Runtime.InteropServices.CharSet]::Unicode)
}
$scm=$native.GetMethod('OpenSCManagerW')
Check ($scm.GetParameters()[0].ParameterType -eq [IntPtr] -and $scm.GetParameters()[1].ParameterType -eq [IntPtr])
Check ($raw.Contains('OpenSCManagerW([IntPtr]::Zero,[IntPtr]::Zero,1)') -and $raw -notmatch 'OpenSCManagerW\(\$null,\$null')
foreach ($forbidden in @('CreateDirectoryW','CreateServiceW','StartServiceW','DeleteServiceW','TerminateProcess','SetTokenInformation')) { Check ($null -eq $native.GetMethod($forbidden)) }
Check ($raw.Contains('actualInstallSupported=$false') -and $raw.Contains('installationRequested=$false') -and $raw.Contains('aclModified=$false'))
Check ($raw.Contains('effectiveSystemCaller=') -and $raw.Contains('system_required') -and $raw.Contains('SCM_unknown') -and $raw.Contains('service_exists'))
Check ($raw.Contains(' -WhatIf') -and $raw.Contains('60000') -and $raw.Contains('65536') -and $raw.Contains('-ne 1060'))
Check (@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -match 'Start-Process|Set-Acl|Remove-Item|New-Item|Invoke-Expression'},$true)).Count -eq 0)
Check ($raw.Contains("'package-beta-qa4'") -and $raw.Contains("status='rejected'"))

$payload='{"version":1,"userKey":"machine-policy-store-v1","controls":{}}'
function Envelope([string]$Value) { return ,[Text.Encoding]::UTF8.GetBytes('{"payload":'+$Value+',"sha256":"'+(Get-AssembleHash ([Text.Encoding]::UTF8.GetBytes($Value)))+'"}') }
Assert-InstallEmptyJournal (Envelope $payload);$checks++
foreach ($bad in @($payload.Replace('machine-policy-store-v1','fictional-user'),$payload.Replace('"controls":{}','"controls":{"managed":{}}'),$payload.Replace('"controls":{}','"controls":{},"controls":{}'))) { Reject { Assert-InstallEmptyJournal (Envelope $bad) } }
Reject { Assert-InstallEmptyJournal ([byte[]]@()) }
Reject { Assert-InstallEmptyJournal ([Text.Encoding]::UTF8.GetBytes('{"payload":'+$payload+',"sha256":"'+('0'*64)+'"}')) }
Assert-AssemblePrivateSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)';$checks++
Reject { Assert-AssemblePrivateSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FR;;;BU)' }
$target='C:\Fixture\GodUseVPN Ultra'
$preview='Verified unsigned Ultra 0.01-beta-qa.4, 406 payload files.'+"`r`n"+'Target: '+$target+"`r`n"+'What if: Create protected directory, copy verified payload, install/start godusevpn-ultra service'
Assert-InstallWhatIfOutput $preview $target;$checks++
Assert-InstallWhatIfOutput ($preview.Replace('copy verified payload,',("copy verified`r`npayload,"))) $target;$checks++
foreach ($bad in @($preview.Replace('406','405'),$preview.Replace('qa.4','qa.3'),$preview.Replace('Create protected directory','Other action'),($preview+"`r`nInstalled Ultra at C:\Fixture"))) { Reject { Assert-InstallWhatIfOutput $bad $target } }
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
    $result=Invoke-InstallPreviewPlan $preflight $whatif
    if ($scenario -ceq 'success') { Check ($result.status -ceq 'completed' -and $result.unchanged -and ($state.events -join ',') -ceq 'preflight,whatif,preflight') }
    elseif ($scenario -in @('timeout','bad_receipt','post_unknown','post_changed')) { Check ($result.status -ceq 'review_required' -and $state.calls -eq 1) }
    else { Check ($result.status -ceq 'rejected' -and $state.calls -eq 0) }
    $json=$result|ConvertTo-Json -Compress;Check ($json.Length -lt 1000 -and $json -notmatch 'fingerprint|fictional|C:|S-1-5')
}

Write-Output ('{"scope":"AST_metadata_real_immutable_manifest_memory_fake_control_flow","checks":'+$checks+ ',"manifestEntries":406,"packageFiles":407,"wrapperMainExecuted":false,"nativeCalled":false,"officialWhatIfExecuted":false,"productExecuted":false,"actualInstallSupported":false}')
