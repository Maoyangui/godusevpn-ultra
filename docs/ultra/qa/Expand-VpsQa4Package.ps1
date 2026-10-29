# Windows PowerShell 5.1 x64. Fixed QA4 package expansion; NEVER runs products.
# Reads one immutable preview.zip from a private SYSTEM staging directory and
# creates one new package-beta-qa4 sibling. It never overwrites or repairs.
# Trusted QA ancestors/admin/OS are required; hostile kernel/administrator and
# DOS-device mutation are outside this wrapper's boundary.
[CmdletBinding()]
param([string]$StageID)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue';$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue';$DebugPreference='SilentlyContinue'

$script:qa4ZipLength=37933559L
$script:qa4ZipHash='fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144'
$script:qa4ManifestHash='2b42f32810d8f27a0bcd7126f217946a41a5ee95b5f4b3c5c69453163f24ea48'
$script:qa4Prefix='godusevpn-ultra-0.01-beta-qa.4-windows-amd64/'
$script:qa4Hashes=@{
    'godusevpn.exe'='c321a62437521c6c8bfcb5b290c78396dd5fb9b7a2464c2c626d064794b8bafc'
    'godusevpn-cli.exe'='53107fddbbbea4b11030d1aaede6beae6839b27ea7b4412d0e6fadbd9d4044be'
    'godusevpn-svc.exe'='b2ede7f86ebe73b4d8e5b3010507357b6f9eb27af4585f2e0f760189cad083f8'
}
$script:qa4Lengths=@{
    'godusevpn.exe'=16122368L
    'godusevpn-cli.exe'=8723456L
    'godusevpn-svc.exe'=43166720L
}

function Get-Qa4Hash([byte[]]$Bytes) {
    $sha=[Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($sha.ComputeHash($Bytes))).Replace('-','').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
function Assert-Qa4ID([string]$Value) {
    if ($Value -cnotmatch '^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$') { throw 'stage_rejected' }
}
function Get-Qa4Components([string]$Path) {
    if (-not $Path -or $Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or
        $Path.Substring(2).Contains(':') -or $Path.Length -gt 30000) { throw 'path_rejected' }
    $root=$Path.Substring(0,3);$result=@($root);$current=$root
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if (-not $part.Length -or $part -in @('.','..') -or $part.EndsWith('.') -or
            $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'path_rejected' }
        foreach ($ch in $part.ToCharArray()) { if ([int]$ch -lt 32) { throw 'path_rejected' } }
        if ($part.Split([char]'.')[0] -match '^(?i:CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])$') { throw 'path_rejected' }
        $current=[IO.Path]::Combine($current,$part);$result+=$current
    }
    return $result
}
function Assert-Qa4PrivateSD([string]$Sddl) {
    $sd=New-Object Security.AccessControl.RawSecurityDescriptor($Sddl)
    if ($null -eq $sd.Owner -or $sd.Owner.Value -notin @('S-1-5-18','S-1-5-32-544') -or $null -eq $sd.DiscretionaryAcl) { throw 'private_acl_rejected' }
    foreach ($ace in $sd.DiscretionaryAcl) {
        if ($ace -isnot [Security.AccessControl.CommonAce] -or $ace.IsCallback -or
            ($ace.AceQualifier -eq [Security.AccessControl.AceQualifier]::AccessAllowed -and
             $ace.SecurityIdentifier.Value -notin @('S-1-5-18','S-1-5-32-544'))) { throw 'private_acl_rejected' }
    }
}
function Assert-Qa4PrivatePath([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path -ErrorAction Stop
    Assert-Qa4PrivateSD ($acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access))
}
function New-Qa4Native {
    $an=New-Object Reflection.AssemblyName('UltraBetaQa4Expand')
    $ab=[AppDomain]::CurrentDomain.DefineDynamicAssembly($an,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $tb=$ab.DefineDynamicModule('Native').DefineType('UltraBetaQa4ExpandNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
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
function Get-Qa4Attributes([IntPtr]$Handle) {
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal(8)
    try {
        if (-not $script:qa4Native::GetFileInformationByHandleEx($Handle,9,$buffer,8)) { throw 'handle_rejected' }
        return [Runtime.InteropServices.Marshal]::ReadInt32($buffer)
    } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
}
function Lock-Qa4Directories([string]$Path,[string]$PrivateRoot) {
    $parts=@(Get-Qa4Components $Path)
    $mapping=New-Object Text.StringBuilder(32768)
    if ($script:qa4Native::QueryDosDeviceW($parts[0].Substring(0,2),$mapping,32768) -eq 0 -or
        $mapping.ToString() -notmatch '^\\Device\\HarddiskVolume[0-9]+$' -or
        (New-Object IO.DriveInfo($parts[0])).DriveType -ne [IO.DriveType]::Fixed) { throw 'drive_rejected' }
    foreach ($part in $parts) {
        $h=$script:qa4Native::CreateFileW($part,0x81,1,[IntPtr]::Zero,3,0x02200000,[IntPtr]::Zero)
        if ($h -eq [IntPtr](-1)) { throw 'directory_open_failed' }
        [void]$script:qa4Held.Add($h)
        $attributes=Get-Qa4Attributes $h
        if (($attributes -band 0x400) -ne 0 -or ($attributes -band 0x10) -eq 0) { throw 'directory_rejected' }
        if ($PrivateRoot -and ($part.Equals($PrivateRoot,[StringComparison]::OrdinalIgnoreCase) -or
            $part.StartsWith($PrivateRoot+'\',[StringComparison]::OrdinalIgnoreCase))) { Assert-Qa4PrivatePath $part }
    }
}
function Read-Qa4File([string]$Path,[long]$Expected,[long]$Limit) {
    $h=$script:qa4Native::CreateFileW($Path,[uint32]2147483776,1,[IntPtr]::Zero,3,0x00200000,[IntPtr]::Zero)
    if ($h -eq [IntPtr](-1)) { throw 'file_open_failed' }
    [void]$script:qa4Held.Add($h)
    if (((Get-Qa4Attributes $h) -band 0x410) -ne 0) { throw 'file_rejected' }
    Assert-Qa4PrivatePath $Path
    $stream=[IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
    try {
        if ($stream.Length -lt 0 -or $stream.Length -gt $Limit -or ($Expected -ge 0 -and $stream.Length -ne $Expected)) { throw 'file_size_rejected' }
        $bytes=New-Object byte[] ([int]$stream.Length);$position=0
        while ($position -lt $bytes.Length) { $n=$stream.Read($bytes,$position,$bytes.Length-$position);if ($n -le 0) { throw 'file_size_rejected' };$position+=$n }
        return ,$bytes
    } finally { $stream.Dispose() }
}
function Assert-Qa4Name([string]$Name) {
    if (-not $Name -or $Name -cne $Name.Trim() -or $Name.StartsWith('/') -or
        $Name.Contains('\') -or $Name.Contains(':')) { throw 'member_name_rejected' }
    foreach ($part in $Name.Split([char]'/')) {
        if (-not $part.Length -or $part -in @('.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'member_name_rejected' }
        foreach ($ch in $part.ToCharArray()) { if ([int]$ch -lt 32) { throw 'member_name_rejected' } }
        if ($part.Split([char]'.')[0] -match '^(?i:CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])$') { throw 'member_name_rejected' }
    }
}
function Read-Qa4Manifest([byte[]]$Bytes) {
    if ($Bytes.Length -lt 1 -or $Bytes.Length -gt 4194304) { throw 'manifest_rejected' }
    $text=[Text.UTF8Encoding]::new($false,$true).GetString($Bytes).TrimEnd([char[]]@("`r","`n"))
    $hashes=[Collections.Generic.Dictionary[string,string]]::new([StringComparer]::Ordinal)
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($line in [regex]::Split($text,'\r?\n')) {
        $match=[regex]::Match($line,'\A([a-f0-9]{64})  (.+)\z')
        if (-not $match.Success) { throw 'manifest_rejected' }
        $name=$match.Groups[2].Value;Assert-Qa4Name $name
        if ($name -ieq 'SHA256SUMS' -or -not $seen.Add($name)) { throw 'manifest_rejected' }
        $hashes.Add($name,$match.Groups[1].Value)
    }
    if ($hashes.Count -ne 406 -or -not $hashes.ContainsKey('ULTRA_PACKAGE.json')) { throw 'manifest_rejected' }
    foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) {
        if (-not $hashes.ContainsKey($name) -or $hashes[$name] -cne $script:qa4Hashes[$name]) { throw 'manifest_rejected' }
    }
    return ,$hashes
}
function Read-Qa4ArchiveEntries([byte[]]$ZipBytes) {
    Add-Type -AssemblyName System.IO.Compression
    $input=[IO.MemoryStream]::new($ZipBytes,$false);$archive=$null
    $payload=[Collections.Generic.Dictionary[string,byte[]]]::new([StringComparer]::Ordinal)
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase);$total=0L;$directories=0
    try {
        $archive=[IO.Compression.ZipArchive]::new($input,[IO.Compression.ZipArchiveMode]::Read,$false)
        if ($archive.Entries.Count -ne 408) { throw 'zip_members_rejected' }
        foreach ($entry in $archive.Entries) {
            $raw=$entry.FullName
            $name=$raw.Replace('\','/')
            if (-not $name.StartsWith($script:qa4Prefix,[StringComparison]::Ordinal)) { throw 'zip_members_rejected' }
            if ($name.EndsWith('/',[StringComparison]::Ordinal)) {
                $relativeDirectory=$name.Substring($script:qa4Prefix.Length).TrimEnd('/')
                if ($relativeDirectory -and $relativeDirectory.EndsWith('/',[StringComparison]::Ordinal)) { throw 'zip_members_rejected' }
                if ($relativeDirectory) { Assert-Qa4Name $relativeDirectory }
                $directories++
                if (($entry.ExternalAttributes -band 0x410) -ne 0) { throw 'zip_members_rejected' }
                continue
            }
            $relative=$name.Substring($script:qa4Prefix.Length);Assert-Qa4Name $relative
            $attributes=[long]$entry.ExternalAttributes;$mode=($attributes -shr 16) -band 0xf000
            if (-not $seen.Add($relative) -or ($attributes -band 0x410) -ne 0 -or $mode -notin @(0,0x8000)) { throw 'zip_members_rejected' }
            if ($entry.Length -lt 0 -or $entry.Length -gt 134217728 -or $entry.CompressedLength -gt 67108864 -or ($total+$entry.Length) -gt 134217728) { throw 'zip_size_rejected' }
            $total+=$entry.Length;$stream=$entry.Open();$out=[IO.MemoryStream]::new()
            try {
                $buffer=New-Object byte[] 65536
                while (($n=$stream.Read($buffer,0,$buffer.Length)) -gt 0) { if ($out.Length+$n -gt $entry.Length) { throw 'zip_size_rejected' };$out.Write($buffer,0,$n) }
                if ($out.Length -ne $entry.Length) { throw 'zip_size_rejected' }
                $payload.Add($relative,$out.ToArray())
            } finally { $stream.Dispose();$out.Dispose() }
        }
    } finally { if ($null -ne $archive) { $archive.Dispose() };$input.Dispose() }
    if ($payload.Count -ne 407) { throw 'zip_members_rejected' }
    return ,$payload
}
function Read-Qa4Bundle([byte[]]$ZipBytes) {
    if ($ZipBytes.Length -ne $script:qa4ZipLength -or (Get-Qa4Hash $ZipBytes) -cne $script:qa4ZipHash) { throw 'zip_hash_rejected' }
    $payload=Read-Qa4ArchiveEntries $ZipBytes
    if (-not $payload.ContainsKey('SHA256SUMS') -or (Get-Qa4Hash $payload['SHA256SUMS']) -cne $script:qa4ManifestHash) { throw 'manifest_hash_rejected' }
    $hashes=Read-Qa4Manifest $payload['SHA256SUMS']
    $expected=[Collections.Generic.Dictionary[string,string]]::new($hashes,[StringComparer]::Ordinal)
    $expected.Add('SHA256SUMS',$script:qa4ManifestHash)
    if ($expected.Count -ne 407) { throw 'manifest_rejected' }
    foreach ($name in $expected.Keys) {
        if (-not $payload.ContainsKey($name) -or (Get-Qa4Hash $payload[$name]) -cne $expected[$name]) { throw 'payload_hash_rejected' }
    }
    $descriptor=[Text.UTF8Encoding]::new($false,$true).GetString($payload['ULTRA_PACKAGE.json']).TrimStart([char]0xfeff) | ConvertFrom-Json
    if ($descriptor.product -cne 'GodUseVPN Ultra' -or $descriptor.namespace -cne 'godusevpn-ultra' -or
        $descriptor.version -cne '0.01-beta-qa.4' -or $descriptor.architecture -cne 'amd64' -or
        $descriptor.unsigned -isnot [bool] -or -not $descriptor.unsigned) { throw 'descriptor_rejected' }
    return [pscustomobject]@{payload=$payload;hashes=$hashes}
}
function New-Qa4Parents([string]$Root,[string]$Relative,[string]$PrivateRoot,$Created) {
    $parts=$Relative.Split([char]'/');$current=$Root
    for ($i=0;$i -lt $parts.Count-1;$i++) {
        $current=[IO.Path]::Combine($current,$parts[$i])
        if (-not $Created.Contains($current)) {
            if (-not $script:qa4Native::CreateDirectoryW($current,[IntPtr]::Zero)) { throw 'directory_create_failed' }
            Lock-Qa4Directories $current $PrivateRoot;[void]$Created.Add($current)
        }
    }
}
function Confirm-Qa4Inventory([string]$Root,$Expected) {
    $found=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $pending=[Collections.Generic.Queue[string]]::new();$pending.Enqueue($Root)
    while ($pending.Count -gt 0) {
        foreach ($item in @(Get-ChildItem -LiteralPath $pending.Dequeue() -Force)) {
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'inventory_rejected' }
            if ($item.PSIsContainer) { $pending.Enqueue($item.FullName);continue }
            $name=$item.FullName.Substring($Root.Length+1).Replace('\','/')
            if (-not $Expected.ContainsKey($name) -or -not $found.Add($name)) { throw 'inventory_rejected' }
            $bytes=Read-Qa4File $item.FullName -1 134217728
            if ((Get-Qa4Hash $bytes) -cne $Expected[$name]) { throw 'target_hash_rejected' }
        }
    }
    if ($found.Count -ne 407 -or $found.Count -ne $Expected.Count) { throw 'inventory_rejected' }
}

$result=[ordered]@{schema='ultra-qa4-expand-v1';ready=$false;stage='preflight';error='';files=0;hashesMatched=0;sha256Match=$false;privateChecked=$false;productExecuted=$false;installerExecuted=$false}
$script:qa4Held=New-Object 'Collections.Generic.List[IntPtr]';$exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or
        -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'host_rejected' }
    Assert-Qa4ID $StageID
    $script:qa4Native=New-Qa4Native
    $known=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData,[Environment+SpecialFolderOption]::DoNotVerify)
    $qaRoot=[IO.Path]::Combine($known,'GodUseVPN-Ultra-QA');$stage=[IO.Path]::Combine($qaRoot,$StageID)
    $preview=[IO.Path]::Combine($stage,'preview.zip');$output=[IO.Path]::Combine($stage,'package-beta-qa4')
    Lock-Qa4Directories $stage $qaRoot
    if ([IO.File]::Exists($output) -or [IO.Directory]::Exists($output)) { throw 'output_exists' }
    $result.stage='zip';$bundle=Read-Qa4Bundle (Read-Qa4File $preview $script:qa4ZipLength $script:qa4ZipLength)
    $result.stage='create'
    if (-not $script:qa4Native::CreateDirectoryW($output,[IntPtr]::Zero)) { throw 'directory_create_failed' }
    Lock-Qa4Directories $output $qaRoot
    $created=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase);[void]$created.Add($output)
    foreach ($name in @($bundle.payload.Keys | Sort-Object)) {
        New-Qa4Parents $output $name $qaRoot $created
        $path=[IO.Path]::Combine($output,$name.Replace('/','\'));$bytes=$bundle.payload[$name]
        $stream=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try { $stream.Write($bytes,0,$bytes.Length);$stream.Flush($true) } finally { $stream.Dispose() }
        $actual=Read-Qa4File $path $bytes.Length 134217728
        if ((Get-Qa4Hash $actual) -cne (Get-Qa4Hash $bytes)) { throw 'target_hash_rejected' }
    }
    $result.stage='verify';$expected=[Collections.Generic.Dictionary[string,string]]::new($bundle.hashes,[StringComparer]::Ordinal);$expected.Add('SHA256SUMS',$script:qa4ManifestHash)
    Confirm-Qa4Inventory $output $expected
    foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) {
        if ([Diagnostics.FileVersionInfo]::GetVersionInfo([IO.Path]::Combine($output,$name)).ProductVersion -cne '0.01-beta-qa.4') { throw 'static_version_rejected' }
        $result.hashesMatched++
    }
    $result.ready=$true;$result.files=407;$result.sha256Match=$true;$result.privateChecked=$true;$result.stage='complete';$exitCode=0
} catch {
    $knownErrors=@('host_rejected','stage_rejected','path_rejected','drive_rejected','handle_rejected','private_acl_rejected','directory_open_failed','directory_rejected','file_open_failed','file_rejected','file_size_rejected','member_name_rejected','manifest_rejected','zip_members_rejected','zip_size_rejected','zip_hash_rejected','manifest_hash_rejected','payload_hash_rejected','descriptor_rejected','directory_create_failed','inventory_rejected','target_hash_rejected','output_exists','static_version_rejected')
    $code=$_.Exception.Message;if ($knownErrors -cnotcontains $code) { $code='check_failed' };$result.error=$code
} finally {
    for ($i=$script:qa4Held.Count-1;$i -ge 0;$i--) { [void]$script:qa4Native::CloseHandle($script:qa4Held[$i]) }
}
$json=$result | ConvertTo-Json -Compress -Depth 4
if ($json.Length -gt 2000) { $json='{"schema":"ultra-qa4-expand-v1","ready":false,"error":"summary_limit","productExecuted":false}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode
