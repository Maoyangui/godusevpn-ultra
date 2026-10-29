# AST-selected pure functions and fictional fixtures ONLY. No main, native
# constructor/PInvoke, real tokens, CIM, registry, processes or remote calls.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$parseErrors=$null
$path=Join-Path $PSScriptRoot 'Vps-Qa3SessionFacts.ps1'
$ast=[Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'session_facts_parser_failed' }
$functions=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
$allowed=@('Assert-StatusAccount','Convert-SessionToken','Test-SessionLinkedMedium','Convert-SessionLUA','Convert-SessionProfile','Select-SessionSource')
foreach ($name in $allowed) {
    $found=@($functions | Where-Object {$_.Name -ceq $name})
    if ($found.Count -ne 1) { throw 'pure_function_missing' }
    . ([scriptblock]::Create($found[0].Extent.Text))
}
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw ('session_fixture_failed_'+$script:checks) };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }
Check ($functions.Count -eq 13 -and @($functions | Group-Object Name | Where-Object {$_.Count -ne 1}).Count -eq 0)
Assert-StatusAccount 'test-account';$checks++
foreach ($bad in @('','domain\name','.\name','name@domain','a b','--flag','username.','name;cmd',('a'*21))) { Reject { Assert-StatusAccount $bad } }
$oldTokens=$null;$oldErrors=$null
$old=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Vps-Qa3MediumStatus.ps1'),[ref]$oldTokens,[ref]$oldErrors)
Check ($oldErrors.Count -eq 0)
foreach ($name in @('Assert-StatusAccount','Get-StatusBuffer','Get-StatusFacts','Get-StatusImage','Resolve-StatusSID')) {
    $before=@($old.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
    $after=@($functions | Where-Object {$_.Name -ceq $name})
    Check ($before.Count -eq 1 -and $after.Count -eq 1 -and $before[0].Extent.Text -ceq $after[0].Extent.Text)
}
$sid='S-1-5-21-1-2-3-1001'
function Token-Fixture { return [pscustomobject]@{sid=$sid;elevated=$false;integrity='S-1-16-8192';elevationType=3;tokenType=1;adminEnabled=$false;session=2} }
$full=Token-Fixture;$full.elevated=$true;$full.integrity='S-1-16-12288';$full.elevationType=2;$full.adminEnabled=$true
$token=Convert-SessionToken (Token-Fixture) $sid
Check ($token.exactUserMatched -and -not $token.elevated -and $token.integrity -ceq 'medium' -and $token.elevationType -ceq 'limited' -and $token.tokenType -ceq 'primary')
Check ((Convert-SessionToken $full $sid).integrity -ceq 'high')
$missing=Convert-SessionToken $null $sid
Check ($missing.exactUserMatched -ceq 'unknown' -and $missing.elevated -ceq 'unknown')
$future=Token-Fixture;$future.integrity='S-1-16-999999';$future.elevationType=99;$future.tokenType='1';$future.elevated='true'
$unknown=Convert-SessionToken $future $sid
Check ($unknown.integrity -ceq 'unknown' -and $unknown.elevationType -ceq 'unknown' -and $unknown.tokenType -ceq 'unknown' -and $unknown.elevated -ceq 'unknown')
Check (Test-SessionLinkedMedium (Token-Fixture) $full $sid)
foreach ($field in @('sid','elevated','integrity','elevationType','tokenType','adminEnabled','session')) {
    $bad=Token-Fixture
    switch ($field) { 'sid' {$bad.sid='S-1-5-18'} 'elevated' {$bad.elevated=$true} 'integrity' {$bad.integrity='S-1-16-12288'} 'elevationType' {$bad.elevationType=2} 'tokenType' {$bad.tokenType=2} 'adminEnabled' {$bad.adminEnabled=$true} 'session' {$bad.session=3} }
    Check (-not (Test-SessionLinkedMedium $bad $full $sid))
}
Check (-not (Test-SessionLinkedMedium $null $full $sid))
Check (-not (Test-SessionLinkedMedium (Token-Fixture) (Token-Fixture) $sid))
Check ((Convert-SessionLUA 'DWord' 1) -ceq 'enabled')
Check ((Convert-SessionLUA 'DWord' 0) -ceq 'disabled')
foreach ($case in @(@('DWord',$null),@('String','1'),@('DWord','1'),@('DWord',2),@('unknown',1))) { Check ((Convert-SessionLUA $case[0] $case[1]) -ceq 'unknown') }
function Profile-Fixture { return [pscustomobject]@{SID=$sid;Loaded=$true;Status=[uint32]1} }
$profile=Convert-SessionProfile @(Profile-Fixture) $sid
Check ($profile.status -ceq 'observed' -and $profile.present -and $profile.exactUserMatched -and $profile.loaded -and $profile.temporaryFlag)
Check ((Convert-SessionProfile @() $sid).status -ceq 'absent')
Check ((Convert-SessionProfile @((Profile-Fixture),(Profile-Fixture)) $sid).status -ceq 'ambiguous')
$bad=Profile-Fixture;$bad.SID='S-1-5-18';Check ((Convert-SessionProfile @($bad) $sid).status -ceq 'unknown')
$bad=Profile-Fixture;$bad.Loaded='false';$bad.Status=16;$p=Convert-SessionProfile @($bad) $sid
Check ($p.loaded -ceq 'unknown' -and $p.temporaryFlag -ceq 'unknown')
$bad=Profile-Fixture;$bad.Status=0;Check (-not (Convert-SessionProfile @($bad) $sid).temporaryFlag)
$cmd='C:\Windows\System32\cmd.exe'
function Cmd-Fixture([int]$ID=123) { return [pscustomobject]@{ProcessId=[uint32]$ID;Handle=([string]$ID);ExecutablePath=$cmd} }
$owner={param($p) [pscustomobject]@{ReturnValue=[uint32]0;Sid=$sid}}
$one=Select-SessionSource @(Cmd-Fixture) $cmd $sid $owner
Check ($one.status -ceq 'unique' -and $one.counts.ownerMatched -eq 1 -and $null -ne $one.selected)
$none=Select-SessionSource @() $cmd $sid $owner
Check ($none.status -ceq 'no_source' -and $none.counts.enumerated -eq 0 -and $null -eq $none.selected)
$many=Select-SessionSource @((Cmd-Fixture 123),(Cmd-Fixture 124)) $cmd $sid $owner
Check ($many.status -ceq 'ambiguous' -and $many.counts.ownerMatched -eq 2 -and $null -eq $many.selected)
$script:ownerReads=0;$capped=@();for ($i=1;$i -le 64;$i++) { $capped+=Cmd-Fixture $i }
$cap=Select-SessionSource $capped $cmd $sid {param($p) $script:ownerReads++;throw 'must_not_read'}
Check ($cap.status -ceq 'ambiguous' -and $cap.counts.limitReached -and $script:ownerReads -eq 0 -and $null -eq $cap.selected)
$other={param($p) [pscustomobject]@{ReturnValue=[uint32]0;Sid='S-1-5-18'}}
Check ((Select-SessionSource @(Cmd-Fixture) $cmd $sid $other).status -ceq 'no_source')
foreach ($callback in @({param($p) throw 'synthetic_private_error'},{param($p) [pscustomobject]@{ReturnValue=1;Sid=$sid}},{param($p) [pscustomobject]@{ReturnValue='0';Sid=$sid}},{param($p) [pscustomobject]@{ReturnValue=0;Sid='not-a-sid'}})) {
    $s=Select-SessionSource @(Cmd-Fixture) $cmd $sid $callback
    Check ($s.status -ceq 'unknown' -and $s.counts.ownerUnknown -eq 1 -and $null -eq $s.selected)
}
$bad=Cmd-Fixture;$bad.ExecutablePath=$null;Check ((Select-SessionSource @($bad) $cmd $sid $owner).status -ceq 'unknown')
$bad=Cmd-Fixture;$bad.ExecutablePath='C:\Windows\SysWOW64\cmd.exe';Check ((Select-SessionSource @($bad) $cmd $sid $owner).status -ceq 'no_source')
$bad=Cmd-Fixture;$bad.Handle='124';Check ((Select-SessionSource @($bad) $cmd $sid $owner).status -ceq 'unknown')
$bad=Cmd-Fixture;$bad.ProcessId='123';Check ((Select-SessionSource @($bad) $cmd $sid $owner).status -ceq 'unknown')
$raw=$ast.Extent.Text
$native=@($functions | Where-Object {$_.Name -ceq 'New-SessionNative'})[0].Extent.Text
$names=@([regex]::Matches($native,"@\('(?:kernel32|advapi32)\.dll','(?<name>[A-Za-z0-9]+)'") | ForEach-Object {$_.Groups['name'].Value})
$expected=@('CloseHandle','OpenProcess','WaitForSingleObject','QueryFullProcessImageNameW','OpenProcessToken','OpenThreadToken','GetTokenInformation','LookupAccountNameW')
Check ($names.Count -eq 8 -and @($names | Where-Object {$expected -cnotcontains $_}).Count -eq 0)
foreach ($blocked in @('DuplicateToken','ImpersonateLoggedOnUser','CreateProcess','CreateEnvironment','AdjustTokenPrivileges','SetTokenInformation','TerminateProcess','Set-Acl','Get-Credential','CommandLine','LocalPath','LoadUserProfile')) { Check (-not $raw.Contains($blocked)) }
$cim=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -ceq 'Get-CimInstance'},$true))
Check ($cim.Count -eq 2)
Check (@($cim | Where-Object {$_.Extent.Text.Contains('Win32_Process') -and $_.Extent.Text.Contains('-Property ProcessId,ExecutablePath,Handle')}).Count -eq 1)
Check (@($cim | Where-Object {$_.Extent.Text.Contains('Win32_UserProfile') -and $_.Extent.Text.Contains('-Property SID,Loaded,Status')}).Count -eq 1)
Check ($raw.Contains('OpenProcess(0x101000,$false,') -and $raw.Contains('OpenProcessToken($source,8,') -and $raw.Contains('Get-StatusBuffer $sourceToken 19'))
Check ($raw.Contains("OpenSubKey('SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System',`$false)") -and -not $raw.Contains('SetValue('))
$out=[ordered]@{sourceCounts=$many.counts;token=$token;linkedToken=(Convert-SessionToken (Token-Fixture) $sid);profile=$profile;enableLUA='enabled'} | ConvertTo-Json -Compress -Depth 5
Check ($out.Length -lt 1600 -and -not $out.Contains($sid) -and -not $out.Contains('C:') -and -not $out.Contains('ProcessId') -and -not $out.Contains('session'))
[Console]::Out.WriteLine(('{"scope":"AST_and_fictional_fixtures_only","checks":'+$checks+',"copiedHelpers":5,"mainExecuted":false,"nativeConstructorCalled":false,"nativeCalled":false,"realTokenRead":false,"registryRead":false,"cimRead":false,"productExecuted":false}'))
