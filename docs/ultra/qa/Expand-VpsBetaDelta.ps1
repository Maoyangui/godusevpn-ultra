# Windows PowerShell 5.1 x64. Fixed QA file reconstruction; no product runs.
# Existing ProgramData/GodUseVPN-Ultra-QA ancestors must be trusted. This
# wrapper checks SY/BA private objects; it never repairs ACLs or overwrites.
# Partial new transfer/output directories are retained on failure. No retry.
# It does not defend against hostile administrators/kernel/DOS-map mutation.
[CmdletBinding()]
param([string]$StageID,[string]$CandidateLeaf,[string]$BaseStageID)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue';$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue';$DebugPreference='SilentlyContinue'
$script:expandZipHash='af5513ea94de4118e2f72bc650da4fee2d18610925a38a3b17dc421b6f885b7f'
$script:expandManifestHash='8f81b6a3a78d500b8d1423ecd1d951beeb81a697e2b50c5190aba8f480835be5'
$script:expandDecoderHash='e4035a5be9d064f3ad51ef79ecce87dc616edd318b87ad32d67fc6862a4a6adf'
$script:expandHashes=@{
    'godusevpn.exe'='8aec81db98b683cb5de06fe6e3939f3cd493fc4018a7015d90a72da90f3d61eb'
    'godusevpn-cli.exe'='7eed52a1e1e6f4424db54f4a2b067346f2dc7bd9df734687529422294cd9673e'
    'godusevpn-svc.exe'='e9105cf5fe9f7e1167455a6654ff7ed3900f24308023663725ebd7f88729b14f'
}

function Get-ExpandHash([byte[]]$Bytes) {
    $sha=[Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($sha.ComputeHash($Bytes))).Replace('-','').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
function Assert-ExpandID([string]$Value) {
    if ($Value -cnotmatch '^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$') { throw 'stage_rejected' }
}
function Get-ExpandComponents([string]$Path) {
    if (-not $Path -or $Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or $Path.Substring(2).Contains(':') -or $Path.Length -gt 30000) { throw 'path_rejected' }
    $root=$Path.Substring(0,3);$result=@($root);$current=$root
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if (-not $part.Length -or $part -in @('.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'path_rejected' }
        foreach ($ch in $part.ToCharArray()) { if ([int]$ch -lt 32) { throw 'path_rejected' } }
        if ($part.Split([char]'.')[0] -match '^(?i:CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9¹²³]|LPT[1-9¹²³])$') { throw 'path_rejected' }
        $current=[IO.Path]::Combine($current,$part);$result+=$current
    }
    return $result
}
function Assert-ExpandPrivateSD([string]$Sddl) {
    $sd=New-Object Security.AccessControl.RawSecurityDescriptor($Sddl)
    if ($null -eq $sd.Owner -or $sd.Owner.Value -notin @('S-1-5-18','S-1-5-32-544') -or $null -eq $sd.DiscretionaryAcl) { throw 'private_acl_rejected' }
    foreach ($ace in $sd.DiscretionaryAcl) {
        if ($ace -isnot [Security.AccessControl.CommonAce] -or $ace.IsCallback -or ($ace.AceQualifier -eq [Security.AccessControl.AceQualifier]::AccessAllowed -and $ace.SecurityIdentifier.Value -notin @('S-1-5-18','S-1-5-32-544'))) { throw 'private_acl_rejected' }
    }
}
function Assert-ExpandPrivatePath([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path -ErrorAction Stop
    Assert-ExpandPrivateSD ($acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access))
}
function New-ExpandNative {
    $an=New-Object Reflection.AssemblyName('UltraBetaExpand')
    $ab=[AppDomain]::CurrentDomain.DefineDynamicAssembly($an,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $tb=$ab.DefineDynamicModule('Native').DefineType('UltraBetaExpandNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $ctor=[Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields=[Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'),[Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'),[Runtime.InteropServices.DllImportAttribute].GetField('CharSet'),[Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    foreach ($spec in @(
        @('CreateFileW',[IntPtr],[type[]]@([string],[uint32],[uint32],[IntPtr],[uint32],[uint32],[IntPtr])),
        @('GetFileInformationByHandleEx',[bool],[type[]]@([IntPtr],[int],[IntPtr],[uint32])),
        @('CloseHandle',[bool],[type[]]@([IntPtr])),
        @('QueryDosDeviceW',[uint32],[type[]]@([string],[Text.StringBuilder],[uint32])),
        @('CreateDirectoryW',[bool],[type[]]@([string],[IntPtr]))
    )) {
        $m=$tb.DefineMethod($spec[0],[Reflection.MethodAttributes]'Public, Static, PinvokeImpl',$spec[1],$spec[2])
        $m.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $m.SetCustomAttribute((New-Object Reflection.Emit.CustomAttributeBuilder($ctor,[object[]]@('kernel32.dll'),$fields,[object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi))))
    }
    return $tb.CreateType()
}
function Get-ExpandAttributes([IntPtr]$Handle) {
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal(8)
    try {
        if (-not $script:expandNative::GetFileInformationByHandleEx($Handle,9,$buffer,8)) { throw 'handle_rejected' }
        return [Runtime.InteropServices.Marshal]::ReadInt32($buffer)
    } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
}
function Lock-ExpandDirectories([string]$Path,[string]$PrivateRoot) {
    $parts=@(Get-ExpandComponents $Path)
    $mapping=New-Object Text.StringBuilder(32768)
    if ($script:expandNative::QueryDosDeviceW($parts[0].Substring(0,2),$mapping,32768) -eq 0 -or $mapping.ToString() -notmatch '^\\Device\\HarddiskVolume[0-9]+$' -or (New-Object IO.DriveInfo($parts[0])).DriveType -ne [IO.DriveType]::Fixed) { throw 'drive_rejected' }
    foreach ($part in $parts) {
        $h=$script:expandNative::CreateFileW($part,0x81,1,[IntPtr]::Zero,3,0x02200000,[IntPtr]::Zero)
        if ($h -eq [IntPtr](-1)) { throw 'directory_open_failed' }
        [void]$script:expandHeld.Add($h)
        $attributes=Get-ExpandAttributes $h
        if (($attributes -band 0x400) -ne 0 -or ($attributes -band 0x10) -eq 0) { throw 'directory_rejected' }
        if ($PrivateRoot -and ($part.Equals($PrivateRoot,[StringComparison]::OrdinalIgnoreCase) -or $part.StartsWith($PrivateRoot+'\',[StringComparison]::OrdinalIgnoreCase))) { Assert-ExpandPrivatePath $part }
    }
}
function Read-ExpandFile([string]$Path,[long]$Expected,[long]$Limit) {
    $h=$script:expandNative::CreateFileW($Path,[uint32]2147483776,1,[IntPtr]::Zero,3,0x00200000,[IntPtr]::Zero)
    if ($h -eq [IntPtr](-1)) { throw 'file_open_failed' }
    [void]$script:expandHeld.Add($h)
    if (((Get-ExpandAttributes $h) -band 0x410) -ne 0) { throw 'file_rejected' }
    Assert-ExpandPrivatePath $Path
    $stream=[IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
    try {
        if ($stream.Length -lt 1 -or $stream.Length -gt $Limit -or ($Expected -ge 0 -and $stream.Length -ne $Expected)) { throw 'file_size_rejected' }
        $bytes=New-Object byte[] ([int]$stream.Length);$position=0
        while ($position -lt $bytes.Length) { $n=$stream.Read($bytes,$position,$bytes.Length-$position);if ($n -le 0) { throw 'file_size_rejected' };$position+=$n }
        return ,$bytes
    } finally { $stream.Dispose() }
}
function Assert-ExpandKeys($Value,[string[]]$Keys) {
    if ($Value -isnot [pscustomobject]) { throw 'manifest_rejected' }
    $actual=@($Value.PSObject.Properties.Name)
    if ($actual.Count -ne $Keys.Count) { throw 'manifest_rejected' }
    foreach ($key in $actual) { if ($Keys -cnotcontains $key) { throw 'manifest_rejected' } }
}
function Assert-ExpandInteger($Value,[long]$Low,[long]$High) {
    if (($Value -isnot [int] -and $Value -isnot [long]) -or $Value -lt $Low -or $Value -gt $High) { throw 'manifest_rejected' }
}
function Read-ExpandArchiveEntries([byte[]]$ZipBytes) {
    Add-Type -AssemblyName System.IO.Compression
    $input=[IO.MemoryStream]::new($ZipBytes,$false);$archive=$null
    $expected=@('manifest.json','Apply-VpsDelta.ps1','godusevpn.exe.delta.gz','godusevpn-cli.exe.delta.gz','godusevpn-svc.exe.delta.gz')
    $payload=@{};$total=0L
    try {
        $archive=[IO.Compression.ZipArchive]::new($input,[IO.Compression.ZipArchiveMode]::Read,$false)
        if ($archive.Entries.Count -ne 5) { throw 'zip_members_rejected' }
        foreach ($entry in $archive.Entries) {
            $prefix='vps-beta-qa2-transfer-v2/'
            if (-not $entry.FullName.StartsWith($prefix,[StringComparison]::Ordinal)) { throw 'zip_members_rejected' }
            $name=$entry.FullName.Substring($prefix.Length)
            $attributes=[long]$entry.ExternalAttributes;$mode=($attributes -shr 16) -band 0xf000
            if ($expected -cnotcontains $name -or $payload.ContainsKey($name) -or ($attributes -band 0x410) -ne 0 -or $mode -notin @(0,0x8000)) { throw 'zip_members_rejected' }
            $limit=33554432L;if ($name -ceq 'manifest.json') { $limit=16384L };if ($name -ceq 'Apply-VpsDelta.ps1') { $limit=131072L }
            if ($entry.Length -lt 1 -or $entry.Length -gt $limit -or $entry.CompressedLength -gt 33554432 -or ($total+$entry.Length) -gt 67108864) { throw 'zip_size_rejected' }
            $total+=$entry.Length;$stream=$entry.Open();$out=[IO.MemoryStream]::new()
            try {
                $buffer=New-Object byte[] 65536
                while (($n=$stream.Read($buffer,0,$buffer.Length)) -gt 0) { if ($out.Length+$n -gt $entry.Length) { throw 'zip_size_rejected' };$out.Write($buffer,0,$n) }
                if ($out.Length -ne $entry.Length) { throw 'zip_size_rejected' }
                $payload[$name]=$out.ToArray()
            } finally { $stream.Dispose();$out.Dispose() }
        }
    } finally { if ($null -ne $archive) { $archive.Dispose() };$input.Dispose() }
    return ,$payload
}
function Read-ExpandBundle([byte[]]$ZipBytes) {
    if ($ZipBytes.Length -ne 16013722 -or (Get-ExpandHash $ZipBytes) -cne $script:expandZipHash) { throw 'zip_hash_rejected' }
    $payload=Read-ExpandArchiveEntries $ZipBytes
    if ((Get-ExpandHash $payload['manifest.json']) -cne $script:expandManifestHash -or (Get-ExpandHash $payload['Apply-VpsDelta.ps1']) -cne $script:expandDecoderHash) { throw 'bundle_hash_rejected' }
    $manifest=[Text.UTF8Encoding]::new($false,$true).GetString($payload['manifest.json']) | ConvertFrom-Json
    Assert-ExpandKeys $manifest @('schema','blockBytes','files')
    Assert-ExpandInteger $manifest.blockBytes 256 256
    if ($manifest.schema -cne 'godusevpn-ultra-exe-delta-v1' -or $manifest.files -isnot [array] -or $manifest.files.Count -ne 3) { throw 'manifest_rejected' }
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($file in $manifest.files) {
        Assert-ExpandKeys $file @('name','baseSha256','newSha256','baseLength','newLength','patchFile','patchSha256','patchLength','encodedLength')
        if (-not $script:expandHashes.ContainsKey([string]$file.name) -or -not $seen.Add([string]$file.name) -or $file.patchFile -cne ($file.name+'.delta.gz') -or $file.newSha256 -cne $script:expandHashes[$file.name]) { throw 'manifest_rejected' }
        foreach ($field in @('baseSha256','newSha256','patchSha256')) { if ($file.$field -isnot [string] -or $file.$field -cnotmatch '^[0-9a-f]{64}$') { throw 'manifest_rejected' } }
        Assert-ExpandInteger $file.baseLength 1 134217728
        Assert-ExpandInteger $file.newLength 1 134217728
        Assert-ExpandInteger $file.encodedLength 13 ($file.newLength*2L+32L)
        Assert-ExpandInteger $file.patchLength 1 33554432
        if ($payload[$file.patchFile].Length -ne $file.patchLength -or (Get-ExpandHash $payload[$file.patchFile]) -cne $file.patchSha256) { throw 'patch_hash_rejected' }
    }
    return [pscustomobject]@{payload=$payload;manifest=$manifest}
}
function Invoke-ExpandDecoder([string]$Shell,[string]$Decoder,[string]$Base,[string]$Transfer,[string]$Output) {
    $info=New-Object Diagnostics.ProcessStartInfo
    $info.FileName=$Shell;$info.WorkingDirectory=$Transfer;$info.UseShellExecute=$false;$info.CreateNoWindow=$true
    # All paths were built from fixed names and validated components (no quotes).
    $info.Arguments='-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$Decoder+'" -BaseDirectory "'+$Base+'" -DeltaDirectory "'+$Transfer+'" -OutputDirectory "'+$Output+'" -ExpectedManifestSha256 '+$script:expandManifestHash
    $info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
    $process=New-Object Diagnostics.Process;$process.StartInfo=$info
    $memory=[IO.MemoryStream]::new();$started=$false;$complete=$false
    try {
        if (-not $process.Start()) { throw 'decoder_start_failed' };$started=$true;$script:expandDecoderStarted=$true
        $watch=[Diagnostics.Stopwatch]::StartNew();$buffer=New-Object byte[] 4096;$errors=New-Object byte[] 256
        $outTask=$process.StandardOutput.BaseStream.ReadAsync($buffer,0,$buffer.Length);$errTask=$process.StandardError.BaseStream.ReadAsync($errors,0,$errors.Length)
        while ($null -ne $outTask -or $null -ne $errTask -or -not $process.HasExited) {
            if ($watch.ElapsedMilliseconds -ge 120000) { throw 'decoder_timeout' }
            if ($null -ne $outTask -and $outTask.IsCompleted) {
                $n=$outTask.GetAwaiter().GetResult()
                if ($n -eq 0) { $outTask=$null } else { if ($memory.Length+$n -gt 4096) { throw 'decoder_output_rejected' };$memory.Write($buffer,0,$n);$outTask=$process.StandardOutput.BaseStream.ReadAsync($buffer,0,$buffer.Length) }
            }
            if ($null -ne $errTask -and $errTask.IsCompleted) { $n=$errTask.GetAwaiter().GetResult();if ($n -gt 0) { throw 'decoder_output_rejected' };$errTask=$null }
            if ($null -ne $outTask -or $null -ne $errTask -or -not $process.HasExited) { [Threading.Thread]::Sleep(10) }
        }
        $process.WaitForExit();if ($process.ExitCode -ne 0) { throw 'decoder_failed' }
        $receipt=[Text.UTF8Encoding]::new($false,$true).GetString($memory.ToArray()) | ConvertFrom-Json
        Assert-ExpandKeys $receipt @('schema','ready','files','sha256Match','productExecuted')
        if ($receipt.schema -cne 'godusevpn-ultra-delta-result-v1' -or $receipt.ready -isnot [bool] -or -not $receipt.ready -or $receipt.files -isnot [int] -or $receipt.files -ne 3 -or $receipt.sha256Match -isnot [bool] -or -not $receipt.sha256Match -or $receipt.productExecuted -isnot [bool] -or $receipt.productExecuted) { throw 'decoder_output_rejected' }
        $complete=$true
    } finally {
        if ($started -and -not $complete) { try { if (-not $process.HasExited) { $process.Kill() };if (-not $process.WaitForExit(3000) -or -not $process.HasExited) { $script:expandTerminationUnknown=$true } } catch { $script:expandTerminationUnknown=$true } }
        $memory.Dispose();$process.Dispose()
    }
}

$result=[ordered]@{schema='ultra-beta-expand-v1';ready=$false;stage='preflight';error='';files=0;hashesMatched=0;sha256Match=$false;privateChecked=$false;decoderExecuted=$false;terminationUnknown=$false;productExecuted=$false}
$script:expandHeld=New-Object 'Collections.Generic.List[IntPtr]'
$script:expandDecoderStarted=$false;$script:expandTerminationUnknown=$false;$exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'host_rejected' }
    Assert-ExpandID $StageID;Assert-ExpandID $BaseStageID
    if ($CandidateLeaf -cnotmatch '^delta-[a-f0-9]{32}$') { throw 'stage_rejected' }
    $script:expandNative=New-ExpandNative
    $known=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData,[Environment+SpecialFolderOption]::DoNotVerify)
    $qaRoot=[IO.Path]::Combine($known,'GodUseVPN-Ultra-QA')
    $stage=[IO.Path]::Combine($qaRoot,$StageID);$base=[IO.Path]::Combine([IO.Path]::Combine($qaRoot,$BaseStageID),'base-for-beta-qa2')
    $candidate=[IO.Path]::Combine($stage,$CandidateLeaf);$transfer=[IO.Path]::Combine($stage,'transfer-beta-qa2')
    Lock-ExpandDirectories $stage $qaRoot;Lock-ExpandDirectories $base $qaRoot
    if ([IO.File]::Exists($candidate) -or [IO.Directory]::Exists($candidate) -or [IO.File]::Exists($transfer) -or [IO.Directory]::Exists($transfer)) { throw 'output_exists' }
    $result.stage='zip'
    $bundle=Read-ExpandBundle (Read-ExpandFile ([IO.Path]::Combine($stage,'preview.zip')) 16013722 16013722)
    foreach ($file in $bundle.manifest.files) {
        $bytes=Read-ExpandFile ([IO.Path]::Combine($base,$file.name)) $file.baseLength 134217728
        if ((Get-ExpandHash $bytes) -cne $file.baseSha256) { throw 'base_hash_rejected' }
    }
    $result.stage='transfer'
    if (-not $script:expandNative::CreateDirectoryW($transfer,[IntPtr]::Zero)) { throw 'transfer_create_failed' }
    Lock-ExpandDirectories $transfer $qaRoot
    foreach ($name in @('manifest.json','Apply-VpsDelta.ps1','godusevpn.exe.delta.gz','godusevpn-cli.exe.delta.gz','godusevpn-svc.exe.delta.gz')) {
        $path=[IO.Path]::Combine($transfer,$name);$stream=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try { $bytes=$bundle.payload[$name];$stream.Write($bytes,0,$bytes.Length);$stream.Flush($true) } finally { $stream.Dispose() }
        $bytes=Read-ExpandFile $path $bundle.payload[$name].Length 33554432
        if ((Get-ExpandHash $bytes) -cne (Get-ExpandHash $bundle.payload[$name])) { throw 'transfer_hash_rejected' }
    }
    $result.stage='decode'
    $system=[Environment]::GetFolderPath([Environment+SpecialFolder]::System,[Environment+SpecialFolderOption]::DoNotVerify)
    $shellDirectory=[IO.Path]::Combine($system,'WindowsPowerShell\v1.0');Lock-ExpandDirectories $shellDirectory ''
    Invoke-ExpandDecoder ([IO.Path]::Combine($shellDirectory,'powershell.exe')) ([IO.Path]::Combine($transfer,'Apply-VpsDelta.ps1')) $base $transfer $candidate
    $result.stage='verify';Lock-ExpandDirectories $candidate $qaRoot
    $items=@(Get-ChildItem -LiteralPath $candidate -Force)
    if ($items.Count -ne 3) { throw 'target_rejected' }
    foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) {
        $bytes=Read-ExpandFile ([IO.Path]::Combine($candidate,$name)) -1 134217728
        if ((Get-ExpandHash $bytes) -cne $script:expandHashes[$name]) { throw 'target_hash_rejected' }
        $result.hashesMatched++
    }
    $result.ready=$true;$result.files=3;$result.sha256Match=$true;$result.privateChecked=$true;$result.stage='complete';$exitCode=0
} catch {
    $knownErrors=@('host_rejected','stage_rejected','path_rejected','drive_rejected','handle_rejected','private_acl_rejected','directory_open_failed','directory_rejected','file_open_failed','file_rejected','file_size_rejected','zip_hash_rejected','zip_members_rejected','zip_size_rejected','bundle_hash_rejected','manifest_rejected','patch_hash_rejected','output_exists','base_hash_rejected','transfer_create_failed','transfer_hash_rejected','decoder_start_failed','decoder_timeout','decoder_output_rejected','decoder_failed','target_rejected','target_hash_rejected')
    $code=$_.Exception.Message;if ($knownErrors -cnotcontains $code) { $code='check_failed' };$result.error=$code
} finally {
    for ($i=$script:expandHeld.Count-1;$i -ge 0;$i--) { [void]$script:expandNative::CloseHandle($script:expandHeld[$i]) }
    $result.decoderExecuted=$script:expandDecoderStarted;$result.terminationUnknown=$script:expandTerminationUnknown
}
$json=$result | ConvertTo-Json -Compress -Depth 4
if ($json.Length -gt 2000) { $json='{"schema":"ultra-beta-expand-v1","ready":false,"error":"summary_limit","productExecuted":false}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode
