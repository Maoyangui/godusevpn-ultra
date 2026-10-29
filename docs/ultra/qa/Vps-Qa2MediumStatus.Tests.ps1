# Pure AST, Reflection.Emit metadata, allocated zeroed memory and fake lifecycle.
# Never main, CIM, token/PInvoke, a real child process, product or service.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Vps-Qa2MediumStatus.ps1'),[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'parser_failed' }
foreach ($name in @('Assert-StatusAccount','Assert-StatusService','Assert-StatusMedium','New-StatusNative','Assert-StatusCodeSD','New-StatusProcessBuffers','Assert-StatusChildFacts','Invoke-StatusLifecycle')) {
    $found=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
    if ($found.Count -ne 1) { throw 'function_missing' }
    . ([scriptblock]::Create($found[0].Extent.Text))
}
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw ('assertion_failed_'+$script:checks) };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }
Check (@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true) | Group-Object Name | Where-Object {$_.Count -ne 1}).Count -eq 0)
Assert-StatusAccount 'test-account';$checks++
foreach ($bad in @('','domain\name','.\name','name@domain','a b','--flag','username.','name;cmd',('a'*21))) { Reject { Assert-StatusAccount $bad } }
$image='C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe'
function Service-Fixture { return [pscustomobject]@{Name='godusevpn-ultra';State='Running';StartName='LocalSystem';ServiceType='Own Process';DesktopInteract=$false;ProcessId=[uint32]123;PathName=('"'+$image+'" service')} }
Assert-StatusService (Service-Fixture) $image;$checks++
foreach ($field in @('StartName','State','ServiceType','Name','PathName')) { $value=Service-Fixture;$value.$field='wrong';Reject { Assert-StatusService $value $image } }
foreach ($account in @('.\LocalSystem','NT AUTHORITY\SYSTEM',('LocalSy'+[char]0x17f+'tem'))) { $value=Service-Fixture;$value.StartName=$account;Reject { Assert-StatusService $value $image } }
foreach ($command in @(('"'+$image+'" SERVICE'),('"'+$image+'" service extra'),($image+' service'))) { $value=Service-Fixture;$value.PathName=$command;Reject { Assert-StatusService $value $image } }
$sid='S-1-5-21-1-2-3-1001';$cli='C:\Program Files\GodUseVPN Ultra\godusevpn-cli.exe'
function Medium-Fixture { return [pscustomobject]@{sid=$sid;integrity='S-1-16-8192';elevated=$false;adminEnabled=$false;tokenType=1;session=2} }
Assert-StatusChildFacts (Medium-Fixture) $sid 2 $cli $cli;$checks++
foreach ($field in @('sid','integrity','elevated','adminEnabled','tokenType','session')) {
    $facts=Medium-Fixture
    switch ($field) { 'sid' {$facts.sid='S-1-5-18'} 'integrity' {$facts.integrity='S-1-16-12288'} 'elevated' {$facts.elevated=$true} 'adminEnabled' {$facts.adminEnabled=$true} 'tokenType' {$facts.tokenType=2} 'session' {$facts.session=3} }
    Reject { Assert-StatusChildFacts $facts $sid 2 $cli $cli }
}
Reject { Assert-StatusChildFacts (Medium-Fixture) $sid 2 'C:\wrong.exe' $cli }
Assert-StatusCodeSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;FRFX;;;BU)';$checks++
Reject { Assert-StatusCodeSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FW;;;BU)' }
Reject { Assert-StatusCodeSD 'O:BUG:BUD:P(A;;FA;;;SY)' }
$native=New-StatusNative
foreach ($name in @('CreateProcessAsUserW','CreateEnvironmentBlock','DestroyEnvironmentBlock','ResumeThread','TerminateProcess','GetExitCodeProcess','OpenProcessToken','DuplicateTokenEx')) {
    $method=$native.GetMethod($name);$attribute=@($method.GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
    Check ($attribute.SetLastError -and $attribute.ExactSpelling -and $attribute.CharSet -eq [Runtime.InteropServices.CharSet]::Unicode)
}
$parameters=$native.GetMethod('CreateProcessAsUserW').GetParameters()
Check ($parameters.Count -eq 11 -and $parameters[2].ParameterType -eq [Text.StringBuilder] -and $parameters[5].ParameterType -eq [bool] -and $parameters[9].ParameterType -eq [IntPtr] -and $parameters[10].ParameterType -eq [IntPtr])
Check ($null -eq $native.GetMethod('Probe') -and $null -eq $native.GetMethod('ImpersonateLoggedOnUser') -and $null -eq $native.GetMethod('RevertToSelf'))
$buffers=New-StatusProcessBuffers
try {
    $startup=New-Object byte[] 104;$process=New-Object byte[] 24
    [Runtime.InteropServices.Marshal]::Copy($buffers.startup,$startup,0,104)
    [Runtime.InteropServices.Marshal]::Copy($buffers.process,$process,0,24)
    Check ([BitConverter]::ToInt32($startup,0)-eq 104 -and @($startup[4..103]|Where-Object {$_ -ne 0}).Count -eq 0)
    Check (@($process|Where-Object {$_ -ne 0}).Count -eq 0)
    # Only memory: prove x64 PROCESS_INFORMATION process/thread offsets.
    [Runtime.InteropServices.Marshal]::WriteIntPtr($buffers.process,0,[IntPtr]321)
    [Runtime.InteropServices.Marshal]::WriteIntPtr($buffers.process,8,[IntPtr]654)
    Check ([Runtime.InteropServices.Marshal]::ReadIntPtr($buffers.process,0)-eq [IntPtr]321 -and [Runtime.InteropServices.Marshal]::ReadIntPtr($buffers.process,8)-eq [IntPtr]654)
} finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffers.startup);[Runtime.InteropServices.Marshal]::FreeHGlobal($buffers.process) }
function Reset-Fixture {
    $script:events=New-Object 'Collections.Generic.List[string]'
    $script:scenario='success'
    $script:summary=[ordered]@{childCreated=$false;childValidated=$false;resumeAttempted=$false;productExecuted=$false;statusCommandExecuted=$false;childExited=$false;statusExitZero=$false;terminationAttempted=$false;terminationConfirmed=$false;resultUnknown=$false}
    $script:operations=@{
        Create={ $script:events.Add('create');if ($script:scenario -eq 'createfail') { throw 'child_create_failed' };[pscustomobject]@{process=[IntPtr]321;thread=[IntPtr]654} }
        Validate={ param($child) $script:events.Add('validate');if ($child.process-ne [IntPtr]321 -or $script:scenario-eq 'validatefail') { throw 'child_identity_rejected' } }
        Resume={ param($handle) $script:events.Add('resume');if ($handle-ne [IntPtr]654) { throw 'bad_thread' };if ($script:scenario-eq 'resumefail') { return [uint32]::MaxValue };[uint32]1 }
        Wait={ param($handle,$milliseconds) $script:events.Add(('wait'+$milliseconds));if ($handle-ne [IntPtr]321) { throw 'bad_process' };if ($milliseconds-eq 15000 -and $script:scenario-in @('timeout','killfail')) { return [uint32]258 };[uint32]0 }
        Exit={ param($handle) $script:events.Add('exit');if ($script:scenario-eq 'exitunknown') { throw 'child_exit_unknown' };if ($script:scenario-eq 'nonzero') { return [uint32]1 };[uint32]0 }
        Terminate={ param($handle) $script:events.Add('terminate');if ($handle-ne [IntPtr]321) { throw 'bad_process' };$script:scenario-ne 'killfail' }
        Close={ param($handle) $script:events.Add(('close'+$handle.ToInt64()));if ($script:scenario-eq 'closefail' -and $handle-eq [IntPtr]654) { throw 'child_close_failed' } }
    }
}
Reset-Fixture;Invoke-StatusLifecycle $operations $summary
Check (($events -join ',')-ceq 'create,validate,resume,wait15000,exit,close654,close321')
Check ($summary.statusExitZero -and $summary.productExecuted -and -not $summary.terminationAttempted -and -not $summary.resultUnknown)
Reset-Fixture;$scenario='createfail';Reject { Invoke-StatusLifecycle $operations $summary }
Check (($events -join ',')-ceq 'create' -and -not $summary.childCreated -and -not $summary.productExecuted)
Reset-Fixture;$scenario='validatefail';Reject { Invoke-StatusLifecycle $operations $summary }
Check (($events -join ',')-ceq 'create,validate,terminate,wait1000,close654,close321' -and -not $summary.resumeAttempted -and -not $summary.productExecuted -and $summary.terminationConfirmed)
Reset-Fixture;$scenario='resumefail';Reject { Invoke-StatusLifecycle $operations $summary }
Check (($events -join ',')-ceq 'create,validate,resume,terminate,wait1000,close654,close321' -and $summary.resultUnknown -and -not $summary.statusExitZero)
Reset-Fixture;$scenario='timeout';Reject { Invoke-StatusLifecycle $operations $summary }
Check (($events -join ',')-ceq 'create,validate,resume,wait15000,terminate,wait1000,close654,close321' -and $summary.resultUnknown -and $summary.productExecuted -and $summary.terminationConfirmed)
Reset-Fixture;$scenario='killfail';Reject { Invoke-StatusLifecycle $operations $summary }
Check (@($events|Where-Object {$_ -eq 'terminate'}).Count -eq 1 -and @($events|Where-Object {$_ -eq 'create'}).Count -eq 1 -and -not $summary.terminationConfirmed -and $summary.resultUnknown)
foreach ($failure in @('nonzero','exitunknown','closefail')) {
    Reset-Fixture;$scenario=$failure;Reject { Invoke-StatusLifecycle $operations $summary }
    Check ($summary.childExited -and -not $summary.terminationAttempted -and $events[$events.Count-1]-ceq 'close321')
    if ($failure -ceq 'exitunknown') { Check $summary.resultUnknown }
}
$raw=$ast.Extent.Text
Check ($raw.Contains('CreateProcessAsUserW($Token,$CLI,$command,[IntPtr]::Zero,[IntPtr]::Zero,$false,0x08000404,') -and $raw.Contains('DuplicateTokenEx($selected,11,[IntPtr]::Zero,2,1,'))
Check ($raw.Contains('-Property ProcessId,ExecutablePath,Handle') -and -not $raw.Contains('StandardOutput') -and -not $raw.Contains('StandardError'))
Check (@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -match 'Start-Process|Invoke-Expression|Set-Acl|Register|Restart-Service|Stop-Process'},$true)).Count -eq 0)
Check (-not $raw.Contains('AdjustTokenPrivileges') -and -not $raw.Contains('LoadUserProfile') -and -not $raw.Contains('SetTokenInformation'))
$serialized=$summary|ConvertTo-Json -Compress
Check (-not $serialized.Contains($sid) -and -not $serialized.Contains('C:') -and $serialized.Length-lt 1000)
[Console]::Out.WriteLine(('{"scope":"AST_metadata_memory_fake_only","checks":'+$checks+',"nativeCalled":false,"tokenRead":false,"mainExecuted":false,"productExecuted":false}'))
