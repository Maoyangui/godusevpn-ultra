# Windows PowerShell 5.1 x64 SYSTEM. Fixed 0.01-beta-qa.3 candidate.
# Runs exactly the installed CLI status under a real existing local user's
# verified medium primary token, only after separate review/approval.
# No other EXE/command, credential, registration, service or policy mutation.
# Separately reviewed qa.3 copy; requires separate execution approval.
[CmdletBinding()]
param([string]$TestAccount)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue';$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue';$DebugPreference='SilentlyContinue'

function Assert-StatusAccount([string]$Name) {
    if ($Name -cnotmatch '^[A-Za-z0-9][A-Za-z0-9_.-]{0,19}$' -or $Name.EndsWith('.')) { throw 'account_rejected' }
}

function Assert-StatusService($Value,[string]$Image) {
    if ($null -eq $Value -or $Value.Name -cne 'godusevpn-ultra' -or $Value.State -cne 'Running' -or $Value.StartName -cne 'LocalSystem' -or $Value.ServiceType -cne 'Own Process' -or $Value.DesktopInteract -isnot [bool] -or $Value.DesktopInteract -or [long]$Value.ProcessId -lt 1 -or [long]$Value.ProcessId -gt [uint32]::MaxValue) { throw 'service_rejected' }
    $command=[string]$Value.PathName
    if ($command.Length -lt 11 -or -not $command.StartsWith('"',[StringComparison]::Ordinal) -or -not $command.EndsWith('" service',[StringComparison]::Ordinal) -or -not [string]::Equals($command.Substring(1,$command.Length-10),$Image,[StringComparison]::OrdinalIgnoreCase)) { throw 'service_rejected' }
}

function Assert-StatusMedium($Facts,[string]$SID,[int]$Type) {
    if ($null -eq $Facts -or $Facts.sid -cne $SID -or $Facts.sid -ceq 'S-1-5-18' -or $Facts.integrity -cne 'S-1-16-8192' -or $Facts.elevated -isnot [bool] -or $Facts.elevated -or $Facts.adminEnabled -isnot [bool] -or $Facts.adminEnabled -or $Facts.tokenType -ne $Type) { throw 'medium_token_rejected' }
}

function New-StatusNative {
    $assembly=New-Object Reflection.AssemblyName('UltraQa3MediumStatus')
    $builder=[AppDomain]::CurrentDomain.DefineDynamicAssembly($assembly,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $type=$builder.DefineDynamicModule('Native').DefineType('UltraQa3MediumStatusNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
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
        @('kernel32.dll','ResumeThread',[uint32],[type[]]@([IntPtr])),
        @('kernel32.dll','TerminateProcess',[bool],[type[]]@([IntPtr],[uint32])),
        @('kernel32.dll','GetExitCodeProcess',[bool],[type[]]@([IntPtr],$uintRef)),
        @('userenv.dll','CreateEnvironmentBlock',[bool],[type[]]@($intptrRef,[IntPtr],[bool])),
        @('userenv.dll','DestroyEnvironmentBlock',[bool],[type[]]@([IntPtr])),
        @('advapi32.dll','CreateProcessAsUserW',[bool],[type[]]@([IntPtr],[string],[Text.StringBuilder],[IntPtr],[IntPtr],[bool],[uint32],[IntPtr],[string],[IntPtr],[IntPtr])),
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
    return $type.CreateType()
}

function Get-StatusBuffer([IntPtr]$Token,[int]$Class) {
    [uint32]$size=0
    $null=$script:statusNative::GetTokenInformation($Token,$Class,[IntPtr]::Zero,0,[ref]$size)
    if ($size -lt 1 -or $size -gt 1048576) { throw 'token_read_failed' }
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal([int]$size)
    if (-not $script:statusNative::GetTokenInformation($Token,$Class,$buffer,$size,[ref]$size)) { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer);throw 'token_read_failed' }
    return $buffer
}

function Get-StatusFacts([IntPtr]$Token) {
    $buffers=New-Object 'Collections.Generic.List[IntPtr]'
    try {
        $values=@{}
        foreach ($c in @(1,2,8,12,18,20,25)) { $p=Get-StatusBuffer $Token $c;[void]$buffers.Add($p);$values[$c]=$p }
        $sid=(New-Object Security.Principal.SecurityIdentifier([Runtime.InteropServices.Marshal]::ReadIntPtr($values[1]))).Value
        $integrity=(New-Object Security.Principal.SecurityIdentifier([Runtime.InteropServices.Marshal]::ReadIntPtr($values[25]))).Value
        $count=[Runtime.InteropServices.Marshal]::ReadInt32($values[2]);if ($count -lt 0 -or $count -gt 1024) { throw 'token_read_failed' }
        $admin=$false
        for ($i=0;$i -lt $count;$i++) {
            $offset=8+$i*16;$group=(New-Object Security.Principal.SecurityIdentifier([Runtime.InteropServices.Marshal]::ReadIntPtr($values[2],$offset))).Value
            $attributes=[Runtime.InteropServices.Marshal]::ReadInt32($values[2],$offset+8)
            if ($group -ceq 'S-1-5-32-544' -and ($attributes -band 4) -ne 0 -and ($attributes -band 16) -eq 0) { $admin=$true }
        }
        return [pscustomobject]@{sid=$sid;integrity=$integrity;adminEnabled=$admin;elevated=([Runtime.InteropServices.Marshal]::ReadInt32($values[20])-ne 0);elevationType=[Runtime.InteropServices.Marshal]::ReadInt32($values[18]);tokenType=[Runtime.InteropServices.Marshal]::ReadInt32($values[8]);session=[Runtime.InteropServices.Marshal]::ReadInt32($values[12])}
    } finally { foreach ($p in $buffers) { [Runtime.InteropServices.Marshal]::FreeHGlobal($p) } }
}

function Get-StatusImage([IntPtr]$Process) {
    $image=New-Object Text.StringBuilder(32768);[uint32]$length=32768
    if (-not $script:statusNative::QueryFullProcessImageNameW($Process,0,$image,[ref]$length)) { throw 'process_image_failed' }
    return $image.ToString()
}

function Assert-StatusCodeSD([string]$Sddl) {
    $roles=@('S-1-5-18','S-1-5-32-544','S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464')
    $sd=New-Object Security.AccessControl.RawSecurityDescriptor($Sddl)
    if ($null -eq $sd.Owner -or $roles -notcontains $sd.Owner.Value -or $null -eq $sd.DiscretionaryAcl) { throw 'code_acl_rejected' }
    foreach ($ace in $sd.DiscretionaryAcl) {
        if ($ace -isnot [Security.AccessControl.CommonAce] -or $ace.IsCallback -or ($ace.AceQualifier -eq [Security.AccessControl.AceQualifier]::AccessAllowed -and ($ace.AccessMask -band 0x500d0156) -ne 0 -and $roles -notcontains $ace.SecurityIdentifier.Value)) { throw 'code_acl_rejected' }
    }
}

function Assert-StatusCodePath([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path;Assert-StatusCodeSD ($acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access))
}

function Lock-StatusCode([string]$Path,[bool]$File) {
    if ($Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or $Path.Substring(2).Contains(':')) { throw 'code_path_rejected' }
    $root=$Path.Substring(0,3);$current=$root;$parts=@($root)
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if (-not $part -or $part -in @('.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'code_path_rejected' }
        $current=[IO.Path]::Combine($current,$part);$parts+=$current
    }
    $mapping=New-Object Text.StringBuilder(32768)
    if ($script:statusNative::QueryDosDeviceW($root.Substring(0,2),$mapping,32768) -eq 0 -or $mapping.ToString() -notmatch '^\\Device\\HarddiskVolume[0-9]+$' -or (New-Object IO.DriveInfo($root)).DriveType -ne [IO.DriveType]::Fixed) { throw 'code_path_rejected' }
    for ($i=0;$i -lt $parts.Count;$i++) {
        $leafFile=$File -and $i -eq $parts.Count-1;$flags=[uint32]0x02200000;$access=[uint32]0x20081
        if ($leafFile) { $flags=[uint32]0x00200000;$access=[uint32]2147614848 }
        $h=$script:statusNative::CreateFileW($parts[$i],$access,1,[IntPtr]::Zero,3,$flags,[IntPtr]::Zero)
        if ($h -eq [IntPtr](-1)) { throw 'code_open_failed' };[void]$script:statusHeld.Add($h)
        $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal(8)
        try { if (-not $script:statusNative::GetFileInformationByHandleEx($h,9,$buffer,8)) { throw 'code_check_failed' };$a=[Runtime.InteropServices.Marshal]::ReadInt32($buffer);if (($a -band 0x400) -ne 0 -or ((($a -band 0x10)-eq 0)-ne $leafFile)) { throw 'code_path_rejected' } }
        finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
    }
    # Existing OS ancestors are trusted/pinned. Only the explicit code root or
    # leaf gets the write-ACL check (volume roots may allow new sibling dirs).
    Assert-StatusCodePath $Path
}

function Get-StatusHash([string]$Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Get-StatusService([string]$Image) {
    $services=@(Get-CimInstance Win32_Service -Filter "Name='godusevpn-ultra'" -Property Name,State,StartName,ServiceType,DesktopInteract,ProcessId,PathName)
    if ($services.Count -ne 1) { throw 'service_rejected' };Assert-StatusService $services[0] $Image;return $services[0]
}

function Resolve-StatusSID([string]$Account) {
    $local=[Environment]::MachineName;[uint32]$sidLength=0;[uint32]$domainLength=0;[uint32]$use=0
    $null=$script:statusNative::LookupAccountNameW($null,($local+'\'+$Account),[IntPtr]::Zero,[ref]$sidLength,$null,[ref]$domainLength,[ref]$use)
    if ($sidLength -lt 1 -or $sidLength -gt 1024 -or $domainLength -gt 1024) { throw 'account_lookup_failed' }
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal([int]$sidLength);$domain=New-Object Text.StringBuilder([int]($domainLength+1));$domainLength++
    try {
        if (-not $script:statusNative::LookupAccountNameW($null,($local+'\'+$Account),$buffer,[ref]$sidLength,$domain,[ref]$domainLength,[ref]$use) -or $use -ne 1 -or -not $domain.ToString().Equals($local,[StringComparison]::OrdinalIgnoreCase)) { throw 'account_lookup_failed' }
        return (New-Object Security.Principal.SecurityIdentifier($buffer)).Value
    } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
}

function New-StatusProcessBuffers {
    # Fixed x64 STARTUPINFOW (104) and PROCESS_INFORMATION (24). No desktop,
    # std handles or STARTF_USESTDHANDLES; CreateProcess does not inherit handles.
    $startup=[Runtime.InteropServices.Marshal]::AllocHGlobal(104)
    $process=[IntPtr]::Zero
    try {
        $process=[Runtime.InteropServices.Marshal]::AllocHGlobal(24)
        [Runtime.InteropServices.Marshal]::Copy((New-Object byte[] 104),0,$startup,104)
        [Runtime.InteropServices.Marshal]::Copy((New-Object byte[] 24),0,$process,24)
        [Runtime.InteropServices.Marshal]::WriteInt32($startup,104)
        return [pscustomobject]@{startup=$startup;process=$process}
    } catch {
        if ($process -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::FreeHGlobal($process) }
        [Runtime.InteropServices.Marshal]::FreeHGlobal($startup);throw
    }
}
function New-StatusChild([IntPtr]$Token,[string]$CLI,[string]$Directory,[IntPtr]$Environment) {
    $buffers=New-StatusProcessBuffers
    try {
        $command=New-Object Text.StringBuilder(('"'+$CLI+'" status'))
        if (-not $script:statusNative::CreateProcessAsUserW($Token,$CLI,$command,[IntPtr]::Zero,[IntPtr]::Zero,$false,0x08000404,$Environment,$Directory,$buffers.startup,$buffers.process)) { throw 'child_create_failed' }
        return [pscustomobject]@{process=[Runtime.InteropServices.Marshal]::ReadIntPtr($buffers.process,0);thread=[Runtime.InteropServices.Marshal]::ReadIntPtr($buffers.process,8)}
    } finally {
        [Runtime.InteropServices.Marshal]::FreeHGlobal($buffers.process)
        [Runtime.InteropServices.Marshal]::FreeHGlobal($buffers.startup)
    }
}
function Assert-StatusChildFacts($Facts,[string]$SID,[int]$Session,[string]$Image,[string]$ExpectedImage) {
    Assert-StatusMedium $Facts $SID 1
    if ($Session -lt 0 -or $Facts.session -ne $Session -or -not [string]::Equals($Image,$ExpectedImage,[StringComparison]::OrdinalIgnoreCase)) { throw 'child_identity_rejected' }
}
function Invoke-StatusLifecycle([hashtable]$Operations,[Collections.IDictionary]$Result) {
    # One owned process handle; no PID-based kill, retry, status output, service
    # mutation or impersonation. The same state machine is tested with fakes.
    $child=$null;$ended=$false
    try {
        $child=&$Operations.Create
        if ($null -eq $child -or $child.process -eq [IntPtr]::Zero -or $child.thread -eq [IntPtr]::Zero) { throw 'child_create_failed' }
        $Result.childCreated=$true
        &$Operations.Validate $child
        $Result.childValidated=$true;$Result.resumeAttempted=$true
        # Until a final exit code is available, an attempted resume has an
        # unknown result even if the API/exit-code read itself later throws.
        $Result.resultUnknown=$true
        $previous=&$Operations.Resume $child.thread
        if ($previous -ne 1) { $Result.resultUnknown=$true;throw 'child_resume_unknown' }
        $Result.productExecuted=$true;$Result.statusCommandExecuted=$true
        $wait=&$Operations.Wait $child.process 15000
        if ($wait -ne 0) { $Result.resultUnknown=$true;throw 'child_wait_unknown' }
        $ended=$true;$Result.childExited=$true
        $code=&$Operations.Exit $child.process
        $Result.resultUnknown=$false
        if ($code -ne 0) { throw 'status_failed' }
        $Result.statusExitZero=$true
    } finally {
        if ($null -ne $child) {
            if (-not $ended -and $child.process -ne [IntPtr]::Zero) {
                $Result.terminationAttempted=$true
                try { $terminated=&$Operations.Terminate $child.process;$Result.terminationConfirmed=($terminated -and ((&$Operations.Wait $child.process 1000)-eq 0)) } catch { $Result.terminationConfirmed=$false }
                if (-not $Result.terminationConfirmed) { $Result.resultUnknown=$true }
            }
            # Close both owned handles even if the first close fails.
            try { if ($child.thread -ne [IntPtr]::Zero) { &$Operations.Close $child.thread } }
            finally { if ($child.process -ne [IntPtr]::Zero) { &$Operations.Close $child.process } }
        }
    }
}

$result=[ordered]@{schema='ultra-medium-status-v1';candidate='0.01-beta-qa.3';status='rejected';error='';systemCaller=$false;serviceBound=$false;installedHashesMatched=$false;realUserMatched=$false;medium=$false;linkedTokenUsed=$false;childCreated=$false;childValidated=$false;resumeAttempted=$false;childExited=$false;statusExitZero=$false;terminationAttempted=$false;terminationConfirmed=$false;postServiceUnchanged=$false;resultUnknown=$false;productExecuted=$false;statusCommandExecuted=$false;cliAuthenticationPassed=$false}
$script:statusNative=$null;$script:statusHeld=New-Object 'Collections.Generic.List[IntPtr]';$environment=[IntPtr]::Zero;$exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess) { throw 'host_rejected' }
    Assert-StatusAccount $TestAccount;$script:statusNative=New-StatusNative
    [IntPtr]$threadToken=[IntPtr]::Zero
    if ($script:statusNative::OpenThreadToken([IntPtr](-2),8,$true,[ref]$threadToken)) { [void]$script:statusHeld.Add($threadToken);throw 'caller_impersonating' }
    if ([Runtime.InteropServices.Marshal]::GetLastWin32Error() -ne 1008) { throw 'caller_context_unknown' }
    [IntPtr]$callerToken=[IntPtr]::Zero
    if (-not $script:statusNative::OpenProcessToken([IntPtr](-1),8,[ref]$callerToken)) { throw 'caller_context_unknown' };[void]$script:statusHeld.Add($callerToken)
    $caller=Get-StatusFacts $callerToken;if ($caller.sid -cne 'S-1-5-18' -or -not $caller.elevated) { throw 'system_required' };$result.systemCaller=$true
    $installed=[IO.Path]::Combine([Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles),'GodUseVPN Ultra')
    $svc=[IO.Path]::Combine($installed,'godusevpn-svc.exe');$cli=[IO.Path]::Combine($installed,'godusevpn-cli.exe')
    $system=[Environment]::GetFolderPath([Environment+SpecialFolder]::System);$cmd=[IO.Path]::Combine($system,'cmd.exe')
    Lock-StatusCode $installed $false;Lock-StatusCode $svc $true;Lock-StatusCode $cli $true;Lock-StatusCode $system $false;Lock-StatusCode $cmd $true
    if ((Get-StatusHash $svc) -cne '2ebd104afd943353f18662510e751acf9421e03fa7cca834d516444b98f5127e' -or (Get-StatusHash $cli) -cne '3a17693ee9e80c865b9bee92ee34f403adeb75883a797b821e6123e9edabfae8') { throw 'installed_hash_rejected' };$result.installedHashesMatched=$true
    $service=Get-StatusService $svc;$targetPID=[uint32]$service.ProcessId
    $target=$script:statusNative::OpenProcess(0x101000,$false,$targetPID);if ($target -eq [IntPtr]::Zero) { throw 'target_open_failed' };[void]$script:statusHeld.Add($target)
    if ($script:statusNative::WaitForSingleObject($target,0) -ne 258 -or -not [string]::Equals((Get-StatusImage $target),$svc,[StringComparison]::OrdinalIgnoreCase)) { throw 'service_rejected' }
    [IntPtr]$targetToken=[IntPtr]::Zero
    if (-not $script:statusNative::OpenProcessToken($target,8,[ref]$targetToken)) { throw 'target_token_unreadable_as_system' };[void]$script:statusHeld.Add($targetToken)
    $targetFacts=Get-StatusFacts $targetToken;if ($targetFacts.sid -cne 'S-1-5-18' -or -not $targetFacts.elevated) { throw 'target_token_rejected' };$result.serviceBound=$true
    $sid=Resolve-StatusSID $TestAccount;$sources=@()
    # Property subset deliberately excludes CommandLine and other process data.
    $cmds=@(Get-CimInstance Win32_Process -Filter "Name='cmd.exe'" -Property ProcessId,ExecutablePath,Handle)
    if ($cmds.Count -gt 64) { throw 'source_ambiguous' }
    foreach ($p in $cmds) {
        if (-not [string]::Equals([string]$p.ExecutablePath,$cmd,[StringComparison]::OrdinalIgnoreCase)) { continue }
        $owner=Invoke-CimMethod -InputObject $p -MethodName GetOwnerSid
        if ($owner.ReturnValue -ne 0) { throw 'source_owner_unknown' };if ($owner.Sid -ceq $sid) { $sources+=$p }
    }
    if ($sources.Count -ne 1) { throw 'source_ambiguous' }
    $source=$script:statusNative::OpenProcess(0x101000,$false,[uint32]$sources[0].ProcessId);if ($source -eq [IntPtr]::Zero) { throw 'source_open_failed' };[void]$script:statusHeld.Add($source)
    if ($script:statusNative::WaitForSingleObject($source,0) -ne 258 -or -not [string]::Equals((Get-StatusImage $source),$cmd,[StringComparison]::OrdinalIgnoreCase)) { throw 'source_image_rejected' }
    [IntPtr]$sourceToken=[IntPtr]::Zero
    if (-not $script:statusNative::OpenProcessToken($source,10,[ref]$sourceToken)) { throw 'source_token_unreadable' };[void]$script:statusHeld.Add($sourceToken)
    $sourceFacts=Get-StatusFacts $sourceToken;if ($sourceFacts.sid -cne $sid -or $sourceFacts.session -lt 0) { throw 'source_user_rejected' };$selected=$sourceToken;$facts=$sourceFacts
    if ($facts.elevationType -eq 2) {
        $buffer=Get-StatusBuffer $sourceToken 19
        try { $selected=[Runtime.InteropServices.Marshal]::ReadIntPtr($buffer) } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
        if ($selected -eq [IntPtr]::Zero) { throw 'linked_token_unavailable' };[void]$script:statusHeld.Add($selected)
        $facts=Get-StatusFacts $selected;if ($facts.elevationType -ne 3) { throw 'medium_token_rejected' };$result.linkedTokenUsed=$true
    } elseif ($facts.elevationType -notin @(1,3)) { throw 'medium_token_rejected' }
    Assert-StatusMedium $facts $sid 1
    if ($facts.session -ne $sourceFacts.session) { throw 'medium_token_rejected' }
    [IntPtr]$duplicate=[IntPtr]::Zero
    if (-not $script:statusNative::DuplicateTokenEx($selected,11,[IntPtr]::Zero,2,1,[ref]$duplicate)) { throw 'duplicate_failed' };[void]$script:statusHeld.Add($duplicate)
    $duplicateFacts=Get-StatusFacts $duplicate;Assert-StatusMedium $duplicateFacts $sid 1
    if ($duplicateFacts.session -ne $sourceFacts.session) { throw 'medium_token_rejected' }
    $result.realUserMatched=$true;$result.medium=$true
    if (-not $script:statusNative::CreateEnvironmentBlock([ref]$environment,$duplicate,$false)) { throw 'environment_failed' }
    $fresh=Get-StatusService $svc
    if ([uint32]$fresh.ProcessId -ne $targetPID -or $script:statusNative::WaitForSingleObject($target,0) -ne 258 -or $script:statusNative::WaitForSingleObject($source,0) -ne 258) { throw 'service_changed' }
    $operations=@{
        Create={ New-StatusChild $duplicate $cli $installed $environment }
        Validate={ param($child)
            [IntPtr]$childToken=[IntPtr]::Zero
            try {
                if (-not $script:statusNative::OpenProcessToken($child.process,8,[ref]$childToken)) { throw 'child_token_unreadable' }
                Assert-StatusChildFacts (Get-StatusFacts $childToken) $sid $sourceFacts.session (Get-StatusImage $child.process) $cli
                $freshChild=Get-StatusService $svc
                if ([uint32]$freshChild.ProcessId -ne $targetPID -or $script:statusNative::WaitForSingleObject($target,0) -ne 258 -or $script:statusNative::WaitForSingleObject($source,0) -ne 258) { throw 'service_changed' }
            } finally { if ($childToken -ne [IntPtr]::Zero) { [void]$script:statusNative::CloseHandle($childToken) } }
        }
        Resume={ param($handle) $script:statusNative::ResumeThread($handle) }
        Wait={ param($handle,$milliseconds) $script:statusNative::WaitForSingleObject($handle,$milliseconds) }
        Exit={ param($handle) [uint32]$value=0;if (-not $script:statusNative::GetExitCodeProcess($handle,[ref]$value)) { throw 'child_exit_unknown' };$value }
        Terminate={ param($handle) $script:statusNative::TerminateProcess($handle,1) }
        Close={ param($handle) if (-not $script:statusNative::CloseHandle($handle)) { throw 'child_close_failed' } }
    }
    Invoke-StatusLifecycle $operations $result
    $fresh=Get-StatusService $svc
    if ([uint32]$fresh.ProcessId -ne $targetPID -or $script:statusNative::WaitForSingleObject($target,0) -ne 258 -or -not [string]::Equals((Get-StatusImage $target),$svc,[StringComparison]::OrdinalIgnoreCase)) { $result.resultUnknown=$true;throw 'service_changed' }
    $result.postServiceUnchanged=$true;$result.cliAuthenticationPassed=$true;$result.status='completed';$exitCode=0
} catch {
    $known=@('account_rejected','host_rejected','caller_impersonating','caller_context_unknown','system_required','code_path_rejected','code_acl_rejected','code_open_failed','code_check_failed','installed_hash_rejected','service_rejected','target_open_failed','target_token_unreadable_as_system','target_token_rejected','account_lookup_failed','source_ambiguous','source_owner_unknown','source_open_failed','source_image_rejected','source_token_unreadable','source_user_rejected','linked_token_unavailable','medium_token_rejected','duplicate_failed','environment_failed','service_changed','token_read_failed','process_image_failed','child_create_failed','child_identity_rejected','child_token_unreadable','child_resume_unknown','child_wait_unknown','child_exit_unknown','child_close_failed','status_failed')
    $code=$_.Exception.Message;if ($known -cnotcontains $code) { $code='check_failed' };$result.error=$code
} finally {
    if ($null -ne $script:statusNative) {
        if ($environment -ne [IntPtr]::Zero) { [void]$script:statusNative::DestroyEnvironmentBlock($environment) }
        for ($i=$script:statusHeld.Count-1;$i -ge 0;$i--) { [void]$script:statusNative::CloseHandle($script:statusHeld[$i]) }
    }
}
$json=$result | ConvertTo-Json -Compress -Depth 4
if ($json.Length -gt 2000) { $json='{"schema":"ultra-medium-status-v1","status":"rejected","error":"summary_limit","resultUnknown":true}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode


