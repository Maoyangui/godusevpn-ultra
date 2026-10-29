# Windows PowerShell 5.1 x64, SYSTEM. PREVIEW ONLY: no Install mode exists. V2 uses IntPtr NULL-safe SCM binding.
# Fixed qa.4 package preflight then official Install-Ultra.ps1 -WhatIf only.
# No product EXE, service start/register, ACL change, directory creation, retry,
# cleanup or rollback. Existing private QA ancestors/admin/OS must be trusted.
# Not a hostile-administrator/kernel/DOS-map race defense or install acceptance.
[CmdletBinding()]
param([string]$CandidateStageID)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue';$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue';$DebugPreference='SilentlyContinue'
$script:assembleManifestHash='2b42f32810d8f27a0bcd7126f217946a41a5ee95b5f4b3c5c69453163f24ea48'
$script:assembleHashes=@{
    'godusevpn.exe'='c321a62437521c6c8bfcb5b290c78396dd5fb9b7a2464c2c626d064794b8bafc'
    'godusevpn-cli.exe'='53107fddbbbea4b11030d1aaede6beae6839b27ea7b4412d0e6fadbd9d4044be'
    'godusevpn-svc.exe'='b2ede7f86ebe73b4d8e5b3010507357b6f9eb27af4585f2e0f760189cad083f8'
}
$script:installLengths=@{'godusevpn.exe'=16122368;'godusevpn-cli.exe'=8723456;'godusevpn-svc.exe'=43166720}

function Get-AssembleHash([byte[]]$Bytes) {
    $sha=[Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($sha.ComputeHash($Bytes))).Replace('-','').ToLowerInvariant() }
    finally { $sha.Dispose() }
}

function Assert-AssembleID([string]$Value) {
    if ($Value -cnotmatch '^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$') { throw 'stage_rejected' }
}

function Get-AssembleComponents([string]$Path) {
    if (-not $Path -or $Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or $Path.Substring(2).Contains(':') -or $Path.Length -gt 30000) { throw 'path_rejected' }
    $root=$Path.Substring(0,3);$result=@($root);$current=$root
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if (-not $part.Length -or $part -in @('.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'path_rejected' }
        foreach ($ch in $part.ToCharArray()) { if ([int]$ch -lt 32) { throw 'path_rejected' } }
        if ($part.Split([char]'.')[0] -match '^(?i:CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])$') { throw 'path_rejected' }
        $current=[IO.Path]::Combine($current,$part);$result+=$current
    }
    return $result
}

function Assert-AssemblePrivateSD([string]$Sddl) {
    $sd=New-Object Security.AccessControl.RawSecurityDescriptor($Sddl)
    if ($null -eq $sd.Owner -or $sd.Owner.Value -notin @('S-1-5-18','S-1-5-32-544') -or $null -eq $sd.DiscretionaryAcl) { throw 'private_acl_rejected' }
    foreach ($ace in $sd.DiscretionaryAcl) {
        if ($ace -isnot [Security.AccessControl.CommonAce] -or $ace.IsCallback -or ($ace.AceQualifier -eq [Security.AccessControl.AceQualifier]::AccessAllowed -and $ace.SecurityIdentifier.Value -notin @('S-1-5-18','S-1-5-32-544'))) { throw 'private_acl_rejected' }
    }
}

function Assert-AssemblePrivatePath([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path -ErrorAction Stop
    Assert-AssemblePrivateSD ($acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access))
}

function Get-AssembleAttributes([IntPtr]$Handle) {
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal(8)
    try {
        if (-not $script:assembleNative::GetFileInformationByHandleEx($Handle,9,$buffer,8)) { throw 'handle_rejected' }
        return [Runtime.InteropServices.Marshal]::ReadInt32($buffer)
    } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
}

function Lock-AssembleDirectories([string]$Path,[string]$PrivateRoot) {
    $parts=@(Get-AssembleComponents $Path)
    $mapping=New-Object Text.StringBuilder(32768)
    if ($script:assembleNative::QueryDosDeviceW($parts[0].Substring(0,2),$mapping,32768) -eq 0 -or $mapping.ToString() -notmatch '^\\Device\\HarddiskVolume[0-9]+$' -or (New-Object IO.DriveInfo($parts[0])).DriveType -ne [IO.DriveType]::Fixed) { throw 'drive_rejected' }
    foreach ($part in $parts) {
        $h=$script:assembleNative::CreateFileW($part,0x81,1,[IntPtr]::Zero,3,0x02200000,[IntPtr]::Zero)
        if ($h -eq [IntPtr](-1)) { throw 'directory_open_failed' }
        [void]$script:assembleHeld.Add($h)
        $attributes=Get-AssembleAttributes $h
        if (($attributes -band 0x400) -ne 0 -or ($attributes -band 0x10) -eq 0) { throw 'directory_rejected' }
        if ($PrivateRoot -and ($part.Equals($PrivateRoot,[StringComparison]::OrdinalIgnoreCase) -or $part.StartsWith($PrivateRoot+'\',[StringComparison]::OrdinalIgnoreCase))) { Assert-AssemblePrivatePath $part }
    }
}

function Read-AssembleFile([string]$Path,[long]$Expected,[long]$Limit) {
    $h=$script:assembleNative::CreateFileW($Path,[uint32]2147483776,1,[IntPtr]::Zero,3,0x00200000,[IntPtr]::Zero)
    if ($h -eq [IntPtr](-1)) { throw 'file_open_failed' }
    [void]$script:assembleHeld.Add($h)
    if (((Get-AssembleAttributes $h) -band 0x410) -ne 0) { throw 'file_rejected' }
    Assert-AssemblePrivatePath $Path
    $stream=[IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
    try {
        # The immutable qa.4 manifest includes two empty build/vet records.
        # Empty ordinary files still require the fixed per-file SHA256.
        if ($stream.Length -lt 0 -or $stream.Length -gt $Limit -or ($Expected -ge 0 -and $stream.Length -ne $Expected)) { throw 'file_size_rejected' }
        $bytes=New-Object byte[] ([int]$stream.Length);$position=0
        while ($position -lt $bytes.Length) { $n=$stream.Read($bytes,$position,$bytes.Length-$position);if ($n -le 0) { throw 'file_size_rejected' };$position+=$n }
        return ,$bytes
    } finally { $stream.Dispose() }
}

function Assert-AssembleName([string]$Name) {
    if (-not $Name -or $Name -cne $Name.Trim() -or $Name.Contains('\') -or $Name.Contains(':') -or $Name.StartsWith('/')) { throw 'member_name_rejected' }
    foreach ($part in $Name.Split([char]'/')) {
        if (-not $part.Length -or $part -in @('.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'member_name_rejected' }
        foreach ($ch in $part.ToCharArray()) { if ([int]$ch -lt 32) { throw 'member_name_rejected' } }
        if ($part.Split([char]'.')[0] -match '^(?i:CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])$') { throw 'member_name_rejected' }
    }
}

function Read-AssembleManifest([byte[]]$Bytes) {
    if ($Bytes.Length -lt 1 -or $Bytes.Length -gt 4194304) { throw 'manifest_rejected' }
    $text=[Text.UTF8Encoding]::new($false,$true).GetString($Bytes).TrimEnd([char[]]@("`r","`n"))
    $hashes=[Collections.Generic.Dictionary[string,string]]::new([StringComparer]::Ordinal)
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($line in [regex]::Split($text,'\r?\n')) {
        $match=[regex]::Match($line,'\A([a-f0-9]{64})  (.+)\z')
        if (-not $match.Success) { throw 'manifest_rejected' }
        $name=$match.Groups[2].Value;Assert-AssembleName $name
        if ($name -ieq 'SHA256SUMS' -or -not $seen.Add($name)) { throw 'manifest_rejected' }
        $hashes.Add($name,$match.Groups[1].Value)
    }
    if ($hashes.Count -ne 406 -or -not $hashes.ContainsKey('ULTRA_PACKAGE.json')) { throw 'manifest_rejected' }
    foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) {
        if (-not $hashes.ContainsKey($name) -or $hashes[$name] -cne $script:assembleHashes[$name]) { throw 'manifest_rejected' }
    }
    return ,$hashes
}

function New-InstallNative {
    $an=New-Object Reflection.AssemblyName('UltraQa4InstallPreview')
    $ab=[AppDomain]::CurrentDomain.DefineDynamicAssembly($an,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $tb=$ab.DefineDynamicModule('Native').DefineType('UltraQa4InstallPreviewNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $ctor=[Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields=[Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'),[Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'),[Runtime.InteropServices.DllImportAttribute].GetField('CharSet'),[Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    foreach ($s in @(
        @('kernel32.dll','CreateFileW',[IntPtr],[type[]]@([string],[uint32],[uint32],[IntPtr],[uint32],[uint32],[IntPtr])),
        @('kernel32.dll','GetFileInformationByHandleEx',[bool],[type[]]@([IntPtr],[int],[IntPtr],[uint32])),
        @('kernel32.dll','CloseHandle',[bool],[type[]]@([IntPtr])),
        @('kernel32.dll','QueryDosDeviceW',[uint32],[type[]]@([string],[Text.StringBuilder],[uint32])),
        @('advapi32.dll','OpenSCManagerW',[IntPtr],[type[]]@([IntPtr],[IntPtr],[uint32])),
        @('advapi32.dll','OpenServiceW',[IntPtr],[type[]]@([IntPtr],[string],[uint32])),
        @('advapi32.dll','CloseServiceHandle',[bool],[type[]]@([IntPtr]))
    )) {
        $m=$tb.DefineMethod($s[1],[Reflection.MethodAttributes]'Public, Static, PinvokeImpl',$s[2],$s[3]);$m.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $m.SetCustomAttribute((New-Object Reflection.Emit.CustomAttributeBuilder($ctor,[object[]]@($s[0]),$fields,[object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi))))
    }
    return $tb.CreateType()
}
function Test-InstallAbsent([string]$Path) {
    $h=$script:assembleNative::CreateFileW($Path,0x80,1,[IntPtr]::Zero,3,0x02200000,[IntPtr]::Zero)
    if ($h -ne [IntPtr](-1)) { [void]$script:assembleNative::CloseHandle($h);return $false }
    if ([Runtime.InteropServices.Marshal]::GetLastWin32Error() -in @(2,3)) { return $true }
    throw 'absence_unknown'
}
function Assert-InstallNoServices {
    $manager=$script:assembleNative::OpenSCManagerW([IntPtr]::Zero,[IntPtr]::Zero,1)
    if ($manager -eq [IntPtr]::Zero) { throw 'SCM_unknown' }
    try {
        foreach ($name in @('godusevpn','godusevpn-ultra')) {
            $service=$script:assembleNative::OpenServiceW($manager,$name,4)
            if ($service -ne [IntPtr]::Zero) { [void]$script:assembleNative::CloseServiceHandle($service);throw 'service_exists' }
            if ([Runtime.InteropServices.Marshal]::GetLastWin32Error() -ne 1060) { throw 'SCM_unknown' }
        }
    } finally { [void]$script:assembleNative::CloseServiceHandle($manager) }
}
function Get-InstallPolicies {
    return @(
        @('SOFTWARE\Policies\Microsoft\Windows\AdvertisingInfo','DisabledByGroupPolicy'),
        @('SOFTWARE\Policies\Microsoft\InputPersonalization','AllowInputPersonalization'),
        @('SOFTWARE\Policies\Microsoft\Windows\System','AllowClipboardHistory'),
        @('SOFTWARE\Policies\Microsoft\Windows\System','AllowCrossDeviceClipboard')
    )
}
function Assert-InstallPoliciesAbsent {
    $hive=[Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine,[Microsoft.Win32.RegistryView]::Registry64)
    try {
        foreach ($policy in @(Get-InstallPolicies)) {
            $key=$hive.OpenSubKey($policy[0],$false)
            if ($null -ne $key) { try { if ($key.GetValueNames() -contains $policy[1]) { throw 'policy_present' } } finally { $key.Dispose() } }
        }
    } finally { $hive.Dispose() }
}
function Assert-InstallEmptyJournal([byte[]]$Bytes) {
    if ($Bytes.Length -lt 1 -or $Bytes.Length -gt 4194304) { throw 'machine_not_empty' }
    $text=[Text.UTF8Encoding]::new($false,$true).GetString($Bytes)
    $m=[regex]::Match($text,'\A\{"payload":(?<payload>\{.*\}),"sha256":"(?<sha>[a-f0-9]{64})"\}\z',[Text.RegularExpressions.RegexOptions]::Singleline)
    if (-not $m.Success -or $m.Groups['payload'].Value -cne '{"version":1,"userKey":"machine-policy-store-v1","controls":{}}' -or (Get-AssembleHash ([Text.Encoding]::UTF8.GetBytes($m.Groups['payload'].Value))) -cne $m.Groups['sha'].Value) { throw 'machine_not_empty' }
}
function Read-InstallMachine([string]$Root) {
    if (Test-InstallAbsent $Root) { return 'absent' }
    Lock-AssembleDirectories $Root $Root
    $entries=@(Get-ChildItem -LiteralPath $Root -Force)
    if ($entries.Count -ne 1 -or $entries[0].Name -cne 'privacy-machine' -or -not $entries[0].PSIsContainer) { throw 'core_or_unknown_data_present' }
    $machine=[IO.Path]::Combine($Root,'privacy-machine');Lock-AssembleDirectories $machine $Root
    $entries=@(Get-ChildItem -LiteralPath $machine -Force)
    if ($entries.Count -ne 2 -or @($entries | Where-Object {$_.Name -ceq 'privacy-journal.json'}).Count -ne 1 -or @($entries | Where-Object {$_.Name -ceq 'privacy.lock'}).Count -ne 1) { throw 'machine_not_empty' }
    $journal=Read-AssembleFile ([IO.Path]::Combine($machine,'privacy-journal.json')) -1 4194304
    Assert-InstallEmptyJournal $journal
    $lock=Read-AssembleFile ([IO.Path]::Combine($machine,'privacy.lock')) 0 0
    return (Get-AssembleHash $journal)+':'+(Get-AssembleHash $lock)
}
function Assert-InstallInventory([string]$Root,[string]$PrivateRoot) {
    Lock-AssembleDirectories $Root $PrivateRoot
    $manifest=Read-AssembleFile ([IO.Path]::Combine($Root,'SHA256SUMS')) -1 4194304
    if ((Get-AssembleHash $manifest) -cne $script:assembleManifestHash) { throw 'manifest_hash_rejected' }
    $expected=Read-AssembleManifest $manifest;$expected.Add('SHA256SUMS',$script:assembleManifestHash)
    $dirs=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($name in $expected.Keys) {
        $parent=[IO.Path]::GetDirectoryName($name.Replace('/','\'))
        while ($parent) { [void]$dirs.Add($parent.Replace('\','/'));$parent=[IO.Path]::GetDirectoryName($parent) }
    }
    $found=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal);$pending=[Collections.Generic.Queue[string]]::new();$pending.Enqueue($Root);$versions=0;$descriptor=$false
    while ($pending.Count) {
        foreach ($item in @(Get-ChildItem -LiteralPath $pending.Dequeue() -Force)) {
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'inventory_rejected' }
            $name=$item.FullName.Substring($Root.Length+1).Replace('\','/')
            if ($item.PSIsContainer) {
                if (-not $dirs.Contains($name)) { throw 'inventory_rejected' };Lock-AssembleDirectories $item.FullName $PrivateRoot;$pending.Enqueue($item.FullName);continue
            }
            if (-not $expected.ContainsKey($name) -or -not $found.Add($name)) { throw 'inventory_rejected' }
            $length=-1;if ($script:installLengths.ContainsKey($name)) { $length=$script:installLengths[$name] }
            $bytes=Read-AssembleFile $item.FullName $length 134217728
            if ((Get-AssembleHash $bytes) -cne $expected[$name]) { throw 'target_hash_rejected' }
            if ($script:installLengths.ContainsKey($name)) { if ([Diagnostics.FileVersionInfo]::GetVersionInfo($item.FullName).ProductVersion -cne '0.01-beta-qa.4') { throw 'static_version_rejected' };$versions++ }
            if ($name -ceq 'ULTRA_PACKAGE.json') {
                $d=[Text.UTF8Encoding]::new($false,$true).GetString($bytes).TrimStart([char]0xfeff) | ConvertFrom-Json
                if ($d.product -cne 'GodUseVPN Ultra' -or $d.namespace -cne 'godusevpn-ultra' -or $d.version -cne '0.01-beta-qa.4' -or $d.architecture -cne 'amd64' -or $d.unsigned -isnot [bool] -or -not $d.unsigned) { throw 'descriptor_rejected' };$descriptor=$true
            }
        }
    }
    if ($found.Count -ne 407 -or $versions -ne 3 -or -not $descriptor) { throw 'inventory_rejected' }
}
function Assert-InstallEnvironment([string]$ProgramData) {
    foreach ($target in @([EnvironmentVariableTarget]::Process,[EnvironmentVariableTarget]::Machine)) { if ([Environment]::GetEnvironmentVariable('GODUSEVPN_ULTRA_DATA',$target)) { throw 'data_override_present' } }
    Assert-InstallNoServices
    foreach ($name in @('godusevpn','godusevpn-cli','godusevpn-svc')) {
        $processes=[Diagnostics.Process]::GetProcessesByName($name)
        try { if ($processes.Count) { throw 'product_process_present' } } finally { foreach ($p in $processes) { $p.Dispose() } }
    }
    foreach ($folder in @([Environment+SpecialFolder]::ProgramFiles,[Environment+SpecialFolder]::ProgramFilesX86)) {
        $base=[Environment]::GetFolderPath($folder,[Environment+SpecialFolderOption]::DoNotVerify)
        if (-not $base) { throw 'known_folder_unknown' };Lock-AssembleDirectories $base ''
        foreach ($name in @('godusevpn','GodUseVPN Ultra')) { if (-not (Test-InstallAbsent ([IO.Path]::Combine($base,$name)))) { throw 'installation_exists' } }
    }
    Assert-InstallPoliciesAbsent
    return Read-InstallMachine ([IO.Path]::Combine($ProgramData,'godusevpn-ultra'))
}
function Assert-InstallWhatIfOutput([string]$Text,[string]$Target) {
    $lines=@([regex]::Split($Text.Trim(),'\r?\n'))
    $flat=[regex]::Replace($Text,'\s+',' ')
    if ($Text.Length -gt 65536 -or $lines.Count -gt 16 -or @($lines | Where-Object {$_ -ceq 'Verified unsigned Ultra 0.01-beta-qa.4, 406 payload files.'}).Count -ne 1 -or @($lines | Where-Object {$_ -ceq ('Target: '+$Target)}).Count -ne 1 -or -not $flat.Contains('Create protected directory, copy verified payload, install/start godusevpn-ultra service') -or $Text.Contains('Installed Ultra at ')) { throw 'whatif_receipt_rejected' }
}
function Invoke-InstallWhatIf([string]$Shell,[string]$Package,[string]$Target) {
    $info=New-Object Diagnostics.ProcessStartInfo;$info.FileName=$Shell;$info.WorkingDirectory=$Package;$info.UseShellExecute=$false;$info.CreateNoWindow=$true
    $info.Arguments='-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+[IO.Path]::Combine($Package,'Install-Ultra.ps1')+'" -Source "'+$Package+'" -WhatIf'
    $info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
    $process=New-Object Diagnostics.Process;$process.StartInfo=$info;$memory=[IO.MemoryStream]::new();$started=$false;$complete=$false
    try {
        if (-not $process.Start()) { throw 'whatif_start_failed' };$started=$true;$script:installPreviewStarted=$true
        $watch=[Diagnostics.Stopwatch]::StartNew();$buffer=New-Object byte[] 4096;$errors=New-Object byte[] 256
        $stdout=$process.StandardOutput.BaseStream.ReadAsync($buffer,0,$buffer.Length);$stderr=$process.StandardError.BaseStream.ReadAsync($errors,0,$errors.Length)
        while ($null -ne $stdout -or $null -ne $stderr -or -not $process.HasExited) {
            if ($watch.ElapsedMilliseconds -ge 60000) { throw 'whatif_timeout' }
            if ($null -ne $stdout -and $stdout.IsCompleted) { $n=$stdout.GetAwaiter().GetResult();if ($n -eq 0) { $stdout=$null } else { if ($memory.Length+$n -gt 65536) { throw 'whatif_output_rejected' };$memory.Write($buffer,0,$n);$stdout=$process.StandardOutput.BaseStream.ReadAsync($buffer,0,$buffer.Length) } }
            if ($null -ne $stderr -and $stderr.IsCompleted) { $n=$stderr.GetAwaiter().GetResult();if ($n -gt 0) { throw 'whatif_output_rejected' };$stderr=$null }
            if ($null -ne $stdout -or $null -ne $stderr -or -not $process.HasExited) { [Threading.Thread]::Sleep(10) }
        }
        $process.WaitForExit();if ($process.ExitCode -ne 0) { throw 'whatif_failed' }
        Assert-InstallWhatIfOutput ([Text.UTF8Encoding]::new($false,$true).GetString($memory.ToArray())) $Target;$complete=$true
    } finally {
        if ($started -and -not $complete) { try { if (-not $process.HasExited) { $process.Kill() };if (-not $process.WaitForExit(3000) -or -not $process.HasExited) { $script:installTerminationUnknown=$true } } catch { $script:installTerminationUnknown=$true } }
        $memory.Dispose();$process.Dispose()
    }
}
function Invoke-InstallPreviewPlan([scriptblock]$Preflight,[scriptblock]$WhatIf) {
    $r=[ordered]@{status='rejected';preflightPassed=$false;whatIfPassed=$false;postflightPassed=$false;unchanged=$false;error=''}
    try {
        $before=&$Preflight;$r.preflightPassed=$true
        &$WhatIf;$r.whatIfPassed=$true
        $after=&$Preflight;$r.postflightPassed=$true
        if ($before -cne $after) { throw 'state_changed' };$r.unchanged=$true;$r.status='completed'
    } catch { if ($r.preflightPassed) { $r.status='review_required' };$known=@('state_changed','whatif_timeout','whatif_failed','whatif_receipt_rejected','whatif_output_rejected','whatif_start_failed','SCM_unknown','service_exists','installation_exists','policy_present','machine_not_empty','core_or_unknown_data_present','product_process_present','data_override_present');if ($known -ccontains $_.Exception.Message) { $r.error=$_.Exception.Message } else { $r.error='preflight_or_preview_failed' } }
    return $r
}

$result=[ordered]@{schema='ultra-qa4-install-preview-v2';mode='preflight_whatif_only';status='rejected';error='';effectiveSystemCaller=$false;files=0;payload=0;preview=$null;whatIfStarted=$false;terminationUnknown=$false;actualInstallSupported=$false;productExecuted=$false;installationRequested=$false;aclModified=$false}
$script:assembleNative=$null;$script:assembleHeld=New-Object 'Collections.Generic.List[IntPtr]';$script:installPreviewStarted=$false;$script:installTerminationUnknown=$false;$exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'host_rejected' }
    Assert-AssembleID $CandidateStageID
    # Effective identity check only; this does not certify a primary token or
    # session 0. No actual-install mode is available under this weaker evidence.
    $id=[Security.Principal.WindowsIdentity]::GetCurrent();try { if (-not $id.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalSystemSid)) { throw 'system_required' };$result.effectiveSystemCaller=$true } finally { $id.Dispose() }
    $script:assembleNative=New-InstallNative
    $data=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData,[Environment+SpecialFolderOption]::DoNotVerify)
    $qa=[IO.Path]::Combine($data,'GodUseVPN-Ultra-QA');$package=[IO.Path]::Combine([IO.Path]::Combine($qa,$CandidateStageID),'package-beta-qa4')
    $target=[IO.Path]::Combine([Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles),'GodUseVPN Ultra')
    $shellDir=[IO.Path]::Combine([Environment]::GetFolderPath([Environment+SpecialFolder]::System),'WindowsPowerShell\v1.0')
    Lock-AssembleDirectories $shellDir ''
    $result.preview=Invoke-InstallPreviewPlan { Assert-InstallInventory $package $qa;Assert-InstallEnvironment $data } { Invoke-InstallWhatIf ([IO.Path]::Combine($shellDir,'powershell.exe')) $package $target }
    $result.status=$result.preview.status
    if ($result.status -ceq 'completed') { $result.files=407;$result.payload=406;$exitCode=0 }
} catch { $result.error='host_or_preflight_failed' }
finally {
    if ($null -ne $script:assembleNative) { for ($i=$script:assembleHeld.Count-1;$i -ge 0;$i--) { [void]$script:assembleNative::CloseHandle($script:assembleHeld[$i]) } }
    $result.whatIfStarted=$script:installPreviewStarted;$result.terminationUnknown=$script:installTerminationUnknown
}
$json=$result | ConvertTo-Json -Compress -Depth 5
if ($json.Length -gt 2500) { $json='{"schema":"ultra-qa4-install-preview-v2","status":"rejected","error":"summary_limit","actualInstallSupported":false}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode

