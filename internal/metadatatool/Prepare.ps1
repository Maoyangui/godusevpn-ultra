# Build-time dependency preparation only. Never runs ExifTool, Perl or installers.
[CmdletBinding()]
param([string]$Archive = '')
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$project = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$buildRoot = Join-Path $project '.codex/tmp/metadata-tool-build'
$output = Join-Path $buildRoot 'exiftool-13.59'
$publisher = 'https://oliverbetz.de/cms/files/Artikel/ExifTool-for-Windows/exiftool-13.59_64.zip'
$archiveHash = '577257dd22baebe77157d905792d6ed2c5916cd03aa627f40b2175db12110ac6'
$manifestHash = 'd1316fddf986e2d17761f85034c753b28375f25bf73235c1084a7c5a38e22faf'
$stage = Join-Path $buildRoot ('.stage-' + [Guid]::NewGuid().ToString('N'))
function Hash-Stream([IO.Stream]$Stream) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($sha.ComputeHash($Stream))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
function Hash-File([string]$Path) {
    $stream = [IO.File]::Open($Path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    try { return Hash-Stream $stream } finally { $stream.Dispose() }
}
$manifestPath = Join-Path $PSScriptRoot 'bundle.sha256'
if ((Hash-File $manifestPath) -cne $manifestHash) { throw 'Embedded dependency manifest changed; review the pinned upstream before updating it.' }
$expected = @{}
$directories = @{ '' = $true }
foreach ($line in [IO.File]::ReadAllLines($manifestPath)) {
    if ($line -notmatch '^([a-f0-9]{64})  ([^\\:\x00]+)$') { throw 'Invalid dependency manifest line.' }
    $hash = $Matches[1]; $relative = $Matches[2]
    foreach ($part in $relative.Split('/')) { if (-not $part -or $part -eq '.' -or $part -eq '..' -or $part.EndsWith('.') -or $part.EndsWith(' ')) { throw 'Unsafe dependency name.' } }
    if ($relative.StartsWith('/') -or $expected.ContainsKey($relative)) { throw 'Duplicate/absolute dependency name.' }
    $expected[$relative] = $hash
    $parent = $relative
    while ($parent.Contains('/')) { $parent = $parent.Substring(0, $parent.LastIndexOf('/')); $directories[$parent] = $true }
}
if ($expected.Count -ne 1501) { throw 'Incomplete dependency manifest.' }
function Test-Prepared([string]$Root) {
    if (-not [IO.Directory]::Exists($Root)) { return $false }
    if (([IO.File]::GetAttributes($Root) -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Prepared root is a reparse point.' }
    $count = 0
    foreach ($entry in Get-ChildItem -LiteralPath $Root -Recurse -Force) {
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Prepared dependencies contain a reparse point.' }
        $relative = $entry.FullName.Substring($Root.Length + 1).Replace('\', '/')
        if ($entry.PSIsContainer) { if (-not $directories.ContainsKey($relative)) { throw 'Unlisted dependency directory.' } }
        else {
            if (-not $expected.ContainsKey($relative) -or (Hash-File $entry.FullName) -cne $expected[$relative]) { throw 'Prepared dependency checksum mismatch.' }
            $count++
        }
    }
    if ($count -ne 1501) { throw 'Prepared dependency files are missing.' }
    return $true
}
[IO.Directory]::CreateDirectory($buildRoot) | Out-Null
if (Test-Prepared $output) {
    [pscustomobject]@{ status='verified-existing'; directory=$output; files=1501; version='13.59'; archiveSha256=$archiveHash; executed=$false } | ConvertTo-Json -Compress
    exit 0
}
if ([IO.File]::Exists($output)) { throw 'Dependency destination is a file; nothing overwritten.' }
if ([string]::IsNullOrWhiteSpace($Archive)) {
    $cache = Join-Path $project '.codex/tmp/metadata-tool-download'
    [IO.Directory]::CreateDirectory($cache) | Out-Null
    $Archive = Join-Path $cache 'exiftool-13.59_64.zip'
    if (-not [IO.File]::Exists($Archive)) { Invoke-WebRequest -Uri $publisher -UseBasicParsing -OutFile $Archive }
}
$Archive = [IO.Path]::GetFullPath($Archive)
$archiveStream = [IO.File]::Open($Archive, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
$zip = $null
try {
    if ((Hash-Stream $archiveStream) -cne $archiveHash) { throw 'Publisher archive SHA256 mismatch; no dependency was installed.' }
    $archiveStream.Position = 0
    $zip = [IO.Compression.ZipArchive]::new($archiveStream, [IO.Compression.ZipArchiveMode]::Read, $true)
    if ([IO.Directory]::Exists($stage) -or [IO.File]::Exists($stage)) { throw 'Fresh staging directory collision.' }
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    $seen = @{}; $total = [int64]0
    foreach ($entry in $zip.Entries) {
        $relative = $entry.FullName
        if ($relative.EndsWith('/')) {
            if (-not $directories.ContainsKey($relative.TrimEnd('/'))) { throw 'Unlisted ZIP directory.' }
            continue
        }
        if (-not $expected.ContainsKey($relative) -or $seen.ContainsKey($relative) -or $entry.Length -gt 33554432 -or (($entry.ExternalAttributes -shr 16) -band 61440) -eq 40960) { throw 'Unexpected/duplicate/link/oversized ZIP member.' }
        $total += $entry.Length
        if ($total -gt 268435456) { throw 'Dependency archive exceeds the extraction bound.' }
        $destination = [IO.Path]::GetFullPath((Join-Path $stage $relative.Replace('/', '\')))
        if (-not $destination.StartsWith($stage + '\', [StringComparison]::OrdinalIgnoreCase)) { throw 'ZIP traversal refused.' }
        [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($destination)) | Out-Null
        $read = $entry.Open(); $write = $null
        try {
            $write = [IO.File]::Open($destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
            $read.CopyTo($write)
            if ($write.Length -ne $entry.Length) { throw 'Truncated dependency member.' }
            $write.Position = 0
            if ((Hash-Stream $write) -cne $expected[$relative]) { throw 'Dependency member SHA256 mismatch.' }
        } finally { if ($null -ne $write) { $write.Dispose() }; $read.Dispose() }
        $seen[$relative] = $true
    }
    if ($seen.Count -ne 1501 -or -not (Test-Prepared $stage)) { throw 'Incomplete dependency extraction.' }
    # Atomic publish refuses a pre-existing destination; never merge/overwrite.
    [IO.Directory]::Move($stage, $output)
    [pscustomobject]@{ status='prepared'; directory=$output; files=1501; version='13.59'; archiveSha256=$archiveHash; executed=$false } | ConvertTo-Json -Compress
} finally {
    if ($null -ne $zip) { $zip.Dispose() }
    $archiveStream.Dispose()
    $resolvedStage = [IO.Path]::GetFullPath($stage)
    $resolvedRoot = [IO.Path]::GetFullPath($buildRoot)
    if (-not $resolvedStage.StartsWith($resolvedRoot + '\', [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolvedStage) -notmatch '^\.stage-[a-f0-9]{32}$') { throw 'Unsafe dependency staging cleanup target.' }
    if ([IO.Directory]::Exists($resolvedStage)) { [IO.Directory]::Delete($resolvedStage, $true) }
}
