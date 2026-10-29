# Receives the fixed QA4 preview ZIP in one SYSTEM Run Command invocation.
# The caller replaces __STAGE__, __MANIFEST__ and __NONCE__ in this template,
# or passes the same values through the explicit parameters. The manifest is
# UTF-8 JSON encoded as base64 and contains the exact, short-lived gist URLs.
# This script only stages bytes under the fixed ProgramData QA directory. It
# never extracts, installs, starts or executes the product.
#
# Expected package:
#   SHA256 fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144
#   Size   37933559 bytes
#   Parts  2 MiB except the final part

[CmdletBinding()]
param(
    [string]$StageID = '__STAGE__',
    [string]$ManifestBase64 = '__MANIFEST__',
    [string]$Nonce = '__NONCE__'
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$script:ReceiveFixedHash = 'fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144'
$script:ReceiveFixedSize = [long]37933559
$script:ReceiveChunkBytes = 2097152
$script:ReceiveMaxParts = 256
$script:ReceiveMaxManifestBytes = 131072
$script:ReceiveMarker = 'ULTRA_RECEIVE_V1:'

function Reply-Receive([bool]$OK, [string]$Status, [int]$Count, [long]$Bytes) {
    $value = [ordered]@{
        v = 1
        ok = $OK
        status = $Status
        parts = $Count
        bytes = $Bytes
        sha256Match = $OK
    }
    [Console]::Out.WriteLine($script:ReceiveMarker + $Nonce + ':' + ($value | ConvertTo-Json -Compress))
}

function Assert-ReceiveStageID([string]$Value) {
    if ($Value -cnotmatch '^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$') { throw 'invalid_stage' }
}

function Get-ReceiveHash([byte[]]$Bytes) {
    $hash = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($hash.ComputeHash($Bytes))).Replace('-', '').ToLowerInvariant() }
    finally { $hash.Dispose() }
}

function Assert-ReceiveNoReparse([string]$Path) {
    $current = [IO.Path]::GetFullPath($Path)
    while ($current) {
        $item = Get-Item -LiteralPath $current -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'reparse_rejected'
        }
        $parent = [IO.Path]::GetDirectoryName($current.TrimEnd('\'))
        if (-not $parent -or $parent -eq $current) { break }
        $current = $parent
    }
}

function Assert-ReceivePrivate([string]$Path, [bool]$Directory) {
    Assert-ReceiveNoReparse $Path
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ([bool]$item.PSIsContainer -ne $Directory) { throw 'wrong_object_type' }
    $acl = Get-Acl -LiteralPath $Path -ErrorAction Stop
    $owner = $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value
    if ($owner -notin @('S-1-5-18', 'S-1-5-32-544')) { throw 'untrusted_owner' }
    if ($Directory -and -not $acl.AreAccessRulesProtected) { throw 'directory_acl_not_private' }
    $write = [Security.AccessControl.FileSystemRights]::WriteData -bor
        [Security.AccessControl.FileSystemRights]::AppendData -bor
        [Security.AccessControl.FileSystemRights]::WriteExtendedAttributes -bor
        [Security.AccessControl.FileSystemRights]::WriteAttributes -bor
        [Security.AccessControl.FileSystemRights]::Delete -bor
        [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor
        [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
        [Security.AccessControl.FileSystemRights]::TakeOwnership
    foreach ($rule in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
        if ($rule.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and
            ($rule.FileSystemRights -band $write) -ne 0 -and
            $rule.IdentityReference.Value -notin @('S-1-5-18', 'S-1-5-32-544')) {
            throw 'untrusted_write_acl'
        }
    }
}

function Ensure-ReceiveDirectory([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        $acl = New-Object Security.AccessControl.DirectorySecurity
        $acl.SetSecurityDescriptorSddlForm('O:SYG:SYD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)')
        $null = [IO.Directory]::CreateDirectory($Path, $acl)
    }
    Assert-ReceivePrivate $Path $true
}

function Open-ReceiveVerified([string]$Path, [long]$Size, [string]$Hash) {
    Assert-ReceivePrivate $Path $false
    $stream = [IO.File]::Open($Path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    try {
        if ($stream.Length -ne $Size) { throw 'existing_file_size_conflict' }
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $actual = ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-', '').ToLowerInvariant() }
        finally { $sha.Dispose() }
        if ($actual -cne $Hash) { throw 'existing_file_hash_conflict' }
        $stream.Position = 0
        return ,$stream
    } catch {
        $stream.Dispose()
        throw
    }
}

function Write-ReceiveNew([string]$Path, [byte[]]$Bytes) {
    $stream = [IO.File]::Open($Path, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    try {
        $stream.Write($Bytes, 0, $Bytes.Length)
        $stream.Flush($true)
    } finally {
        $stream.Dispose()
    }
}

function Test-ReceiveGistUrl([string]$Url) {
    # No query, fragment, userinfo, alternate host, redirect target or path
    # is accepted. The owner and gist id are deliberately case-sensitive.
    return [regex]::IsMatch(
        $Url,
        '^https://gist\.githubusercontent\.com/Maoyangui/[0-9a-f]{32}/raw/[A-Za-z0-9][A-Za-z0-9._-]{0,127}$',
        [Text.RegularExpressions.RegexOptions]::CultureInvariant)
}

function Assert-ReceiveManifest([string]$Base64) {
    if ([string]::IsNullOrWhiteSpace($Base64) -or $Base64.Length -gt 200000) {
        throw 'invalid_manifest'
    }
    try { $bytes = [Convert]::FromBase64String($Base64) }
    catch { throw 'invalid_manifest' }
    if ($bytes.Length -lt 1 -or $bytes.Length -gt $script:ReceiveMaxManifestBytes) {
        throw 'invalid_manifest'
    }
    try {
        $text = [Text.UTF8Encoding]::new($false, $true).GetString($bytes)
        $manifest = $text | ConvertFrom-Json
    } catch { throw 'invalid_manifest' }
    $required = @('version', 'kind', 'sha256', 'size', 'chunkBytes', 'parts')
    $actual = @($manifest.PSObject.Properties.Name | Sort-Object)
    if (($actual -join '|') -cne (($required | Sort-Object) -join '|') -or
        $manifest.version -ne 1 -or $manifest.kind -cne 'GodUseVPN-Ultra-QA-stage' -or
        $manifest.sha256 -cne $script:ReceiveFixedHash -or
        $manifest.size -ne $script:ReceiveFixedSize -or
        $manifest.chunkBytes -ne $script:ReceiveChunkBytes) {
        throw 'invalid_manifest'
    }
    $parts = @($manifest.parts)
    if ($parts.Count -lt 1 -or $parts.Count -gt $script:ReceiveMaxParts) { throw 'invalid_manifest' }
    $sum = [long]0
    $urls = New-Object 'Collections.Generic.HashSet[string]' ([StringComparer]::Ordinal)
    for ($index = 0; $index -lt $parts.Count; $index++) {
        $part = $parts[$index]
        $keys = @($part.PSObject.Properties.Name | Sort-Object)
        if (($keys -join '|') -cne 'sha256|size|url' -or
            $part.size -isnot [int] -and $part.size -isnot [long] -or
            $part.size -lt 1 -or $part.size -gt $script:ReceiveChunkBytes -or
            ($index -lt $parts.Count - 1 -and $part.size -ne $script:ReceiveChunkBytes) -or
            ($index -eq $parts.Count - 1 -and $part.size -ne 184823) -or
            $part.sha256 -isnot [string] -or $part.sha256 -cnotmatch '^[a-f0-9]{64}$' -or
            $part.url -isnot [string] -or -not (Test-ReceiveGistUrl $part.url) -or
            -not $urls.Add($part.url)) {
            throw 'invalid_manifest_part'
        }
        $sum += [long]$part.size
    }
    if ($sum -ne $script:ReceiveFixedSize) { throw 'invalid_manifest_size' }
    return [pscustomobject]@{ Bytes = $bytes; Manifest = $manifest; Parts = $parts }
}

function Receive-ReceivePart([string]$Url, [long]$ExpectedSize, [string]$ExpectedHash) {
    if (-not (Test-ReceiveGistUrl $Url)) { throw 'invalid_manifest_part' }
    $request = [Net.HttpWebRequest]::Create($Url)
    $request.Method = 'GET'
    $request.AllowAutoRedirect = $false
    $request.UseDefaultCredentials = $false
    $request.PreAuthenticate = $false
    $request.Timeout = 120000
    $request.ReadWriteTimeout = 120000
    $request.UserAgent = 'GodUseVPN-Ultra-QA/1'
    try {
        $response = $request.GetResponse()
        try {
            if ($response.StatusCode -ne [Net.HttpStatusCode]::OK) { throw 'download_http_status' }
            if ($response.ContentLength -ge 0 -and $response.ContentLength -ne $ExpectedSize) {
                throw 'download_length_mismatch'
            }
            $input = $response.GetResponseStream()
            $memory = New-Object IO.MemoryStream
            try {
                $buffer = New-Object byte[] 65536
                while (($read = $input.Read($buffer, 0, $buffer.Length)) -gt 0) {
                    if ($memory.Length + $read -gt $ExpectedSize) { throw 'download_length_mismatch' }
                    $memory.Write($buffer, 0, $read)
                }
                $bytes = $memory.ToArray()
            } finally {
                $memory.Dispose()
                $input.Dispose()
            }
        } finally {
            $response.Dispose()
        }
    } catch [Net.WebException] {
        throw 'download_failed'
    }
    if ($bytes.Length -ne $ExpectedSize -or (Get-ReceiveHash $bytes) -cne $ExpectedHash) {
        throw 'download_hash_mismatch'
    }
    return ,$bytes
}

function Invoke-ReceivePackage {
    Assert-ReceiveStageID $StageID
    if ([Security.Principal.WindowsIdentity]::GetCurrent().User.Value -ne 'S-1-5-18') {
        throw 'system_run_command_required'
    }
    $validated = Assert-ReceiveManifest $ManifestBase64
    $programData = [Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData)
    $driveRoot = [IO.Path]::GetPathRoot($programData)
    if ($driveRoot.Length -ne 3 -or -not [IO.Directory]::Exists($programData) -or
        ([IO.DriveInfo]::new($driveRoot)).DriveType -ne [IO.DriveType]::Fixed) {
        throw 'invalid_programdata'
    }
    Assert-ReceiveNoReparse $programData
    $qaRoot = Join-Path $programData 'GodUseVPN-Ultra-QA'
    Ensure-ReceiveDirectory $qaRoot
    $stage = Join-Path $qaRoot $StageID
    Ensure-ReceiveDirectory $stage
    $manifestPath = Join-Path $stage 'stage.json'
    $manifestHash = Get-ReceiveHash $validated.Bytes
    if (Test-Path -LiteralPath $manifestPath) {
        $existingManifest = Open-ReceiveVerified $manifestPath $validated.Bytes.Length $manifestHash
        $existingManifest.Dispose()
    } else {
        Write-ReceiveNew $manifestPath $validated.Bytes
    }
    for ($index = 0; $index -lt $validated.Parts.Count; $index++) {
        $part = $validated.Parts[$index]
        $partPath = Join-Path $stage ('part-{0:D4}.bin' -f $index)
        if (Test-Path -LiteralPath $partPath) {
            $existing = Open-ReceiveVerified $partPath $part.size $part.sha256
            $existing.Dispose()
            continue
        }
        $bytes = Receive-ReceivePart $part.url $part.size $part.sha256
        Write-ReceiveNew $partPath $bytes
        $verified = Open-ReceiveVerified $partPath $part.size $part.sha256
        $verified.Dispose()
    }
    $zipPath = Join-Path $stage 'preview.zip'
    if (Test-Path -LiteralPath $zipPath) {
        $existingZip = Open-ReceiveVerified $zipPath $script:ReceiveFixedSize $script:ReceiveFixedHash
        $existingZip.Dispose()
        Reply-Receive $true 'present' $validated.Parts.Count $script:ReceiveFixedSize
        return
    }
    $mergePath = Join-Path $stage ('merge-{0}.tmp' -f ([Guid]::NewGuid().ToString('N')))
    $output = [IO.File]::Open($mergePath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    try {
        for ($index = 0; $index -lt $validated.Parts.Count; $index++) {
            $part = $validated.Parts[$index]
            $input = Open-ReceiveVerified (Join-Path $stage ('part-{0:D4}.bin' -f $index)) $part.size $part.sha256
            try { $input.CopyTo($output, 1048576) } finally { $input.Dispose() }
        }
        $output.Flush($true)
    } finally {
        $output.Dispose()
    }
    $merged = Open-ReceiveVerified $mergePath $script:ReceiveFixedSize $script:ReceiveFixedHash
    $merged.Dispose()
    [IO.File]::Move($mergePath, $zipPath)
    $final = Open-ReceiveVerified $zipPath $script:ReceiveFixedSize $script:ReceiveFixedHash
    $final.Dispose()
    Reply-Receive $true 'merged' $validated.Parts.Count $script:ReceiveFixedSize
}

try {
    if ($Nonce -cnotmatch '^[a-f0-9]{32}$') { throw 'invalid_nonce' }
    Invoke-ReceivePackage
} catch {
    $status = [string]$_.Exception.Message
    $known = @(
        'invalid_stage', 'invalid_nonce', 'invalid_manifest', 'invalid_manifest_part',
        'invalid_manifest_size', 'system_run_command_required', 'invalid_programdata',
        'reparse_rejected', 'wrong_object_type', 'untrusted_owner',
        'directory_acl_not_private', 'untrusted_write_acl', 'existing_file_size_conflict',
        'existing_file_hash_conflict', 'download_http_status', 'download_length_mismatch',
        'download_failed', 'download_hash_mismatch'
    )
    if ($known -notcontains $status) { $status = 'receive_failed' }
    Reply-Receive $false $status 0 0
    exit 1
}
