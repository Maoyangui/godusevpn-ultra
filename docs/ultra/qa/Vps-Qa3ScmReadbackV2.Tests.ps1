# AST / metadata only. Does not invoke the script main block or native methods.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$path=Join-Path $PSScriptRoot 'Vps-Qa3ScmReadbackV2.ps1'
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$errors)
if ($errors.Count) { throw 'parser_failed' }
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw ('assertion_failed_'+$script:checks) };$script:checks++ }
Check (-not [regex]::IsMatch($ast.Extent.Text,'[^\x00-\x7f]'))
$functions=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
Check (@($functions|Group-Object Name|Where-Object {$_.Count -ne 1}).Count -eq 0)
$new=@($functions|Where-Object Name -ceq 'New-ScmNative');Check $new.Count -eq 1
. ([scriptblock]::Create($new[0].Extent.Text))
$native=New-ScmNative
foreach($name in @('OpenSCManagerW','OpenServiceW','CloseServiceHandle')) {
    $m=$native.GetMethod($name);$a=@($m.GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
    Check ($a.SetLastError -and $a.ExactSpelling -and $a.CharSet -eq [Runtime.InteropServices.CharSet]::Unicode)
}
$scm=$native.GetMethod('OpenSCManagerW');Check ($scm.GetParameters()[0].ParameterType -eq [IntPtr] -and $scm.GetParameters()[1].ParameterType -eq [IntPtr])
$raw=$ast.Extent.Text
Check ($raw.Contains('OpenSCManagerW([IntPtr]::Zero,[IntPtr]::Zero,1)'))
Check ($raw -notmatch 'OpenSCManagerW\(\$null|OpenSCManagerW\([^,]+,\$null')
Check ($raw.Contains("schema='ultra-qa3-scm-readback-v2'"))
Check ($raw.Contains('Get-CimInstance Win32_Service'))
foreach($forbidden in @('CreateServiceW','StartServiceW','DeleteServiceW','CreateFileW','Set-Acl','Remove-Item','New-Item','Start-Process','Invoke-Expression','Set-ItemProperty')) { Check (@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -ceq $forbidden},$true)).Count -eq 0) }
Check ($raw.Contains('productExecuted=$false') -and $raw.Contains('changesRequested=$false'))
Write-Output ('{"scope":"AST_metadata_only","checks":'+$checks+ ',"mainExecuted":false,"nativeCalled":false,"mutations":false}')
