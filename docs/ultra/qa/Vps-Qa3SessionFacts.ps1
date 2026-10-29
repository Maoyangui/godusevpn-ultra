# Windows PowerShell 5.1 x64 SYSTEM. Read-only session feasibility facts ONLY.
# Requires separate source review before any execution. Never runs a product,
# launcher or child, duplicates/impersonates a token, enables privileges, reads
# credentials/environment/profile paths, changes a DACL or writes settings.
# Only local System32 cmd candidates are considered. Multiple/unknown matches
# are not guessed. Output omits account, SID, PID, session ID and actual paths.
# EnableLUA/profile flags are limited provider facts, not UAC or desktop tests;
# an unset temporary-profile flag does not disprove a TEMP desktop warning.
[CmdletBinding()]
param([string]$TestAccount)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue';$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue';$DebugPreference='SilentlyContinue'

function New-SessionNative {
    $assembly=New-Object Reflection.AssemblyName('UltraQa3SessionFacts')
    $builder=[AppDomain]::CurrentDomain.DefineDynamicAssembly($assembly,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $type=$builder.DefineDynamicModule('Native').DefineType('UltraQa3SessionFactsNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $intptrRef=[IntPtr].MakeByRefType();$uintRef=[uint32].MakeByRefType()
    $specs=@(
        @('kernel32.dll','CloseHandle',[bool],[type[]]@([IntPtr])),
        @('kernel32.dll','OpenProcess',[IntPtr],[type[]]@([uint32],[bool],[uint32])),
        @('kernel32.dll','WaitForSingleObject',[uint32],[type[]]@([IntPtr],[uint32])),
        @('kernel32.dll','QueryFullProcessImageNameW',[bool],[type[]]@([IntPtr],[uint32],[Text.StringBuilder],$uintRef)),
        @('advapi32.dll','OpenProcessToken',[bool],[type[]]@([IntPtr],[uint32],$intptrRef)),
        @('advapi32.dll','OpenThreadToken',[bool],[type[]]@([IntPtr],[uint32],[bool],$intptrRef)),
        @('advapi32.dll','GetTokenInformation',[bool],[type[]]@([IntPtr],[int],[IntPtr],[uint32],$uintRef)),
        @('advapi32.dll','LookupAccountNameW',[bool],[type[]]@([string],[string],[IntPtr],$uintRef,[Text.StringBuilder],$uintRef,$uintRef))
    )
    $ctor=[Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields=[Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'),[Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'),[Runtime.InteropServices.DllImportAttribute].GetField('CharSet'),[Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    foreach ($s in $specs) {
        $method=$type.DefineMethod($s[1],[Reflection.MethodAttributes]'Public, Static, PinvokeImpl',$s[2],$s[3]);$method.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $method.SetCustomAttribute((New-Object Reflection.Emit.CustomAttributeBuilder($ctor,[object[]]@($s[0]),$fields,[object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi))))
    }
    return $type.CreateType()
}

function Assert-StatusAccount([string]$Name) {
    if ($Name -cnotmatch '^[A-Za-z0-9][A-Za-z0-9_.-]{0,19}$' -or $Name.EndsWith('.')) { throw 'account_rejected' }
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

function Convert-SessionToken($Facts,[string]$SID) {
    $r=[ordered]@{exactUserMatched='unknown';elevated='unknown';integrity='unknown';elevationType='unknown';tokenType='unknown'}
    if ($null -eq $Facts) { return [pscustomobject]$r }
    $r.exactUserMatched=($Facts.sid -ceq $SID)
    if ($Facts.elevated -is [bool]) { $r.elevated=$Facts.elevated }
    switch -CaseSensitive ($Facts.integrity) {
        'S-1-16-0' {$r.integrity='untrusted'} 'S-1-16-4096' {$r.integrity='low'}
        'S-1-16-8192' {$r.integrity='medium'} 'S-1-16-8448' {$r.integrity='medium_plus'}
        'S-1-16-12288' {$r.integrity='high'} 'S-1-16-16384' {$r.integrity='system'}
        'S-1-16-20480' {$r.integrity='protected'}
    }
    if ($Facts.elevationType -is [int]) { switch ($Facts.elevationType) { 1 {$r.elevationType='default'} 2 {$r.elevationType='full'} 3 {$r.elevationType='limited'} } }
    if ($Facts.tokenType -is [int]) { switch ($Facts.tokenType) { 1 {$r.tokenType='primary'} 2 {$r.tokenType='impersonation'} } }
    return [pscustomobject]$r
}
function Test-SessionLinkedMedium($Facts,$Source,[string]$SID) {
    return ($null -ne $Facts -and $null -ne $Source -and $Source.sid -ceq $SID -and $Source.tokenType -eq 1 -and $Source.elevationType -eq 2 -and $Source.elevated -is [bool] -and $Source.elevated -and $Facts.sid -ceq $SID -and $Facts.sid -cne 'S-1-5-18' -and $Facts.integrity -ceq 'S-1-16-8192' -and $Facts.elevated -is [bool] -and -not $Facts.elevated -and $Facts.adminEnabled -is [bool] -and -not $Facts.adminEnabled -and $Facts.elevationType -eq 3 -and $Facts.tokenType -eq 1 -and $Facts.session -ge 0 -and $Facts.session -eq $Source.session)
}
function Convert-SessionLUA($Kind,$Value) {
    if ($Kind -cne 'DWord' -or ($Value -isnot [int] -and $Value -isnot [uint32])) { return 'unknown' }
    if ($Value -eq 1) { return 'enabled' };if ($Value -eq 0) { return 'disabled' };return 'unknown'
}
function Read-SessionLUA {
    $hive=$null;$key=$null
    try {
        $hive=[Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine,[Microsoft.Win32.RegistryView]::Registry64)
        $key=$hive.OpenSubKey('SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System',$false)
        if ($null -eq $key -or $key.GetValueNames() -cnotcontains 'EnableLUA') { return 'unknown' }
        return Convert-SessionLUA ($key.GetValueKind('EnableLUA').ToString()) ($key.GetValue('EnableLUA',$null,[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames))
    } catch { return 'unknown' }
    finally { if ($null -ne $key) { $key.Dispose() };if ($null -ne $hive) { $hive.Dispose() } }
}
function Convert-SessionProfile([object[]]$Rows,[string]$SID) {
    $r=[ordered]@{status='unknown';present='unknown';exactUserMatched='unknown';loaded='unknown';temporaryFlag='unknown'}
    if ($Rows.Count -eq 0) { $r.status='absent';$r.present=$false;return [pscustomobject]$r }
    if ($Rows.Count -ne 1) { $r.status='ambiguous';return [pscustomobject]$r }
    $p=$Rows[0]
    if ($p.SID -cne $SID) { return [pscustomobject]$r }
    $r.status='observed';$r.present=$true;$r.exactUserMatched=$true
    if ($p.Loaded -is [bool]) { $r.loaded=$p.Loaded }
    if (($p.Status -is [int] -or $p.Status -is [uint32]) -and $p.Status -ge 0 -and $p.Status -le 15) { $r.temporaryFlag=(($p.Status -band 1)-ne 0) }
    return [pscustomobject]$r
}
function Read-SessionProfile([string]$SID) {
    try {
        $rows=@(Get-CimInstance Win32_UserProfile -Filter ("SID='"+$SID+"'") -Property SID,Loaded,Status -OperationTimeoutSec 5 | Select-Object -First 2)
        return Convert-SessionProfile $rows $SID
    } catch { return [pscustomobject]@{status='unknown';present='unknown';exactUserMatched='unknown';loaded='unknown';temporaryFlag='unknown'} }
}
function Select-SessionSource([object[]]$Rows,[string]$Cmd,[string]$SID,[scriptblock]$ReadOwner) {
    $counts=[ordered]@{enumerated=$Rows.Count;system32Cmd=0;ownerMatched=0;ownerUnknown=0;unclassified=0;limitReached=($Rows.Count -ge 64)}
    $chosen=$null;$state='no_source'
    if ($counts.limitReached) { return [pscustomobject]@{status='ambiguous';counts=[pscustomobject]$counts;selected=$null} }
    foreach ($p in $Rows) {
        if (-not $p.ExecutablePath) { $counts.unclassified++;continue }
        if (-not [string]::Equals([string]$p.ExecutablePath,$Cmd,[StringComparison]::OrdinalIgnoreCase)) { continue }
        $counts.system32Cmd++
        try {
            if (($p.ProcessId -isnot [int] -and $p.ProcessId -isnot [uint32]) -or [long]$p.ProcessId -lt 1 -or [long]$p.ProcessId -gt [uint32]::MaxValue -or [string]$p.Handle -cnotmatch '^[1-9][0-9]{0,9}$' -or [uint64]$p.Handle -ne [uint64]$p.ProcessId) { throw 'source_row_unknown' }
            $owner=&$ReadOwner $p
            if ($null -eq $owner -or ($owner.ReturnValue -isnot [int] -and $owner.ReturnValue -isnot [uint32]) -or $owner.ReturnValue -ne 0 -or -not $owner.Sid -or $owner.Sid -cnotmatch '^S-[0-9]+(?:-[0-9]+)+$') { throw 'source_owner_unknown' }
            if ($owner.Sid -ceq $SID) { $counts.ownerMatched++;$chosen=$p }
        } catch { $counts.ownerUnknown++ }
    }
    if ($counts.ownerMatched -gt 1) { $state='ambiguous';$chosen=$null }
    elseif ($counts.ownerUnknown -gt 0 -or $counts.unclassified -gt 0) { $state='unknown';$chosen=$null }
    elseif ($counts.ownerMatched -eq 1) { $state='unique' }
    return [pscustomobject]@{status=$state;counts=[pscustomobject]$counts;selected=$chosen}
}

$result=[ordered]@{schema='ultra-qa3-session-facts-v1';status='unknown';error='';systemCaller=$false;accountResolved=$false;sourceCounts=$null;exactUserMatched='unknown';token=(Convert-SessionToken $null '');linkedInspection='not_inspected';linkedMediumAvailable='unknown';linkedToken=(Convert-SessionToken $null '');enableLUA='unknown';profile=[pscustomobject]@{status='unknown';present='unknown';exactUserMatched='unknown';loaded='unknown';temporaryFlag='unknown'};observationOnly=$true;productExecuted=$false;mediumTestExecuted=$false}
$script:statusNative=$null;$held=New-Object 'Collections.Generic.List[IntPtr]';$exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'host_rejected' }
    Assert-StatusAccount $TestAccount;$script:statusNative=New-SessionNative
    [IntPtr]$threadToken=[IntPtr]::Zero
    if ($script:statusNative::OpenThreadToken([IntPtr](-2),8,$true,[ref]$threadToken)) { [void]$held.Add($threadToken);throw 'caller_impersonating' }
    if ([Runtime.InteropServices.Marshal]::GetLastWin32Error() -ne 1008) { throw 'caller_context_unknown' }
    [IntPtr]$callerToken=[IntPtr]::Zero
    if (-not $script:statusNative::OpenProcessToken([IntPtr](-1),8,[ref]$callerToken)) { throw 'caller_context_unknown' };[void]$held.Add($callerToken)
    $caller=Get-StatusFacts $callerToken
    if ($caller.sid -cne 'S-1-5-18' -or -not $caller.elevated -or $caller.tokenType -ne 1) { throw 'system_required' };$result.systemCaller=$true
    $sid=Resolve-StatusSID $TestAccount
    if ($sid -cnotmatch '^S-1-5-21-(?:[0-9]+-){3}[0-9]+$') { throw 'account_lookup_failed' };$result.accountResolved=$true
    $result.enableLUA=Read-SessionLUA;$result.profile=Read-SessionProfile $sid
    $system=[Environment]::GetFolderPath([Environment+SpecialFolder]::System)
    if ($system -notmatch '^[A-Za-z]:\\' -or [IO.Path]::GetFileName($system) -ine 'System32') { throw 'system_path_unknown' }
    $cmd=[IO.Path]::Combine($system,'cmd.exe')
    # Bounded subset only. A full cap is conservatively ambiguous, even if the
    # provider might have exactly 64 rows; no token is selected in that case.
    $rows=@(Get-CimInstance Win32_Process -Filter "Name='cmd.exe'" -Property ProcessId,ExecutablePath,Handle -OperationTimeoutSec 5 | Select-Object -First 64)
    $selection=Select-SessionSource $rows $cmd $sid {param($p) Invoke-CimMethod -InputObject $p -MethodName GetOwnerSid -OperationTimeoutSec 5}
    $result.sourceCounts=$selection.counts;$result.status=$selection.status
    if ($selection.status -ceq 'unique') {
        $source=$script:statusNative::OpenProcess(0x101000,$false,[uint32]$selection.selected.ProcessId)
        if ($source -eq [IntPtr]::Zero) { throw 'source_open_failed' };[void]$held.Add($source)
        if ($script:statusNative::WaitForSingleObject($source,0) -ne 258 -or -not [string]::Equals((Get-StatusImage $source),$cmd,[StringComparison]::OrdinalIgnoreCase)) { throw 'source_image_rejected' }
        [IntPtr]$sourceToken=[IntPtr]::Zero
        if (-not $script:statusNative::OpenProcessToken($source,8,[ref]$sourceToken)) { throw 'source_token_unreadable' };[void]$held.Add($sourceToken)
        $facts=Get-StatusFacts $sourceToken
        if ($facts.sid -cne $sid -or $facts.session -lt 0 -or $facts.tokenType -ne 1) { throw 'source_user_rejected' }
        $result.exactUserMatched=$true;$result.token=Convert-SessionToken $facts $sid
        if ($facts.elevationType -eq 2) {
            $result.linkedInspection='unknown'
            try {
                $buffer=Get-StatusBuffer $sourceToken 19
                try { $linked=[Runtime.InteropServices.Marshal]::ReadIntPtr($buffer) } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
                if ($linked -eq [IntPtr]::Zero) { throw 'linked_token_unavailable' };[void]$held.Add($linked)
                $linkedFacts=Get-StatusFacts $linked;$result.linkedToken=Convert-SessionToken $linkedFacts $sid
                $result.linkedMediumAvailable=Test-SessionLinkedMedium $linkedFacts $facts $sid;$result.linkedInspection='observed'
            } catch { $result.linkedMediumAvailable='unknown';$result.linkedInspection='unknown' }
        } elseif ($facts.elevationType -in @(1,3)) { $result.linkedInspection='not_applicable';$result.linkedMediumAvailable=$false }
        if ($script:statusNative::WaitForSingleObject($source,0) -ne 258 -or -not [string]::Equals((Get-StatusImage $source),$cmd,[StringComparison]::OrdinalIgnoreCase)) { throw 'source_exited_or_changed' }
        $result.status='observed'
    }
    $exitCode=0
} catch {
    $result.status='unknown'
    $known=@('host_rejected','account_rejected','caller_impersonating','caller_context_unknown','system_required','account_lookup_failed','system_path_unknown','source_open_failed','source_image_rejected','source_token_unreadable','source_user_rejected','source_exited_or_changed','token_read_failed','process_image_failed')
    if ($known -ccontains $_.Exception.Message) { $result.error=$_.Exception.Message } else { $result.error='facts_unavailable' }
} finally { if ($null -ne $script:statusNative) { for ($i=$held.Count-1;$i -ge 0;$i--) { [void]$script:statusNative::CloseHandle($held[$i]) } } }
$json=$result | ConvertTo-Json -Compress -Depth 6
if ($json.Length -gt 2400) { $json='{"schema":"ultra-qa3-session-facts-v1","status":"unknown","error":"summary_limit","productExecuted":false,"mediumTestExecuted":false}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode
