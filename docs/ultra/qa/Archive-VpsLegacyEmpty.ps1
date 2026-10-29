# Windows PowerShell 5.1 x64, SYSTEM. Explicit QA archive operation ONLY.
# Before execution ROOT must separately obtain consent for this exact move.
# Moves fixed ProgramData/godusevpn-ultra to the existing private QA StageID's
# fresh legacy-empty-before-beta child. No products are executed, ACLs repaired,
# objects deleted, migration performed, or rollback/retry attempted.
# Only one privacy-machine directory containing the two empty legacy artifacts
# is accepted. Source owner must be SY/BA; one historical account-form allow SID
# is permitted ONLY in the explicitly archived source, and is never emitted.
# Existing role-only QA ancestors remain role-only; legacy ACLs/bytes are kept.
# Same-volume Directory.Move does not overwrite. Source handles close before
# rename; this assumes a trusted administrator and stable namespace, not safety
# against malicious administrators or all concurrent namespace/registry races.
[CmdletBinding()]
param([string]$StageID)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue'
$DebugPreference='SilentlyContinue'

function Get-ArchivePolicies {
    return @(
        [pscustomobject]@{path='SOFTWARE\Policies\Microsoft\Windows\AdvertisingInfo';name='DisabledByGroupPolicy'},
        [pscustomobject]@{path='SOFTWARE\Policies\Microsoft\InputPersonalization';name='AllowInputPersonalization'},
        [pscustomobject]@{path='SOFTWARE\Policies\Microsoft\Windows\System';name='AllowClipboardHistory'},
        [pscustomobject]@{path='SOFTWARE\Policies\Microsoft\Windows\System';name='AllowCrossDeviceClipboard'}
    )
}
function Get-ArchiveHash([byte[]]$Bytes) {
    $h=[Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($h.ComputeHash($Bytes))).Replace('-','').ToLowerInvariant() }
    finally { $h.Dispose() }
}
function Assert-ArchiveEnvelope([byte[]]$Bytes) {
    if ($Bytes.Length -lt 1 -or $Bytes.Length -gt 4194304) { throw 'journal_rejected' }
    $text=(New-Object Text.UTF8Encoding($false,$true)).GetString($Bytes)
    $match=[regex]::Match($text,'\A\{"payload":(?<payload>\{.*\}),"sha256":"(?<sha>[a-f0-9]{64})"\}\z',[Text.RegularExpressions.RegexOptions]::Singleline)
    if (-not $match.Success -or $match.Groups['payload'].Value -cne '{"version":1,"userKey":"machine-policy-store-v1","controls":{}}' -or (Get-ArchiveHash ([Text.Encoding]::UTF8.GetBytes($match.Groups['payload'].Value))) -cne $match.Groups['sha'].Value) { throw 'journal_rejected' }
}
function Assert-ArchiveSD([string]$Sddl,[bool]$Legacy) {
    $sd=New-Object Security.AccessControl.RawSecurityDescriptor($Sddl)
    if ($null -eq $sd.Owner -or $sd.Owner.Value -notin @('S-1-5-18','S-1-5-32-544') -or $null -eq $sd.DiscretionaryAcl) { throw 'acl_rejected' }
    $extra=New-Object 'Collections.Generic.HashSet[string]'
    foreach ($ace in $sd.DiscretionaryAcl) {
        if ($ace -isnot [Security.AccessControl.CommonAce] -or $ace.IsCallback) { throw 'acl_rejected' }
        if ($ace.AceQualifier -eq [Security.AccessControl.AceQualifier]::AccessAllowed -and $ace.SecurityIdentifier.Value -notin @('S-1-5-18','S-1-5-32-544')) {
            if (-not $Legacy -or $ace.SecurityIdentifier.Value -notmatch '^S-1-5-21-(?:[0-9]+-){3}[0-9]+$') { throw 'acl_rejected' }
            [void]$extra.Add($ace.SecurityIdentifier.Value)
        }
    }
    if ($extra.Count -gt 1) { throw 'acl_rejected' }
    return [pscustomobject]@{legacySids=@($extra)}
}
function Assert-ArchiveEntries($RootEntries,$MachineEntries) {
    if ($RootEntries.Count -ne 1 -or $RootEntries[0] -cne 'privacy-machine' -or $MachineEntries.Count -ne 2 -or -not ($MachineEntries -ccontains 'privacy-journal.json') -or -not ($MachineEntries -ccontains 'privacy.lock')) { throw 'extra_entries' }
}
function Assert-ArchiveSnapshot($Snapshot) {
    if ($null -eq $Snapshot -or -not $Snapshot.empty -or $Snapshot.fileCount -ne 2 -or $Snapshot.lockBytes -ne 0 -or $Snapshot.legacyCount -gt 1) { throw 'snapshot_rejected' }
    foreach ($name in @('journalHash','lockHash')) { if ($Snapshot.$name -cnotmatch '^[0-9a-f]{64}$') { throw 'snapshot_rejected' } }
}
function Assert-ArchiveSame($Before,$After) {
    Assert-ArchiveSnapshot $Before;Assert-ArchiveSnapshot $After
    foreach ($name in @('journalHash','lockHash','rootSddl','machineSddl','journalSddl','lockSddl')) { if ($Before.$name -cne $After.$name) { throw 'snapshot_changed' } }
}
function Invoke-ArchivePlan([scriptblock]$Preflight,[scriptblock]$ReadSource,[scriptblock]$Move,[scriptblock]$ReadTarget,[scriptblock]$SourceAbsent,[scriptblock]$PoliciesAbsent) {
    $r=[ordered]@{status='rejected';preflightPassed=$false;moveAttempted=$false;files=0;hashesMatch=$false;aclMatch=$false;emptyControls=$false;sourceAbsent=$false;policiesAbsent=$false;error=''}
    try {
        &$Preflight
        $before=&$ReadSource;Assert-ArchiveSnapshot $before
        if (-not (&$PoliciesAbsent)) { throw 'policy_present' }
        $r.preflightPassed=$true
        &$Preflight
        $fresh=&$ReadSource;Assert-ArchiveSame $before $fresh
        if (-not (&$PoliciesAbsent)) { throw 'policy_present' }
        $r.moveAttempted=$true
        &$Move
        $after=&$ReadTarget;Assert-ArchiveSame $before $after
        $r.files=2;$r.hashesMatch=$true;$r.aclMatch=$true;$r.emptyControls=$true
        $r.sourceAbsent=[bool](&$SourceAbsent)
        if (-not $r.sourceAbsent) { throw 'source_still_exists' }
        $r.policiesAbsent=[bool](&$PoliciesAbsent)
        if (-not $r.policiesAbsent) { throw 'policy_present' }
        $r.status='completed'
    } catch {
        if ($r.moveAttempted) { $r.status='review_required' }
        $known=@('journal_rejected','acl_rejected','extra_entries','snapshot_rejected','snapshot_changed','policy_present','source_still_exists','destination_exists','environment_not_empty')
        if ($known -ccontains $_.Exception.Message) { $r.error=$_.Exception.Message } else { $r.error='archive_check_failed' }
    }
    return $r
}

function New-ArchiveNative {
    $an=New-Object Reflection.AssemblyName('UltraLegacyArchive')
    $ab=[AppDomain]::CurrentDomain.DefineDynamicAssembly($an,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $tb=$ab.DefineDynamicModule('Native').DefineType('UltraLegacyArchiveNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $specs=@(
        @('CreateFileW',[IntPtr],[type[]]@([string],[uint32],[uint32],[IntPtr],[uint32],[uint32],[IntPtr])),
        @('GetFileInformationByHandleEx',[bool],[type[]]@([IntPtr],[int],[IntPtr],[uint32])),
        @('CloseHandle',[bool],[type[]]@([IntPtr])),
        @('QueryDosDeviceW',[uint32],[type[]]@([string],[Text.StringBuilder],[uint32]))
    )
    $ctor=[Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields=[Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'),[Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'),[Runtime.InteropServices.DllImportAttribute].GetField('CharSet'),[Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    foreach ($s in $specs) {
        $m=$tb.DefineMethod($s[0],[Reflection.MethodAttributes]'Public, Static, PinvokeImpl',$s[1],$s[2]);$m.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $a=New-Object Reflection.Emit.CustomAttributeBuilder($ctor,[object[]]@('kernel32.dll'),$fields,[object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi));$m.SetCustomAttribute($a)
    }
    return $tb.CreateType()
}
function Get-ArchiveComponents([string]$Path) {
    if (-not $Path -or $Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or $Path.Substring(2).Contains(':')) { throw 'path_rejected' }
    $parts=@($Path.Substring(0,3));$current=$parts[0]
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if (-not $part.Length -or $part -in @('.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'path_rejected' }
        foreach ($c in $part.ToCharArray()) { if ([int]$c -lt 32) { throw 'path_rejected' } }
        if ($part.Split([char]'.')[0].ToUpperInvariant() -match '^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])$') { throw 'path_rejected' }
        $current=[IO.Path]::Combine($current,$part);$parts+=$current
    }
    return $parts
}
function Close-ArchiveHandles($Handles) { for ($i=$Handles.Count-1;$i -ge 0;$i--) { [void]$script:archiveNative::CloseHandle($Handles[$i]) } }
function Open-ArchiveNode([string]$Path,[bool]$Directory,$Handles) {
    [uint32]$access=0x20080;[uint32]$flags=0x00200000
    if ($Directory) { $access=$access -bor 1;$flags=$flags -bor 0x02000000 } else { $access=$access -bor 0x80000000L }
    $h=$script:archiveNative::CreateFileW($Path,$access,1,[IntPtr]::Zero,3,$flags,[IntPtr]::Zero)
    if ($h -eq [IntPtr](-1)) { throw 'node_open_failed' }
    [void]$Handles.Add($h)
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal(24)
    try {
        if (-not $script:archiveNative::GetFileInformationByHandleEx($h,9,$buffer,8)) { throw 'node_rejected' }
        $attrs=[Runtime.InteropServices.Marshal]::ReadInt32($buffer)
        if (($attrs -band 0x400) -ne 0 -or ((($attrs -band 0x10) -ne 0) -ne $Directory)) { throw 'node_rejected' }
        if (-not $Directory -and (-not $script:archiveNative::GetFileInformationByHandleEx($h,1,$buffer,24) -or [Runtime.InteropServices.Marshal]::ReadInt32($buffer,16) -ne 1)) { throw 'node_rejected' }
    } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
}
function Get-ArchiveSddl([string]$Path,[bool]$Legacy) {
    $acl=Get-Acl -LiteralPath $Path
    $sections=[Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Group -bor [Security.AccessControl.AccessControlSections]::Access
    $sddl=$acl.GetSecurityDescriptorSddlForm($sections)
    $null=Assert-ArchiveSD $sddl $Legacy
    return $sddl
}
function Open-ArchiveParents([string]$Path,[string]$PrivateFrom,$Handles) {
    $parts=@(Get-ArchiveComponents $Path);$mapping=New-Object Text.StringBuilder(32768)
    if ($script:archiveNative::QueryDosDeviceW($parts[0].Substring(0,2),$mapping,32768) -eq 0 -or $mapping.ToString() -notmatch '^\\Device\\HarddiskVolume[0-9]+$' -or (New-Object IO.DriveInfo($parts[0])).DriveType -ne [IO.DriveType]::Fixed) { throw 'drive_rejected' }
    foreach ($part in $parts) {
        Open-ArchiveNode $part $true $Handles
        if ($PrivateFrom -and ($part.Equals($PrivateFrom,[StringComparison]::OrdinalIgnoreCase) -or $part.StartsWith($PrivateFrom+'\',[StringComparison]::OrdinalIgnoreCase))) { $null=Get-ArchiveSddl $part $false }
    }
}
function Test-ArchiveAbsent([string]$Path) {
    $h=$script:archiveNative::CreateFileW($Path,0x80,1,[IntPtr]::Zero,3,0x02200000,[IntPtr]::Zero)
    if ($h -ne [IntPtr](-1)) { [void]$script:archiveNative::CloseHandle($h);return $false }
    $code=[Runtime.InteropServices.Marshal]::GetLastWin32Error()
    if ($code -in @(2,3)) { return $true }
    throw 'absence_unconfirmed'
}
function Read-ArchiveBytes([string]$Path,[int]$Limit) {
    $f=[IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
    try {
        if ($f.Length -gt $Limit) { throw 'file_size_rejected' }
        $bytes=New-Object byte[] ([int]$f.Length);$position=0
        while ($position -lt $bytes.Length) { $read=$f.Read($bytes,$position,$bytes.Length-$position);if ($read -le 0) { throw 'file_read_failed' };$position+=$read }
        return ,$bytes
    } finally { $f.Dispose() }
}
function Read-ArchiveSnapshot([string]$Root) {
    $handles=New-Object 'Collections.Generic.List[IntPtr]'
    try {
        Open-ArchiveNode $Root $true $handles
        $rootNames=@([IO.Directory]::GetFileSystemEntries($Root) | ForEach-Object {[IO.Path]::GetFileName($_)})
        if ($rootNames.Count -ne 1 -or $rootNames[0] -cne 'privacy-machine') { throw 'extra_entries' }
        $machine=[IO.Path]::Combine($Root,'privacy-machine');Open-ArchiveNode $machine $true $handles
        $machineNames=@([IO.Directory]::GetFileSystemEntries($machine) | ForEach-Object {[IO.Path]::GetFileName($_)})
        Assert-ArchiveEntries $rootNames $machineNames
        $journal=[IO.Path]::Combine($machine,'privacy-journal.json');$lock=[IO.Path]::Combine($machine,'privacy.lock')
        Open-ArchiveNode $journal $false $handles;Open-ArchiveNode $lock $false $handles
        $r=[ordered]@{empty=$true;fileCount=2;lockBytes=0;legacyCount=0;journalHash='';lockHash='';rootSddl='';machineSddl='';journalSddl='';lockSddl=''}
        $legacy=New-Object 'Collections.Generic.HashSet[string]'
        foreach ($pair in @(@('rootSddl',$Root),@('machineSddl',$machine),@('journalSddl',$journal),@('lockSddl',$lock))) {
            $sddl=Get-ArchiveSddl $pair[1] $true;$r[$pair[0]]=$sddl
            $info=Assert-ArchiveSD $sddl $true
            foreach ($sid in $info.legacySids) { [void]$legacy.Add($sid) }
        }
        if ($legacy.Count -gt 1) { throw 'acl_rejected' };$r.legacyCount=$legacy.Count
        $bytes=Read-ArchiveBytes $journal 4194304;Assert-ArchiveEnvelope $bytes;$r.journalHash=Get-ArchiveHash $bytes
        $lockBytes=Read-ArchiveBytes $lock 0;$r.lockHash=Get-ArchiveHash $lockBytes
        return [pscustomobject]$r
    } finally { Close-ArchiveHandles $handles }
}
function Test-ArchivePoliciesAbsent {
    $hive=[Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine,[Microsoft.Win32.RegistryView]::Registry64)
    try {
        foreach ($policy in @(Get-ArchivePolicies)) {
            $key=$hive.OpenSubKey($policy.path,$false)
            if ($null -ne $key) { try { if ($key.GetValueNames() -contains $policy.name) { return $false } } finally { $key.Dispose() } }
        }
        return $true
    } finally { $hive.Dispose() }
}
function Assert-ArchiveNoInstallation {
    Add-Type -AssemblyName System.ServiceProcess
    $services=[ServiceProcess.ServiceController]::GetServices()
    try { if (@($services | Where-Object {$_.ServiceName -in @('godusevpn','godusevpn-ultra')}).Count -ne 0) { throw 'environment_not_empty' } }
    finally { foreach ($service in $services) { $service.Dispose() } }
    foreach ($name in @('godusevpn','godusevpn-cli','godusevpn-svc')) {
        $processes=[Diagnostics.Process]::GetProcessesByName($name)
        try { if ($processes.Count -ne 0) { throw 'environment_not_empty' } } finally { foreach ($process in $processes) { $process.Dispose() } }
    }
    foreach ($folder in @([Environment+SpecialFolder]::ProgramFiles,[Environment+SpecialFolder]::ProgramFilesX86)) {
        $base=[Environment]::GetFolderPath($folder,[Environment+SpecialFolderOption]::DoNotVerify)
        if (-not $base) { continue }
        $pins=New-Object 'Collections.Generic.List[IntPtr]'
        try {
            Open-ArchiveParents $base '' $pins
            foreach ($leaf in @('godusevpn','GodUseVPN Ultra')) { if (-not (Test-ArchiveAbsent ([IO.Path]::Combine($base,$leaf)))) { throw 'environment_not_empty' } }
        } finally { Close-ArchiveHandles $pins }
    }
    if (-not (Test-ArchiveAbsent $script:archiveTarget)) { throw 'destination_exists' }
}

$result=[ordered]@{schema='ultra-legacy-empty-archive-v1';status='rejected';systemToken=$false;archive=$null;productExecuted=$false;aclModified=$false;objectsDeleted=$false;productionMigration=$false;rollbackAttempted=$false;error=''}
$script:archiveNative=$null;$held=New-Object 'Collections.Generic.List[IntPtr]';$exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT -or $StageID -cnotmatch '^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$') { throw 'host_or_stage_rejected' }
    $identity=[Security.Principal.WindowsIdentity]::GetCurrent()
    try { if (-not $identity.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalSystemSid)) { throw 'system_required' };$result.systemToken=$true } finally { $identity.Dispose() }
    $script:archiveNative=New-ArchiveNative
    $programData=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData,[Environment+SpecialFolderOption]::DoNotVerify)
    Open-ArchiveParents $programData '' $held
    $source=[IO.Path]::Combine($programData,'godusevpn-ultra')
    $qa=[IO.Path]::Combine($programData,'GodUseVPN-Ultra-QA');$stage=[IO.Path]::Combine($qa,$StageID)
    Open-ArchiveParents $stage $qa $held
    $script:archiveTarget=[IO.Path]::Combine($stage,'legacy-empty-before-beta')
    $result.archive=Invoke-ArchivePlan {Assert-ArchiveNoInstallation} {Read-ArchiveSnapshot $source} {[IO.Directory]::Move($source,$script:archiveTarget)} {Read-ArchiveSnapshot $script:archiveTarget} {Test-ArchiveAbsent $source} {Test-ArchivePoliciesAbsent}
    $result.status=$result.archive.status
    if ($result.status -ceq 'completed') { $exitCode=0 }
} catch { $result.error='preflight_failed' }
finally { if ($null -ne $script:archiveNative) { Close-ArchiveHandles $held } }
$json=$result | ConvertTo-Json -Compress -Depth 6
if ($json.Length -gt 3000) { $json='{"schema":"ultra-legacy-empty-archive-v1","status":"rejected","error":"summary_limit"}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode
