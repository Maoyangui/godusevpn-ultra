# Windows PowerShell 5.1 x64. Fixed QA package assembly; NEVER runs products.
# New private package-beta-qa2 only; no overwrite, ACL repair, deletion or retry.
# Trusted QA ancestors/admin/OS are required; hostile namespace mutation is not covered.
[CmdletBinding()]
param([string]$SupportStageID,[string]$CandidateStageID,[string]$CandidateLeaf)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue';$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue';$DebugPreference='SilentlyContinue'
$script:assembleZipHash='99f81bce6e6a432c7be28267627f7f8e7ab36e3b3de886cc0f7894e2d95d4131'
$script:assembleManifestHash='9f4d7836cefd477fed1b1f16247ae1c389aa7eed818c11f18a9185c8fc8eaf6d'
$script:assembleHashes=@{
'godusevpn.exe'='8aec81db98b683cb5de06fe6e3939f3cd493fc4018a7015d90a72da90f3d61eb'
'godusevpn-cli.exe'='7eed52a1e1e6f4424db54f4a2b067346f2dc7bd9df734687529422294cd9673e'
'godusevpn-svc.exe'='e9105cf5fe9f7e1167455a6654ff7ed3900f24308023663725ebd7f88729b14f'
}

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

function New-AssembleNative {
    $an=New-Object Reflection.AssemblyName('UltraBetaAssemble')
    $ab=[AppDomain]::CurrentDomain.DefineDynamicAssembly($an,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $tb=$ab.DefineDynamicModule('Native').DefineType('UltraBetaAssembleNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
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
        if ($stream.Length -lt 1 -or $stream.Length -gt $Limit -or ($Expected -ge 0 -and $stream.Length -ne $Expected)) { throw 'file_size_rejected' }
        $bytes=New-Object byte[] ([int]$stream.Length);$position=0
        while ($position -lt $bytes.Length) { $n=$stream.Read($bytes,$position,$bytes.Length-$position);if ($n -le 0) { throw 'file_size_rejected' };$position+=$n }
        return ,$bytes
    } finally { $stream.Dispose() }
}

function Assert-AssembleKeys($Value,[string[]]$Keys) {
    if ($Value -isnot [pscustomobject]) { throw 'manifest_rejected' }
    $actual=@($Value.PSObject.Properties.Name)
    if ($actual.Count -ne $Keys.Count) { throw 'manifest_rejected' }
    foreach ($key in $actual) { if ($Keys -cnotcontains $key) { throw 'manifest_rejected' } }
}

function Assert-AssembleInteger($Value,[long]$Low,[long]$High) {
    if (($Value -isnot [int] -and $Value -isnot [long]) -or $Value -lt $Low -or $Value -gt $High) { throw 'manifest_rejected' }
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
    if ($hashes.Count -ne 302 -or -not $hashes.ContainsKey('ULTRA_PACKAGE.json')) { throw 'manifest_rejected' }
    foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) {
        if (-not $hashes.ContainsKey($name) -or $hashes[$name] -cne $script:assembleHashes[$name]) { throw 'manifest_rejected' }
    }
    return ,$hashes
}
function Read-AssembleEntries([byte[]]$ZipBytes) {
    Add-Type -AssemblyName System.IO.Compression
    $input=[IO.MemoryStream]::new($ZipBytes,$false);$archive=$null
    $payload=[Collections.Generic.Dictionary[string,byte[]]]::new([StringComparer]::Ordinal)
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase);$total=0L
    try {
        $archive=[IO.Compression.ZipArchive]::new($input,[IO.Compression.ZipArchiveMode]::Read,$false)
        if ($archive.Entries.Count -ne 300) { throw 'zip_members_rejected' }
        foreach ($entry in $archive.Entries) {
            $name=$entry.FullName;Assert-AssembleName $name
            $attributes=[long]$entry.ExternalAttributes;$mode=($attributes -shr 16) -band 0xf000
            if (-not $seen.Add($name) -or [IO.Path]::GetExtension($name) -ieq '.exe' -or ($attributes -band 0x410) -ne 0 -or $mode -notin @(0,0x8000)) { throw 'zip_members_rejected' }
            if ($entry.Length -lt 1 -or $entry.Length -gt 16777216 -or $entry.CompressedLength -gt 16777216 -or $total+$entry.Length -gt 67108864) { throw 'zip_size_rejected' }
            $total+=$entry.Length;$stream=$entry.Open();$out=[IO.MemoryStream]::new()
            try {
                $buffer=New-Object byte[] 65536
                while (($n=$stream.Read($buffer,0,$buffer.Length)) -gt 0) { if ($out.Length+$n -gt $entry.Length) { throw 'zip_size_rejected' };$out.Write($buffer,0,$n) }
                if ($out.Length -ne $entry.Length) { throw 'zip_size_rejected' }
                $payload.Add($name,$out.ToArray())
            } finally { $stream.Dispose();$out.Dispose() }
        }
    } finally { if ($null -ne $archive) { $archive.Dispose() };$input.Dispose() }
    return ,$payload
}
function Read-AssembleBundle([byte[]]$ZipBytes) {
    if ($ZipBytes.Length -ne 10313075 -or (Get-AssembleHash $ZipBytes) -cne $script:assembleZipHash) { throw 'zip_hash_rejected' }
    $payload=Read-AssembleEntries $ZipBytes
    if (-not $payload.ContainsKey('SHA256SUMS') -or (Get-AssembleHash $payload['SHA256SUMS']) -cne $script:assembleManifestHash) { throw 'manifest_hash_rejected' }
    $hashes=Read-AssembleManifest $payload['SHA256SUMS']
    $expected=@($hashes.Keys | Where-Object { -not $script:assembleHashes.ContainsKey($_) })+'SHA256SUMS'
    if ($expected.Count -ne 300) { throw 'manifest_rejected' }
    foreach ($name in $expected) {
        if (-not $payload.ContainsKey($name)) { throw 'zip_members_rejected' }
        if ($name -cne 'SHA256SUMS' -and (Get-AssembleHash $payload[$name]) -cne $hashes[$name]) { throw 'payload_hash_rejected' }
    }
    $descriptor=[Text.UTF8Encoding]::new($false,$true).GetString($payload['ULTRA_PACKAGE.json']).TrimStart([char]0xfeff) | ConvertFrom-Json
    if ($descriptor.product -cne 'GodUseVPN Ultra' -or $descriptor.namespace -cne 'godusevpn-ultra' -or $descriptor.version -cne '0.01-beta-qa.2' -or $descriptor.architecture -cne 'amd64' -or $descriptor.unsigned -isnot [bool] -or -not $descriptor.unsigned) { throw 'descriptor_rejected' }
    return [pscustomobject]@{payload=$payload;hashes=$hashes}
}
function New-AssembleParents([string]$Root,[string]$Relative,[string]$PrivateRoot,$Created) {
    $parts=$Relative.Split([char]'/');$current=$Root
    for ($i=0;$i -lt $parts.Count-1;$i++) {
        $current=[IO.Path]::Combine($current,$parts[$i])
        if (-not $Created.Contains($current)) {
            if (-not $script:assembleNative::CreateDirectoryW($current,[IntPtr]::Zero)) { throw 'directory_create_failed' }
            Lock-AssembleDirectories $current $PrivateRoot
            [void]$Created.Add($current)
        }
    }
}
function Confirm-AssembleInventory([string]$Root,$Expected) {
    $found=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $pending=[Collections.Generic.Queue[string]]::new();$pending.Enqueue($Root)
    while ($pending.Count -gt 0) {
        foreach ($item in @(Get-ChildItem -LiteralPath $pending.Dequeue() -Force)) {
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'inventory_rejected' }
            if ($item.PSIsContainer) { $pending.Enqueue($item.FullName);continue }
            $name=$item.FullName.Substring($Root.Length+1).Replace('\','/')
            if (-not $Expected.ContainsKey($name) -or -not $found.Add($name)) { throw 'inventory_rejected' }
            $bytes=Read-AssembleFile $item.FullName -1 134217728
            if ((Get-AssembleHash $bytes) -cne $Expected[$name]) { throw 'target_hash_rejected' }
        }
    }
    if ($found.Count -ne 303 -or $found.Count -ne $Expected.Count) { throw 'inventory_rejected' }
}

$result=[ordered]@{schema='ultra-beta-assemble-v1';ready=$false;stage='preflight';error='';files=0;copiedEXEs=0;supportFiles=0;sha256Match=$false;staticVersionsMatched=0;privateChecked=$false;productExecuted=$false;installerExecuted=$false}
$script:assembleHeld=New-Object 'Collections.Generic.List[IntPtr]';$exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'host_rejected' }
    Assert-AssembleID $SupportStageID;Assert-AssembleID $CandidateStageID
    if ($CandidateLeaf -cnotmatch '^delta-[a-f0-9]{32}$') { throw 'stage_rejected' }
    $script:assembleNative=New-AssembleNative
    $known=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData,[Environment+SpecialFolderOption]::DoNotVerify)
    $qaRoot=[IO.Path]::Combine($known,'GodUseVPN-Ultra-QA')
    $support=[IO.Path]::Combine($qaRoot,$SupportStageID);$candidateStage=[IO.Path]::Combine($qaRoot,$CandidateStageID)
    $candidate=[IO.Path]::Combine($candidateStage,$CandidateLeaf);$output=[IO.Path]::Combine($candidateStage,'package-beta-qa2')
    Lock-AssembleDirectories $support $qaRoot;Lock-AssembleDirectories $candidate $qaRoot
    if ([IO.File]::Exists($output) -or [IO.Directory]::Exists($output)) { throw 'output_exists' }
    $result.stage='support'
    $bundle=Read-AssembleBundle (Read-AssembleFile ([IO.Path]::Combine($support,'preview.zip')) 10313075 10313075)
    $payload=$bundle.payload
    foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) {
        $path=[IO.Path]::Combine($candidate,$name);$bytes=Read-AssembleFile $path -1 134217728
        if ((Get-AssembleHash $bytes) -cne $script:assembleHashes[$name]) { throw 'source_hash_rejected' }
        if ([Diagnostics.FileVersionInfo]::GetVersionInfo($path).ProductVersion -cne '0.01-beta-qa.2') { throw 'static_version_rejected' }
        $payload.Add($name,$bytes)
    }
    if ($payload.Count -ne 303) { throw 'inventory_rejected' }
    $result.stage='create'
    if (-not $script:assembleNative::CreateDirectoryW($output,[IntPtr]::Zero)) { throw 'directory_create_failed' }
    Lock-AssembleDirectories $output $qaRoot
    $created=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase);[void]$created.Add($output)
    foreach ($name in @($payload.Keys | Sort-Object)) {
        New-AssembleParents $output $name $qaRoot $created
        $path=[IO.Path]::Combine($output,$name.Replace('/','\'));$bytes=$payload[$name]
        $stream=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try { $stream.Write($bytes,0,$bytes.Length);$stream.Flush($true) } finally { $stream.Dispose() }
        $actual=Read-AssembleFile $path $bytes.Length 134217728
        if ((Get-AssembleHash $actual) -cne (Get-AssembleHash $bytes)) { throw 'target_hash_rejected' }
    }
    $result.stage='verify'
    $expected=[Collections.Generic.Dictionary[string,string]]::new($bundle.hashes,[StringComparer]::Ordinal)
    $expected.Add('SHA256SUMS',$script:assembleManifestHash)
    Confirm-AssembleInventory $output $expected
    foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) {
        if ([Diagnostics.FileVersionInfo]::GetVersionInfo([IO.Path]::Combine($output,$name)).ProductVersion -cne '0.01-beta-qa.2') { throw 'static_version_rejected' }
        $result.staticVersionsMatched++
    }
    $result.ready=$true;$result.files=303;$result.copiedEXEs=3;$result.supportFiles=300;$result.sha256Match=$true;$result.privateChecked=$true;$result.stage='complete';$exitCode=0
} catch {
    $knownErrors=@('host_rejected','stage_rejected','path_rejected','drive_rejected','handle_rejected','private_acl_rejected','directory_open_failed','directory_rejected','file_open_failed','file_rejected','file_size_rejected','member_name_rejected','manifest_rejected','zip_members_rejected','zip_size_rejected','zip_hash_rejected','manifest_hash_rejected','payload_hash_rejected','descriptor_rejected','directory_create_failed','inventory_rejected','target_hash_rejected','output_exists','source_hash_rejected','static_version_rejected')
    $code=$_.Exception.Message;if ($knownErrors -cnotcontains $code) { $code='check_failed' };$result.error=$code
} finally {
    for ($i=$script:assembleHeld.Count-1;$i -ge 0;$i--) { [void]$script:assembleNative::CloseHandle($script:assembleHeld[$i]) }
}
$json=$result | ConvertTo-Json -Compress -Depth 4
if ($json.Length -gt 2000) { $json='{"schema":"ultra-beta-assemble-v1","ready":false,"error":"summary_limit","productExecuted":false}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode
