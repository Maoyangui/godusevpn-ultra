# Parse/load only pure facts, native metadata/IL; never call Probe, token APIs,
# main, CIM, product/status, filesystem native methods or a service.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Vps-Qa2MediumTokenProbe.ps1'),[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'parser_failed' }
foreach ($name in @('Assert-ProbeAccount','Assert-ProbeService','Assert-ProbeMedium','Convert-ProbeResult','New-ProbeNative','Assert-ProbeCodeSD')) {
    $found=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
    if ($found.Count -ne 1) { throw 'function_missing' }
    . ([scriptblock]::Create($found[0].Extent.Text))
}
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw 'assertion_failed' };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }
Assert-ProbeAccount 'test-account';$checks++
foreach ($bad in @('','domain\name','.\name','name@domain','a b','--flag','username.','name;cmd',('a'*21))) { Reject { Assert-ProbeAccount $bad } }
$image='C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe'
function Service-Fixture { return [pscustomobject]@{Name='godusevpn-ultra';State='Running';StartName='LocalSystem';ServiceType='Own Process';DesktopInteract=$false;ProcessId=[uint32]123;PathName=('"'+$image+'" service')} }
Assert-ProbeService (Service-Fixture) $image;$checks++
foreach ($field in @('StartName','State','ServiceType','Name','PathName')) {
    $value=Service-Fixture;$value.$field='wrong';Reject { Assert-ProbeService $value $image }
}
foreach ($account in @('.\LocalSystem','NT AUTHORITY\SYSTEM',('LocalSy'+[char]0x17f+'tem'))) { $value=Service-Fixture;$value.StartName=$account;Reject { Assert-ProbeService $value $image } }
foreach ($command in @(('"'+$image+'" SERVICE'),('"'+$image+'" service extra'),($image+' service'))) { $value=Service-Fixture;$value.PathName=$command;Reject { Assert-ProbeService $value $image } }
$sid='S-1-5-21-1-2-3-1001'
function Medium-Fixture { return [pscustomobject]@{sid=$sid;integrity='S-1-16-8192';elevated=$false;adminEnabled=$false;tokenType=1} }
Assert-ProbeMedium (Medium-Fixture) $sid 1;$checks++
foreach ($field in @('sid','integrity','elevated','adminEnabled','tokenType')) {
    $facts=Medium-Fixture
    switch ($field) { 'sid' {$facts.sid='S-1-5-18'} 'integrity' {$facts.integrity='S-1-16-12288'} 'elevated' {$facts.elevated=$true} 'adminEnabled' {$facts.adminEnabled=$true} 'tokenType' {$facts.tokenType=2} }
    Reject { Assert-ProbeMedium $facts $sid 1 }
}
$denied=Convert-ProbeResult ([int[]]@(1,0,1,0,0,5,1,0,0))
Check ($denied.impersonated -and $denied.processOpened -and -not $denied.tokenQueryOpened -and $denied.tokenQueryWin32 -eq 5 -and $denied.revertConfirmed)
$unknown=Convert-ProbeResult ([int[]]@(1,0,1,0,0,5,0,5,0))
Check (-not $unknown.revertConfirmed -and $unknown.revertWin32 -eq 5)
Reject { Convert-ProbeResult ([int[]]@(0,5,0,0,1,0,0,0,0)) }
Reject { Convert-ProbeResult ([int[]]@(1,0,1,0,0,-1,1,0,0)) }
$serialized=$denied|ConvertTo-Json -Compress
Check (-not $serialized.Contains($sid) -and -not $serialized.Contains('C:') -and $serialized.Length -lt 1000)
Assert-ProbeCodeSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;FRFX;;;BU)';$checks++
Reject { Assert-ProbeCodeSD 'O:BAG:BAD:P(A;;FA;;;BA)(A;;FW;;;BU)' }
Reject { Assert-ProbeCodeSD 'O:BUG:BUD:P(A;;FA;;;SY)' }
$native=New-ProbeNative
foreach ($name in @('ImpersonateLoggedOnUser','RevertToSelf','OpenProcess','OpenProcessToken','OpenThreadToken','DuplicateTokenEx')) {
    $m=$native.GetMethod($name);$attribute=@($m.GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
    Check ($attribute.SetLastError -and $attribute.ExactSpelling)
}
$method=$native.GetMethod('Probe')
Check ($method.ReturnType -eq [int[]] -and $method.GetParameters().Count -eq 2)
$clauses=@($method.GetMethodBody().ExceptionHandlingClauses)
Check (@($clauses | Where-Object {$_.Flags -eq [Reflection.ExceptionHandlingClauseOptions]::Finally}).Count -eq 1)
Check (@($clauses | Where-Object {$_.Flags -eq [Reflection.ExceptionHandlingClauseOptions]::Clause}).Count -eq 1)
# JIT validates the emitted control flow only. It does not invoke the method,
# P/Invoke functions, impersonation, a token or any probe against a real PID.
[Runtime.CompilerServices.RuntimeHelpers]::PrepareMethod($method.MethodHandle)
$checks++
Check (@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -match 'Start-Process|Invoke-Expression|Set-Acl'},$true)).Count -eq 0)
[Console]::Out.WriteLine(('{"scope":"AST_metadata_IL_only","checks":'+$checks+',"nativeProbeCalled":false,"tokenRead":false,"mainExecuted":false,"productExecuted":false}'))
