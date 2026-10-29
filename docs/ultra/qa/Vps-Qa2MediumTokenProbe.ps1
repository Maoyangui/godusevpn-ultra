# Windows PowerShell 5.1 x64, SYSTEM. QA capability probe ONLY.
# No product/status/GUI/service execution, registration, DACL/policy changes,
# new user, password or retry. A real existing local user's cmd token is used;
# restricted SYSTEM is never a substitute. Requires separate execution approval.
# Fixed qa.2 installed CLI/service hashes; SCM configuration is not runtime SID.
[CmdletBinding()]
param([string]$TestAccount)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue';$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue';$DebugPreference='SilentlyContinue'

function Assert-ProbeAccount([string]$Name) {
    if ($Name -cnotmatch '^[A-Za-z0-9][A-Za-z0-9_.-]{0,19}$' -or $Name.EndsWith('.')) { throw 'account_rejected' }
}
function Assert-ProbeService($Value,[string]$Image) {
    if ($null -eq $Value -or $Value.Name -cne 'godusevpn-ultra' -or $Value.State -cne 'Running' -or $Value.StartName -cne 'LocalSystem' -or $Value.ServiceType -cne 'Own Process' -or $Value.DesktopInteract -isnot [bool] -or $Value.DesktopInteract -or [long]$Value.ProcessId -lt 1 -or [long]$Value.ProcessId -gt [uint32]::MaxValue) { throw 'service_rejected' }
    $command=[string]$Value.PathName
    if ($command.Length -lt 11 -or -not $command.StartsWith('"',[StringComparison]::Ordinal) -or -not $command.EndsWith('" service',[StringComparison]::Ordinal) -or -not [string]::Equals($command.Substring(1,$command.Length-10),$Image,[StringComparison]::OrdinalIgnoreCase)) { throw 'service_rejected' }
}
function Assert-ProbeMedium($Facts,[string]$SID,[int]$Type) {
    if ($null -eq $Facts -or $Facts.sid -cne $SID -or $Facts.sid -ceq 'S-1-5-18' -or $Facts.integrity -cne 'S-1-16-8192' -or $Facts.elevated -isnot [bool] -or $Facts.elevated -or $Facts.adminEnabled -isnot [bool] -or $Facts.adminEnabled -or $Facts.tokenType -ne $Type) { throw 'medium_token_rejected' }
}
function Convert-ProbeResult([int[]]$Raw) {
    if ($Raw.Length -ne 9) { throw 'probe_result_rejected' }
    foreach ($i in @(0,2,4,6,8)) { if ($Raw[$i] -notin @(0,1)) { throw 'probe_result_rejected' } }
    foreach ($i in @(1,3,5,7)) { if ($Raw[$i] -lt 0 -or $Raw[$i] -gt 65535) { throw 'probe_result_rejected' } }
    if (($Raw[2] -eq 1 -and $Raw[0] -ne 1) -or ($Raw[4] -eq 1 -and ($Raw[0] -ne 1 -or $Raw[2] -ne 1)) -or ($Raw[6] -eq 1 -and $Raw[0] -ne 1)) { throw 'probe_result_rejected' }
    return [ordered]@{impersonated=($Raw[0]-eq 1);impersonateWin32=$Raw[1];processOpened=($Raw[2]-eq 1);processWin32=$Raw[3];tokenQueryOpened=($Raw[4]-eq 1);tokenQueryWin32=$Raw[5];revertConfirmed=($Raw[6]-eq 1);revertWin32=$Raw[7];nativeException=($Raw[8]-eq 1)}
}
function New-ProbeNative {
    $assembly=New-Object Reflection.AssemblyName('UltraQaMediumProbe')
    $builder=[AppDomain]::CurrentDomain.DefineDynamicAssembly($assembly,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $type=$builder.DefineDynamicModule('Native').DefineType('UltraQaMediumProbeNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $intptrRef=[IntPtr].MakeByRefType();$uintRef=[uint32].MakeByRefType()
    $specs=@(
        @('kernel32.dll','CreateFileW',[IntPtr],[type[]]@([string],[uint32],[uint32],[IntPtr],[uint32],[uint32],[IntPtr])),
        @('kernel32.dll','GetFileInformationByHandleEx',[bool],[type[]]@([IntPtr],[int],[IntPtr],[uint32])),
        @('kernel32.dll','CloseHandle',[bool],[type[]]@([IntPtr])),
        @('kernel32.dll','QueryDosDeviceW',[uint32],[type[]]@([string],[Text.StringBuilder],[uint32])),
        @('kernel32.dll','OpenProcess',[IntPtr],[type[]]@([uint32],[bool],[uint32])),
        @('kernel32.dll','WaitForSingleObject',[uint32],[type[]]@([IntPtr],[uint32])),
        @('kernel32.dll','QueryFullProcessImageNameW',[bool],[type[]]@([IntPtr],[uint32],[Text.StringBuilder],$uintRef)),
        @('advapi32.dll','OpenProcessToken',[bool],[type[]]@([IntPtr],[uint32],$intptrRef)),
        @('advapi32.dll','OpenThreadToken',[bool],[type[]]@([IntPtr],[uint32],[bool],$intptrRef)),
        @('advapi32.dll','GetTokenInformation',[bool],[type[]]@([IntPtr],[int],[IntPtr],[uint32],$uintRef)),
        @('advapi32.dll','DuplicateTokenEx',[bool],[type[]]@([IntPtr],[uint32],[IntPtr],[int],[int],$intptrRef)),
        @('advapi32.dll','ImpersonateLoggedOnUser',[bool],[type[]]@([IntPtr])),
        @('advapi32.dll','RevertToSelf',[bool],[type[]]@()),
        @('advapi32.dll','LookupAccountNameW',[bool],[type[]]@([string],[string],[IntPtr],$uintRef,[Text.StringBuilder],$uintRef,$uintRef))
    )
    $ctor=[Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields=[Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'),[Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'),[Runtime.InteropServices.DllImportAttribute].GetField('CharSet'),[Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    $methods=@{}
    foreach ($s in $specs) {
        $method=$type.DefineMethod($s[1],[Reflection.MethodAttributes]'Public, Static, PinvokeImpl',$s[2],$s[3]);$method.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $method.SetCustomAttribute((New-Object Reflection.Emit.CustomAttributeBuilder($ctor,[object[]]@($s[0]),$fields,[object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi))))
        $methods[$s[1]]=$method
    }
    # All impersonation, probe calls, handle closure and RevertToSelf execute
    # in ONE emitted synchronous CLR method with a finally, not a PS pipeline.
    $probe=$type.DefineMethod('Probe',[Reflection.MethodAttributes]'Public, Static',[int[]],[type[]]@([IntPtr],[uint32]))
    $il=$probe.GetILGenerator();$ops=[Reflection.Emit.OpCodes]
    $result=$il.DeclareLocal([int[]]);$active=$il.DeclareLocal([bool]);$process=$il.DeclareLocal([IntPtr]);$token=$il.DeclareLocal([IntPtr]);$okay=$il.DeclareLocal([bool])
    $last=[Runtime.InteropServices.Marshal].GetMethod('GetLastWin32Error',[type[]]@())
    $store={param([int]$Index,[scriptblock]$Load) $il.Emit($ops::Ldloc,$result);$il.Emit($ops::Ldc_I4,$Index);&$Load;$il.Emit($ops::Stelem_I4)}
    $il.Emit($ops::Ldc_I4,9);$il.Emit($ops::Newarr,[int]);$il.Emit($ops::Stloc,$result)
    $end=$il.BeginExceptionBlock()
    $il.Emit($ops::Ldarg_0);$il.Emit($ops::Call,$methods.ImpersonateLoggedOnUser);$il.Emit($ops::Stloc,$active)
    &$store 0 {$il.Emit($ops::Ldloc,$active)}
    $impersonated=$il.DefineLabel();$il.Emit($ops::Ldloc,$active);$il.Emit($ops::Brtrue,$impersonated)
    &$store 1 {$il.Emit($ops::Call,$last)};$il.Emit($ops::Leave,$end);$il.MarkLabel($impersonated)
    $il.Emit($ops::Ldc_I4,0x101000);$il.Emit($ops::Ldc_I4_0);$il.Emit($ops::Ldarg_1);$il.Emit($ops::Call,$methods.OpenProcess);$il.Emit($ops::Stloc,$process)
    $opened=$il.DefineLabel();$il.Emit($ops::Ldloc,$process);$il.Emit($ops::Brtrue,$opened)
    &$store 3 {$il.Emit($ops::Call,$last)};$il.Emit($ops::Leave,$end);$il.MarkLabel($opened)
    &$store 2 {$il.Emit($ops::Ldc_I4_1)}
    $il.Emit($ops::Ldloc,$process);$il.Emit($ops::Ldc_I4_8);$il.Emit($ops::Ldloca,$token);$il.Emit($ops::Call,$methods.OpenProcessToken);$il.Emit($ops::Stloc,$okay)
    &$store 4 {$il.Emit($ops::Ldloc,$okay)}
    $queried=$il.DefineLabel();$il.Emit($ops::Ldloc,$okay);$il.Emit($ops::Brtrue,$queried)
    &$store 5 {$il.Emit($ops::Call,$last)};$il.MarkLabel($queried);$il.Emit($ops::Leave,$end)
    $il.BeginCatchBlock([Exception]);$il.Emit($ops::Pop);&$store 8 {$il.Emit($ops::Ldc_I4_1)};$il.Emit($ops::Leave,$end)
    $il.BeginFinallyBlock()
    # Revert precedes even handle cleanup, so a cleanup exception cannot skip it.
    $skipRevert=$il.DefineLabel();$il.Emit($ops::Ldloc,$active);$il.Emit($ops::Brfalse,$skipRevert)
    $il.Emit($ops::Call,$methods.RevertToSelf);$il.Emit($ops::Stloc,$okay);&$store 6 {$il.Emit($ops::Ldloc,$okay)}
    $il.Emit($ops::Ldloc,$okay);$il.Emit($ops::Brtrue,$skipRevert);&$store 7 {$il.Emit($ops::Call,$last)};$il.MarkLabel($skipRevert)
    foreach ($handle in @($token,$process)) {
        $skip=$il.DefineLabel();$il.Emit($ops::Ldloc,$handle);$il.Emit($ops::Brfalse,$skip);$il.Emit($ops::Ldloc,$handle);$il.Emit($ops::Call,$methods.CloseHandle);$il.Emit($ops::Pop);$il.MarkLabel($skip)
    }
    $il.EndExceptionBlock();$il.Emit($ops::Ldloc,$result);$il.Emit($ops::Ret)
    return $type.CreateType()
}
function Get-ProbeBuffer([IntPtr]$Token,[int]$Class) {
    [uint32]$size=0
    $null=$script:probeNative::GetTokenInformation($Token,$Class,[IntPtr]::Zero,0,[ref]$size)
    if ($size -lt 1 -or $size -gt 1048576) { throw 'token_read_failed' }
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal([int]$size)
    if (-not $script:probeNative::GetTokenInformation($Token,$Class,$buffer,$size,[ref]$size)) { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer);throw 'token_read_failed' }
    return $buffer
}
function Get-ProbeFacts([IntPtr]$Token) {
    $buffers=New-Object 'Collections.Generic.List[IntPtr]'
    try {
        $values=@{}
        foreach ($c in @(1,2,8,18,20,25)) { $p=Get-ProbeBuffer $Token $c;[void]$buffers.Add($p);$values[$c]=$p }
        $sid=(New-Object Security.Principal.SecurityIdentifier([Runtime.InteropServices.Marshal]::ReadIntPtr($values[1]))).Value
        $integrity=(New-Object Security.Principal.SecurityIdentifier([Runtime.InteropServices.Marshal]::ReadIntPtr($values[25]))).Value
        $count=[Runtime.InteropServices.Marshal]::ReadInt32($values[2]);if ($count -lt 0 -or $count -gt 1024) { throw 'token_read_failed' }
        $admin=$false
        for ($i=0;$i -lt $count;$i++) {
            $offset=8+$i*16;$group=(New-Object Security.Principal.SecurityIdentifier([Runtime.InteropServices.Marshal]::ReadIntPtr($values[2],$offset))).Value
            $attributes=[Runtime.InteropServices.Marshal]::ReadInt32($values[2],$offset+8)
            if ($group -ceq 'S-1-5-32-544' -and ($attributes -band 4) -ne 0 -and ($attributes -band 16) -eq 0) { $admin=$true }
        }
        return [pscustomobject]@{sid=$sid;integrity=$integrity;adminEnabled=$admin;elevated=([Runtime.InteropServices.Marshal]::ReadInt32($values[20])-ne 0);elevationType=[Runtime.InteropServices.Marshal]::ReadInt32($values[18]);tokenType=[Runtime.InteropServices.Marshal]::ReadInt32($values[8])}
    } finally { foreach ($p in $buffers) { [Runtime.InteropServices.Marshal]::FreeHGlobal($p) } }
}
function Get-ProbeImage([IntPtr]$Process) {
    $image=New-Object Text.StringBuilder(32768);[uint32]$length=32768
    if (-not $script:probeNative::QueryFullProcessImageNameW($Process,0,$image,[ref]$length)) { throw 'process_image_failed' }
    return $image.ToString()
}
function Assert-ProbeCodeSD([string]$Sddl) {
    $roles=@('S-1-5-18','S-1-5-32-544','S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464')
    $sd=New-Object Security.AccessControl.RawSecurityDescriptor($Sddl)
    if ($null -eq $sd.Owner -or $roles -notcontains $sd.Owner.Value -or $null -eq $sd.DiscretionaryAcl) { throw 'code_acl_rejected' }
    foreach ($ace in $sd.DiscretionaryAcl) {
        if ($ace -isnot [Security.AccessControl.CommonAce] -or $ace.IsCallback -or ($ace.AceQualifier -eq [Security.AccessControl.AceQualifier]::AccessAllowed -and ($ace.AccessMask -band 0x500d0156) -ne 0 -and $roles -notcontains $ace.SecurityIdentifier.Value)) { throw 'code_acl_rejected' }
    }
}
function Assert-ProbeCodePath([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path;Assert-ProbeCodeSD ($acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access))
}
function Lock-ProbeCode([string]$Path,[bool]$File) {
    if ($Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or $Path.Substring(2).Contains(':')) { throw 'code_path_rejected' }
    $root=$Path.Substring(0,3);$current=$root;$parts=@($root)
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if (-not $part -or $part -in @('.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'code_path_rejected' }
        $current=[IO.Path]::Combine($current,$part);$parts+=$current
    }
    $mapping=New-Object Text.StringBuilder(32768)
    if ($script:probeNative::QueryDosDeviceW($root.Substring(0,2),$mapping,32768) -eq 0 -or $mapping.ToString() -notmatch '^\\Device\\HarddiskVolume[0-9]+$' -or (New-Object IO.DriveInfo($root)).DriveType -ne [IO.DriveType]::Fixed) { throw 'code_path_rejected' }
    for ($i=0;$i -lt $parts.Count;$i++) {
        $leafFile=$File -and $i -eq $parts.Count-1;$flags=[uint32]0x02200000;$access=[uint32]0x20081
        if ($leafFile) { $flags=[uint32]0x00200000;$access=[uint32]2147614848 }
        $h=$script:probeNative::CreateFileW($parts[$i],$access,1,[IntPtr]::Zero,3,$flags,[IntPtr]::Zero)
        if ($h -eq [IntPtr](-1)) { throw 'code_open_failed' };[void]$script:probeHeld.Add($h)
        $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal(8)
        try { if (-not $script:probeNative::GetFileInformationByHandleEx($h,9,$buffer,8)) { throw 'code_check_failed' };$a=[Runtime.InteropServices.Marshal]::ReadInt32($buffer);if (($a -band 0x400) -ne 0 -or ((($a -band 0x10)-eq 0)-ne $leafFile)) { throw 'code_path_rejected' } }
        finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
    }
    # Existing OS ancestors are trusted/pinned. Only the explicit code root or
    # leaf gets the write-ACL check (volume roots may allow new sibling dirs).
    Assert-ProbeCodePath $Path
}
function Get-ProbeHash([string]$Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Get-ProbeService([string]$Image) {
    $services=@(Get-CimInstance Win32_Service -Filter "Name='godusevpn-ultra'")
    if ($services.Count -ne 1) { throw 'service_rejected' };Assert-ProbeService $services[0] $Image;return $services[0]
}
function Resolve-ProbeSID([string]$Account) {
    $local=[Environment]::MachineName;[uint32]$sidLength=0;[uint32]$domainLength=0;[uint32]$use=0
    $null=$script:probeNative::LookupAccountNameW($null,($local+'\'+$Account),[IntPtr]::Zero,[ref]$sidLength,$null,[ref]$domainLength,[ref]$use)
    if ($sidLength -lt 1 -or $sidLength -gt 1024 -or $domainLength -gt 1024) { throw 'account_lookup_failed' }
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal([int]$sidLength);$domain=New-Object Text.StringBuilder([int]($domainLength+1));$domainLength++
    try {
        if (-not $script:probeNative::LookupAccountNameW($null,($local+'\'+$Account),$buffer,[ref]$sidLength,$domain,[ref]$domainLength,[ref]$use) -or $use -ne 1 -or -not $domain.ToString().Equals($local,[StringComparison]::OrdinalIgnoreCase)) { throw 'account_lookup_failed' }
        return (New-Object Security.Principal.SecurityIdentifier($buffer)).Value
    } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
}

$result=[ordered]@{schema='ultra-medium-token-probe-v1';mode='token_query_probe';status='rejected';error='';systemCaller=$false;serviceBound=$false;installedHashesMatched=$false;realUserMatched=$false;medium=$false;elevated=$false;adminGroupEnabled=$false;linkedTokenUsed=$false;probe=$null;postServiceUnchanged=$false;contextUnknown=$false;productExecuted=$false;statusCommandExecuted=$false;cliAuthenticationPassed=$false}
$script:probeNative=$null;$script:probeHeld=New-Object 'Collections.Generic.List[IntPtr]';$exitCode=1;$probeInvoked=$false;$probeReturned=$false
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess) { throw 'host_rejected' }
    Assert-ProbeAccount $TestAccount;$script:probeNative=New-ProbeNative
    [IntPtr]$threadToken=[IntPtr]::Zero
    if ($script:probeNative::OpenThreadToken([IntPtr](-2),8,$true,[ref]$threadToken)) { [void]$script:probeHeld.Add($threadToken);throw 'caller_impersonating' }
    if ([Runtime.InteropServices.Marshal]::GetLastWin32Error() -ne 1008) { throw 'caller_context_unknown' }
    [IntPtr]$callerToken=[IntPtr]::Zero
    if (-not $script:probeNative::OpenProcessToken([IntPtr](-1),8,[ref]$callerToken)) { throw 'caller_context_unknown' };[void]$script:probeHeld.Add($callerToken)
    $caller=Get-ProbeFacts $callerToken;if ($caller.sid -cne 'S-1-5-18' -or -not $caller.elevated) { throw 'system_required' };$result.systemCaller=$true
    $installed=[IO.Path]::Combine([Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles),'GodUseVPN Ultra')
    $svc=[IO.Path]::Combine($installed,'godusevpn-svc.exe');$cli=[IO.Path]::Combine($installed,'godusevpn-cli.exe')
    $system=[Environment]::GetFolderPath([Environment+SpecialFolder]::System);$cmd=[IO.Path]::Combine($system,'cmd.exe')
    Lock-ProbeCode $installed $false;Lock-ProbeCode $svc $true;Lock-ProbeCode $cli $true;Lock-ProbeCode $system $false;Lock-ProbeCode $cmd $true
    if ((Get-ProbeHash $svc) -cne 'e9105cf5fe9f7e1167455a6654ff7ed3900f24308023663725ebd7f88729b14f' -or (Get-ProbeHash $cli) -cne '7eed52a1e1e6f4424db54f4a2b067346f2dc7bd9df734687529422294cd9673e') { throw 'installed_hash_rejected' };$result.installedHashesMatched=$true
    $service=Get-ProbeService $svc;$targetPID=[uint32]$service.ProcessId
    $target=$script:probeNative::OpenProcess(0x101000,$false,$targetPID);if ($target -eq [IntPtr]::Zero) { throw 'target_open_failed' };[void]$script:probeHeld.Add($target)
    if ($script:probeNative::WaitForSingleObject($target,0) -ne 258 -or -not [string]::Equals((Get-ProbeImage $target),$svc,[StringComparison]::OrdinalIgnoreCase)) { throw 'service_rejected' }
    [IntPtr]$targetToken=[IntPtr]::Zero
    if (-not $script:probeNative::OpenProcessToken($target,8,[ref]$targetToken)) { throw 'target_token_unreadable_as_system' };[void]$script:probeHeld.Add($targetToken)
    $targetFacts=Get-ProbeFacts $targetToken;if ($targetFacts.sid -cne 'S-1-5-18' -or -not $targetFacts.elevated) { throw 'target_token_rejected' };$result.serviceBound=$true
    $sid=Resolve-ProbeSID $TestAccount;$sources=@();$cmds=@(Get-CimInstance Win32_Process -Filter "Name='cmd.exe'")
    if ($cmds.Count -gt 64) { throw 'source_ambiguous' }
    foreach ($p in $cmds) {
        if (-not [string]::Equals([string]$p.ExecutablePath,$cmd,[StringComparison]::OrdinalIgnoreCase)) { continue }
        $owner=Invoke-CimMethod -InputObject $p -MethodName GetOwnerSid
        if ($owner.ReturnValue -ne 0) { throw 'source_owner_unknown' };if ($owner.Sid -ceq $sid) { $sources+=$p }
    }
    if ($sources.Count -ne 1) { throw 'source_ambiguous' }
    $source=$script:probeNative::OpenProcess(0x101000,$false,[uint32]$sources[0].ProcessId);if ($source -eq [IntPtr]::Zero) { throw 'source_open_failed' };[void]$script:probeHeld.Add($source)
    if ($script:probeNative::WaitForSingleObject($source,0) -ne 258 -or -not [string]::Equals((Get-ProbeImage $source),$cmd,[StringComparison]::OrdinalIgnoreCase)) { throw 'source_image_rejected' }
    [IntPtr]$sourceToken=[IntPtr]::Zero
    if (-not $script:probeNative::OpenProcessToken($source,10,[ref]$sourceToken)) { throw 'source_token_unreadable' };[void]$script:probeHeld.Add($sourceToken)
    $facts=Get-ProbeFacts $sourceToken;if ($facts.sid -cne $sid) { throw 'source_user_rejected' };$selected=$sourceToken
    if ($facts.elevationType -eq 2) {
        $buffer=Get-ProbeBuffer $sourceToken 19
        try { $selected=[Runtime.InteropServices.Marshal]::ReadIntPtr($buffer) } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
        if ($selected -eq [IntPtr]::Zero) { throw 'linked_token_unavailable' };[void]$script:probeHeld.Add($selected)
        $facts=Get-ProbeFacts $selected;if ($facts.elevationType -ne 3) { throw 'medium_token_rejected' };$result.linkedTokenUsed=$true
    } elseif ($facts.elevationType -notin @(1,3)) { throw 'medium_token_rejected' }
    Assert-ProbeMedium $facts $sid 1
    [IntPtr]$duplicate=[IntPtr]::Zero
    if (-not $script:probeNative::DuplicateTokenEx($selected,14,[IntPtr]::Zero,2,2,[ref]$duplicate)) { throw 'duplicate_failed' };[void]$script:probeHeld.Add($duplicate)
    Assert-ProbeMedium (Get-ProbeFacts $duplicate) $sid 2
    $result.realUserMatched=$true;$result.medium=$true
    $fresh=Get-ProbeService $svc;if ([uint32]$fresh.ProcessId -ne $targetPID -or $script:probeNative::WaitForSingleObject($target,0) -ne 258) { throw 'service_changed' }
    $probeInvoked=$true;$raw=$script:probeNative::Probe($duplicate,$targetPID);$probeReturned=$true
    $result.probe=Convert-ProbeResult $raw
    if ($result.probe.impersonated -and -not $result.probe.revertConfirmed) { $result.contextUnknown=$true;throw 'revert_failed' }
    if (-not $result.probe.impersonated -or $result.probe.nativeException) { throw 'probe_failed' }
    # No privilege-sensitive operation occurs after an unconfirmed Revert.
    [IntPtr]$afterToken=[IntPtr]::Zero
    if ($script:probeNative::OpenThreadToken([IntPtr](-2),8,$true,[ref]$afterToken)) { [void]$script:probeHeld.Add($afterToken);$result.contextUnknown=$true;throw 'revert_failed' }
    if ([Runtime.InteropServices.Marshal]::GetLastWin32Error() -ne 1008) { $result.contextUnknown=$true;throw 'revert_failed' }
    $fresh=Get-ProbeService $svc;if ([uint32]$fresh.ProcessId -ne $targetPID -or $script:probeNative::WaitForSingleObject($target,0) -ne 258 -or -not [string]::Equals((Get-ProbeImage $target),$svc,[StringComparison]::OrdinalIgnoreCase)) { throw 'service_changed' }
    $result.postServiceUnchanged=$true;$result.status='completed';$exitCode=0
} catch {
    if ($probeInvoked -and -not $probeReturned) { $result.contextUnknown=$true }
    $known=@('account_rejected','host_rejected','caller_impersonating','caller_context_unknown','system_required','code_path_rejected','code_acl_rejected','code_open_failed','code_check_failed','installed_hash_rejected','service_rejected','target_open_failed','target_token_unreadable_as_system','target_token_rejected','account_lookup_failed','source_ambiguous','source_owner_unknown','source_open_failed','source_image_rejected','source_token_unreadable','source_user_rejected','linked_token_unavailable','medium_token_rejected','duplicate_failed','service_changed','revert_failed','probe_failed','probe_result_rejected','token_read_failed','process_image_failed')
    $code=$_.Exception.Message;if ($known -cnotcontains $code) { $code='check_failed' };$result.error=$code
} finally {
    # Closing already-held handles is the only native cleanup after failure.
    if ($null -ne $script:probeNative) { for ($i=$script:probeHeld.Count-1;$i -ge 0;$i--) { [void]$script:probeNative::CloseHandle($script:probeHeld[$i]) } }
}
$json=$result | ConvertTo-Json -Compress -Depth 5
if ($json.Length -gt 2000) { $json='{"schema":"ultra-medium-token-probe-v1","status":"rejected","error":"summary_limit","productExecuted":false}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode
