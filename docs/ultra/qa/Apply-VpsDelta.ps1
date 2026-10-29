# Windows PowerShell 5.1 x64. QA file reconstruction ONLY; never runs a product.
# Supply explicit local directories and the independently approved manifest hash.
# OutputDirectory must be a NEW delta-<32 lowercase hex> child of an existing QA
# staging directory. No overwrite/resume/delete/ACL takeover is implemented.
# All three results are verified in memory before creating output; partial output
# on an I/O failure is retained for manual review. Use a fresh output ID afterward.
# Existing staging ancestors must be trusted; no defense against a hostile local
# account/administrator changing namespaces concurrently is claimed here.
# Builder emits a single gzip member with no optional header. Explicit footer
# ISIZE/CRC rejects ordinary truncation/trailer damage (PS5 GZipStream may not).
# It does not independently certify all deflate/member boundary encodings: the
# approved compressed SHA and final EXE SHA remain the content trust anchors.
[CmdletBinding()]
param(
    [string]$BaseDirectory,
    [string]$DeltaDirectory,
    [string]$OutputDirectory,
    [string]$ExpectedManifestSha256
)
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$WarningPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$VerbosePreference = 'SilentlyContinue'
$DebugPreference = 'SilentlyContinue'
$maxFile = 134217728L
$maxTotal = 402653184L
$names = @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')

function Stop-Delta([string]$Code) { throw [InvalidOperationException]::new($Code) }
function Assert-Local([string]$Path, [bool]$Directory) {
    if (-not $Path -or $Path -notmatch '^[A-Za-z]:\\' -or $Path.Substring(2).Contains(':') -or $Path.Contains('/')) { Stop-Delta 'unsafe_path' }
    foreach ($component in $Path.Substring(3).TrimEnd('\').Split([char]'\')) {
        if ($component -eq '.' -or $component -eq '..' -or $component.EndsWith('.') -or $component.EndsWith(' ') -or $component.IndexOfAny([char[]]'<>"|?*') -ge 0) { Stop-Delta 'unsafe_path' }
    }
    $full = [IO.Path]::GetFullPath($Path)
    # .NET Framework may expand a valid TEMP 8.3 ancestor to its long name.
    # Reject ambiguous components before normalization, then verify that path.
    $root = [IO.Path]::GetPathRoot($full)
    if ($full.TrimEnd('\') -ieq $root.TrimEnd('\')) { Stop-Delta 'unsafe_path' }
    if ((New-Object IO.DriveInfo($root)).DriveType -ne [IO.DriveType]::Fixed) { Stop-Delta 'unsafe_path' }
    $current = $root
    foreach ($part in @('') + $full.Substring($root.Length).TrimEnd('\').Split([char]'\')) {
        if ($part.Length) {
            if ($part -eq '.' -or $part -eq '..' -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { Stop-Delta 'unsafe_path' }
            $current = [IO.Path]::Combine($current,$part)
        }
        $attributes = [IO.File]::GetAttributes($current)
        if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { Stop-Delta 'unsafe_path' }
        if ($current -ne $full -and ($attributes -band [IO.FileAttributes]::Directory) -eq 0) { Stop-Delta 'unsafe_path' }
    }
    if ((($attributes -band [IO.FileAttributes]::Directory) -ne 0) -ne $Directory) { Stop-Delta 'unsafe_path' }
    return $full.TrimEnd('\')
}
function Read-Bounded([string]$Path,[long]$Expected,[long]$Limit) {
    $stream = [IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
    try {
        if ($stream.Length -lt 1 -or $stream.Length -gt $Limit -or ($Expected -ge 0 -and $stream.Length -ne $Expected)) { Stop-Delta 'input_size_mismatch' }
        $bytes = New-Object byte[] ([int]$stream.Length)
        $position=0
        while ($position -lt $bytes.Length) {
            $read=$stream.Read($bytes,$position,$bytes.Length-$position)
            if ($read -le 0) { Stop-Delta 'input_size_mismatch' }
            $position += $read
        }
        return ,$bytes
    } finally { $stream.Dispose() }
}
function Get-Hash([byte[]]$Bytes) {
    $h=[Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($h.ComputeHash($Bytes))).Replace('-','').ToLowerInvariant() }
    finally { $h.Dispose() }
}
function Assert-Keys($Value,[string[]]$Keys) {
    if ($null -eq $Value -or $Value -isnot [pscustomobject]) { Stop-Delta 'invalid_manifest' }
    $actual=@($Value.PSObject.Properties.Name)
    if ($actual.Count -ne $Keys.Count) { Stop-Delta 'invalid_manifest' }
    foreach ($key in $actual) { if ($Keys -cnotcontains $key) { Stop-Delta 'invalid_manifest' } }
}
function Assert-Integer($Value,[long]$Low,[long]$High) {
    if (($Value -isnot [int] -and $Value -isnot [long]) -or $Value -lt $Low -or $Value -gt $High) { Stop-Delta 'invalid_manifest' }
}
function Expand-Patch([byte[]]$Compressed,[int]$Expected) {
    if ($Compressed.Length -lt 18 -or $Compressed[0] -ne 31 -or $Compressed[1] -ne 139 -or $Compressed[2] -ne 8 -or $Compressed[3] -ne 0) { Stop-Delta 'gzip_header_mismatch' }
    if ([BitConverter]::ToUInt32($Compressed,$Compressed.Length-4) -ne $Expected) { Stop-Delta 'gzip_footer_mismatch' }
    $expectedCrc=[BitConverter]::ToUInt32($Compressed,$Compressed.Length-8)
    $source=[IO.MemoryStream]::new($Compressed,$false)
    $gzip=$null; $output=[IO.MemoryStream]::new()
    try {
        $gzip=[IO.Compression.GZipStream]::new($source,[IO.Compression.CompressionMode]::Decompress,$false)
        $buffer=New-Object byte[] 65536
        while (($read=$gzip.Read($buffer,0,$buffer.Length)) -gt 0) {
            if ($output.Length+$read -gt $Expected) { Stop-Delta 'encoded_length_mismatch' }
            $output.Write($buffer,0,$read)
        }
        if ($output.Length -ne $Expected) { Stop-Delta 'encoded_length_mismatch' }
        $bytes=$output.ToArray()
        if ([UltraQaDeltaDirectory]::Crc32($bytes) -ne $expectedCrc) { Stop-Delta 'gzip_footer_mismatch' }
        return ,$bytes
    } catch {
        if ($_.Exception.Message -in @('encoded_length_mismatch','gzip_footer_mismatch')) { throw }
        Stop-Delta 'gzip_decode_failed'
    } finally { if ($null -ne $gzip) { $gzip.Dispose() };$source.Dispose();$output.Dispose() }
}
function Decode-Patch([byte[]]$Base,[byte[]]$Encoded,[int]$TargetLength) {
    if ($Encoded.Length -lt 13 -or [Text.Encoding]::ASCII.GetString($Encoded,0,8) -cne 'GUVDEL01' -or [BitConverter]::ToUInt32($Encoded,8) -ne $TargetLength) { Stop-Delta 'invalid_header' }
    $result=New-Object byte[] $TargetLength
    [long]$position=12;[long]$written=0;[long]$commands=0
    while ($position -lt $Encoded.Length) {
        $op=$Encoded[$position];$position++
        if ($op -eq 255) {
            if ($position -ne $Encoded.Length -or $written -ne $TargetLength) { Stop-Delta 'target_length_mismatch' }
            return ,$result
        }
        $commands++;if ($commands -gt 2000000) { Stop-Delta 'command_limit' }
        if ($op -eq 0) {
            if ($position+8 -gt $Encoded.Length) { Stop-Delta 'truncated_command' }
            [long]$offset=[BitConverter]::ToUInt32($Encoded,[int]$position);$position+=4
            [long]$length=[BitConverter]::ToUInt32($Encoded,[int]$position);$position+=4
            if ($length -lt 1 -or $offset -gt $Base.Length -or $length -gt $Base.Length-$offset -or $length -gt $TargetLength-$written) { Stop-Delta 'copy_bounds' }
            [Buffer]::BlockCopy($Base,[int]$offset,$result,[int]$written,[int]$length);$written+=$length
        } elseif ($op -eq 1) {
            if ($position+4 -gt $Encoded.Length) { Stop-Delta 'truncated_command' }
            [long]$length=[BitConverter]::ToUInt32($Encoded,[int]$position);$position+=4
            if ($length -lt 1 -or $length -gt $Encoded.Length-$position -or $length -gt $TargetLength-$written) { Stop-Delta 'literal_bounds' }
            [Buffer]::BlockCopy($Encoded,[int]$position,$result,[int]$written,[int]$length);$written+=$length;$position+=$length
        } else { Stop-Delta 'invalid_opcode' }
    }
    Stop-Delta 'missing_end'
}

try {
    if ([IntPtr]::Size -ne 8) { Stop-Delta 'x64_host_required' }
    if ($ExpectedManifestSha256 -cnotmatch '^[0-9a-f]{64}$') { Stop-Delta 'expected_manifest_hash_required' }
    $base=Assert-Local $BaseDirectory $true
    $delta=Assert-Local $DeltaDirectory $true
    if (-not $OutputDirectory -or [IO.Path]::GetFileName($OutputDirectory) -cnotmatch '^delta-[0-9a-f]{32}$') { Stop-Delta 'new_qa_directory_required' }
    $parent=Assert-Local ([IO.Path]::GetDirectoryName($OutputDirectory)) $true
    $output=[IO.Path]::Combine($parent,[IO.Path]::GetFileName($OutputDirectory))
    if ([IO.Directory]::Exists($output) -or [IO.File]::Exists($output)) { Stop-Delta 'output_exists' }
    $manifestPath=Assert-Local ([IO.Path]::Combine($delta,'manifest.json')) $false
    $manifestBytes=Read-Bounded $manifestPath -1 16384
    if ((Get-Hash $manifestBytes) -cne $ExpectedManifestSha256) { Stop-Delta 'manifest_hash_mismatch' }
    $manifest=[Text.UTF8Encoding]::new($false,$true).GetString($manifestBytes) | ConvertFrom-Json
    Assert-Keys $manifest @('schema','blockBytes','files')
    Assert-Integer $manifest.blockBytes 256 256
    if ($manifest.schema -cne 'godusevpn-ultra-exe-delta-v1' -or $manifest.blockBytes -ne 256 -or $manifest.files -isnot [array] -or $manifest.files.Count -ne 3) { Stop-Delta 'invalid_manifest' }
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    [long]$total=0
    foreach ($f in $manifest.files) {
        Assert-Keys $f @('name','baseSha256','newSha256','baseLength','newLength','patchFile','patchSha256','patchLength','encodedLength')
        if ($names -cnotcontains $f.name -or -not $seen.Add([string]$f.name) -or $f.patchFile -cne ($f.name+'.delta.gz')) { Stop-Delta 'invalid_manifest' }
        foreach ($field in @('baseSha256','newSha256','patchSha256')) { if ($f.$field -isnot [string] -or $f.$field -cnotmatch '^[0-9a-f]{64}$') { Stop-Delta 'invalid_manifest' } }
        Assert-Integer $f.baseLength 1 $maxFile
        Assert-Integer $f.newLength 1 $maxFile
        Assert-Integer $f.encodedLength 13 ($f.newLength*2L+32L)
        Assert-Integer $f.patchLength 1 ($f.encodedLength+1048576L)
        $total+=$f.newLength;if ($total -gt $maxTotal) { Stop-Delta 'invalid_manifest' }
    }
    # Fixed native mkdir and in-memory CRC only; no command/product invocation.
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class UltraQaDeltaDirectory {
    [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
    public static extern bool CreateDirectory(string path, IntPtr attributes);
    private static readonly uint[] Table = MakeTable();
    private static uint[] MakeTable() {
        uint[] table = new uint[256];
        for (uint i = 0; i < 256; i++) {
            uint c = i;
            for (int bit = 0; bit < 8; bit++) c = (c & 1) != 0 ? 0xedb88320U ^ (c >> 1) : c >> 1;
            table[i] = c;
        }
        return table;
    }
    public static uint Crc32(byte[] bytes) {
        uint crc = 0xffffffffU;
        foreach (byte b in bytes) crc = Table[(crc ^ b) & 255] ^ (crc >> 8);
        return crc ^ 0xffffffffU;
    }
}
'@
    $rebuilt=@{}
    foreach ($f in $manifest.files) {
        $basePath=[IO.Path]::Combine($base,$f.name)
        if (-not [IO.File]::Exists($basePath)) { Stop-Delta 'missing_base' }
        $basePath=Assert-Local $basePath $false
        $baseBytes=Read-Bounded $basePath $f.baseLength $maxFile
        if ((Get-Hash $baseBytes) -cne $f.baseSha256) { Stop-Delta 'base_hash_mismatch' }
        $patchPath=Assert-Local ([IO.Path]::Combine($delta,$f.patchFile)) $false
        $patch=Read-Bounded $patchPath $f.patchLength ($f.encodedLength+1048576L)
        if ((Get-Hash $patch) -cne $f.patchSha256) { Stop-Delta 'patch_hash_mismatch' }
        $encoded=Expand-Patch $patch $f.encodedLength
        $bytes=Decode-Patch $baseBytes $encoded $f.newLength
        if ((Get-Hash $bytes) -cne $f.newSha256) { Stop-Delta 'target_hash_mismatch' }
        $rebuilt[$f.name]=$bytes
    }
    # Atomic create rejects an output directory even if it appeared after the
    # earlier existence check. Only a kernel directory API, never a product call.
    if (-not [UltraQaDeltaDirectory]::CreateDirectory($output,[IntPtr]::Zero)) { Stop-Delta 'output_create_failed' }
    foreach ($name in $names) {
        $null=Assert-Local $output $true
        $destination=[IO.Path]::Combine($output,$name)
        $stream=[IO.File]::Open($destination,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try { $bytes=$rebuilt[$name];$stream.Write($bytes,0,$bytes.Length);$stream.Flush($true) }
        finally { $stream.Dispose() }
    }
    @{schema='godusevpn-ultra-delta-result-v1';ready=$true;files=3;sha256Match=$true;productExecuted=$false} | ConvertTo-Json -Compress
    exit 0
} catch {
    $known=@('unsafe_path','input_size_mismatch','invalid_manifest','encoded_length_mismatch','gzip_header_mismatch','gzip_footer_mismatch','gzip_decode_failed','invalid_header','target_length_mismatch','command_limit','truncated_command','copy_bounds','literal_bounds','invalid_opcode','missing_end','x64_host_required','expected_manifest_hash_required','new_qa_directory_required','output_exists','manifest_hash_mismatch','missing_base','base_hash_mismatch','patch_hash_mismatch','target_hash_mismatch','output_create_failed')
    $code=$_.Exception.Message
    if ($known -cnotcontains $code) { $code='delta_apply_failed' }
    @{schema='godusevpn-ultra-delta-result-v1';ready=$false;error=$code;productExecuted=$false} | ConvertTo-Json -Compress
    exit 1
}
