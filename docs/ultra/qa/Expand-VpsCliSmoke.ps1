# Windows PowerShell 5.1, 64-bit. Run explicitly as SYSTEM after the separate
# Stage-VpsPackage.py upload/merge has completed. No product is executed here.
# Only the fixed preview ZIP in ProgramData/GodUseVPN-Ultra-QA/<StageID> is read.
# All payloads are checked in memory against SHA256SUMS; only the CLI, manifest
# and package descriptor are written to a NEW private stage/cli-smoke directory.
# No GUI/service/installer/extension is extracted, installed, or executed.
#
# The OS ProgramData and its ancestors must already be trusted. This script
# checks local fixed-drive paths/reparse points and private QA/stage ACLs; it
# does not defend against a hostile administrator changing namespaces mid-run.
# Existing objects/ACLs are never repaired, replaced, deleted, or taken over.
# Failed/partial cli-smoke directories remain for deliberate operator review;
# another invocation rejects an existing output directory rather than resuming.
# Run Command may retain script/log data. Output has fixed status/counts only,
# no account, SID, host, subscription, IP/MAC, or absolute directory information.
#
# Example: powershell.exe -NoProfile -NonInteractive -File .\Expand-VpsCliSmoke.ps1 -StageID '<same UUID4 hex>'
# On smokeReady=true, separately run Vps-ReadOnlySmoke.ps1 with PackageDirectory
# = this same private stage's cli-smoke child and ExpectedCliSha256 = the fixed
# hash below. Do not run service version or privacy detection. The smoke observes
# the current SYSTEM token's HKCU/APPDATA, never the RDP/GUI user's profile.
[CmdletBinding()]
param([string]$StageID)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$WarningPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$VerbosePreference = 'SilentlyContinue'
$DebugPreference = 'SilentlyContinue'

$fixedZipHash = 'a8717578892160bf45576882cf242fce48ccb8759726878ff3c6a00f08cda746'
$fixedManifestHash = '16c41ad2296575cdf0cdcd46f3abfa6227cfc4601ea7acd842cd94b669cd0ad6'
$fixedCliHash = 'bf728ab1a000b4a8aae5329d7ba76cc25aae4c7a87667fecc0bfdf4d9f5ffaa1'
$zipPrefix = 'godusevpn-ultra-0.1.0-ultra.1-windows-amd64/'
$payloadNames = @(
    'godusevpn-cli.exe', 'godusevpn-svc.exe', 'godusevpn.exe', 'Install-Ultra.ps1',
    'LICENSE', 'NOTICE', 'PROJECT_PLAN.zh-CN.md', 'README-ULTRA.zh-CN.md',
    'ULTRA_PACKAGE.json', 'Uninstall-Ultra.ps1',
    'browser-extension/clean-link.js', 'browser-extension/core.js',
    'browser-extension/launcher.html', 'browser-extension/launcher.js',
    'browser-extension/manifest.json', 'browser-extension/panel.css',
    'browser-extension/panel.html', 'browser-extension/panel.js',
    'browser-extension/README.md', 'browser-extension/tests.cjs', 'browser-extension/ui-tests.cjs',
    'docs/ultra/browser-regression.txt', 'docs/ultra/build-results.json',
    'docs/ultra/desktop-regression.txt', 'docs/ultra/EXECUTION_LOG.md',
    'docs/ultra/FEATURE_STATUS.zh-CN.md', 'docs/ultra/go-regression.txt',
    'docs/ultra/smoke-results.json', 'docs/ultra/source-integrity.json',
    'docs/ultra/ui-dom-regression.txt'
)

function Assert-ExistingLocal([string]$Path, [bool]$Directory, [bool]$Private) {
    if ($Path -notmatch '^[A-Za-z]:\\' -or $Path.Substring(2).Contains(':') -or $Path.Contains('/')) {
        throw 'local_path_required'
    }
    $full = [IO.Path]::GetFullPath($Path)
    $root = [IO.Path]::GetPathRoot($full)
    if ((New-Object IO.DriveInfo($root)).DriveType -ne [IO.DriveType]::Fixed) { throw 'fixed_drive_required' }
    $current = $root
    $paths = @($root)
    foreach ($component in $full.Substring($root.Length).Split([char]'\')) {
        if (-not $component.Length) { continue }
        $current = [IO.Path]::Combine($current, $component)
        $paths += $current
    }
    foreach ($item in $paths) {
        $attributes = [IO.File]::GetAttributes($item)
        if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'reparse_rejected' }
        if ($item -ne $full -and ($attributes -band [IO.FileAttributes]::Directory) -eq 0) { throw 'invalid_parent' }
    }
    if ((($attributes -band [IO.FileAttributes]::Directory) -ne 0) -ne $Directory) { throw 'wrong_object_type' }
    if ($Private) {
        $acl = Get-Acl -LiteralPath $full -ErrorAction Stop
        $trusted = @('S-1-5-18', 'S-1-5-32-544')
        if ($acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -notin $trusted) { throw 'untrusted_owner' }
        if ($Directory -and -not $acl.AreAccessRulesProtected) { throw 'directory_acl_not_private' }
        $write = [Security.AccessControl.FileSystemRights]::Write -bor
            [Security.AccessControl.FileSystemRights]::Delete -bor
            [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor
            [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
            [Security.AccessControl.FileSystemRights]::TakeOwnership
        foreach ($rule in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
            if ($rule.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and
                ($rule.FileSystemRights -band $write) -ne 0 -and $rule.IdentityReference.Value -notin $trusted) {
                throw 'untrusted_write_acl'
            }
        }
    }
    return $full
}

function Get-StreamHash($Stream) {
    $hash = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($hash.ComputeHash($Stream))).Replace('-', '').ToLowerInvariant() }
    finally { $hash.Dispose() }
}

function Get-EntryRelativeName([string]$Name) {
    if (-not $Name.StartsWith($zipPrefix, [StringComparison]::Ordinal)) { throw 'unexpected_zip_prefix' }
    $relative = $Name.Substring($zipPrefix.Length)
    if (-not $relative.Length -or $relative -notmatch '^[A-Za-z0-9._/-]+$' -or $relative.StartsWith('/')) {
        throw 'invalid_zip_path'
    }
    foreach ($component in $relative.TrimEnd('/').Split([char]'/')) {
        if (-not $component.Length -or $component -eq '.' -or $component -eq '..' -or $component.EndsWith('.')) {
            throw 'invalid_zip_path'
        }
    }
    return $relative
}

function Read-SmallEntry($Entry, [int]$Maximum) {
    if ($Entry.Length -lt 1 -or $Entry.Length -gt $Maximum) { throw 'invalid_small_entry_size' }
    $inputStream = $Entry.Open()
    $memory = New-Object IO.MemoryStream
    try {
        $inputStream.CopyTo($memory, 4096)
        if ($memory.Length -ne $Entry.Length -or $memory.Length -gt $Maximum) { throw 'entry_length_mismatch' }
        return ,$memory.ToArray()
    } finally { $memory.Dispose(); $inputStream.Dispose() }
}

function Read-ChecksumManifest([byte[]]$Bytes) {
    $memory = New-Object IO.MemoryStream(,$Bytes)
    try { if ((Get-StreamHash $memory) -cne $fixedManifestHash) { throw 'manifest_hash_mismatch' } }
    finally { $memory.Dispose() }
    foreach ($value in $Bytes) { if ($value -gt 127) { throw 'non_ascii_manifest' } }
    $text = [Text.Encoding]::ASCII.GetString($Bytes)
    $checksums = @{}
    foreach ($line in $text.Split([char]10)) {
        $line = $line.TrimEnd([char]13)
        if (-not $line.Length) { continue }
        $match = [regex]::Match($line, '\A([a-f0-9]{64})  ([A-Za-z0-9._/-]+)\z')
        if (-not $match.Success) { throw 'invalid_checksum_line' }
        $name = $match.Groups[2].Value
        if ($payloadNames -cnotcontains $name -or $checksums.ContainsKey($name)) { throw 'unexpected_checksum_name' }
        $checksums[$name] = $match.Groups[1].Value
    }
    if ($checksums.Count -ne $payloadNames.Count -or $checksums['godusevpn-cli.exe'] -cne $fixedCliHash) {
        throw 'incomplete_checksum_manifest'
    }
    return $checksums
}

$result = [ordered]@{
    schemaVersion = 1; status = 'rejected'; stage = 'preflight'; observer = 'SYSTEM_required_not_yet_confirmed'
    zipSha256Match = $false; entryCountVerified = 0; checksumsVerified = 0
    extractedFileCount = 0; cliSha256Match = $false; smokeReady = $false
    outputLeaf = 'cli-smoke'; productExecuted = $false
}
$exitCode = 1
$zipStream = $null
$archive = $null
try {
    if ($PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or
        [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'unsupported_host' }
    if ([Security.Principal.WindowsIdentity]::GetCurrent().User.Value -ne 'S-1-5-18') { throw 'system_required' }
    $result.observer = 'SYSTEM'
    if ($StageID -cnotmatch '^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$') { throw 'invalid_stage_uuid4' }
    $programData = Assert-ExistingLocal ([Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData)) $true $false
    $qaRoot = Assert-ExistingLocal ([IO.Path]::Combine($programData, 'GodUseVPN-Ultra-QA')) $true $true
    $stageRoot = Assert-ExistingLocal ([IO.Path]::Combine($qaRoot, $StageID)) $true $true
    $zipPath = Assert-ExistingLocal ([IO.Path]::Combine($stageRoot, 'preview.zip')) $false $true
    $outputRoot = [IO.Path]::Combine($stageRoot, 'cli-smoke')
    # GetAttributes differentiates not-found from denied; neither a file nor a
    # reparse point/directory at the output name is accepted or taken over.
    $absent = $false
    try { [void][IO.File]::GetAttributes($outputRoot) }
    catch [IO.FileNotFoundException] { $absent = $true }
    catch [IO.DirectoryNotFoundException] { $absent = $true }
    if (-not $absent) { throw 'output_already_exists' }

    $result.stage = 'verify_zip'
    $zipStream = [IO.File]::Open($zipPath, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    if ($zipStream.Length -ne 25963585 -or (Get-StreamHash $zipStream) -cne $fixedZipHash) { throw 'zip_hash_mismatch' }
    $result.zipSha256Match = $true
    $zipStream.Position = 0
    Add-Type -AssemblyName System.IO.Compression # Framework assembly load only; no C# compilation.
    $archive = New-Object IO.Compression.ZipArchive($zipStream, [IO.Compression.ZipArchiveMode]::Read, $true)
    $entries = @{}
    $total = [long]0
    foreach ($entry in $archive.Entries) {
        $relative = Get-EntryRelativeName $entry.FullName
        if ($entries.ContainsKey($relative) -or $entry.Length -gt 67108864 -or $entry.Length -lt 0) { throw 'invalid_zip_entry' }
        $unixType = ($entry.ExternalAttributes -shr 16) -band 61440
        if (($entry.ExternalAttributes -band 1024) -ne 0 -or $unixType -notin @(0, 16384, 32768)) { throw 'zip_link_or_special_file' }
        if ($relative.EndsWith('/')) {
            if ($relative -cne 'docs/' -or $entry.Length -ne 0 -or $unixType -eq 32768) { throw 'unexpected_zip_directory' }
        } elseif (($payloadNames -cnotcontains $relative -and $relative -cne 'SHA256SUMS') -or $unixType -eq 16384) {
            throw 'unexpected_zip_file'
        }
        $entries[$relative] = $entry
        $total += $entry.Length
        if ($total -gt 67637605) { throw 'zip_expansion_limit' }
    }
    if ($entries.Count -ne 32 -or $total -ne 67637605 -or -not $entries.ContainsKey('docs/') -or
        -not $entries.ContainsKey('SHA256SUMS')) { throw 'incomplete_zip_entries' }
    $result.entryCountVerified = $entries.Count
    $manifestBytes = Read-SmallEntry $entries['SHA256SUMS'] 16384
    $checksums = Read-ChecksumManifest $manifestBytes
    $result.stage = 'verify_payloads'
    foreach ($name in $payloadNames) {
        if (-not $entries.ContainsKey($name)) { throw 'missing_payload' }
        $inputStream = $entries[$name].Open()
        try { if ((Get-StreamHash $inputStream) -cne $checksums[$name]) { throw 'payload_hash_mismatch' } }
        finally { $inputStream.Dispose() }
        $result.checksumsVerified++
    }
    $result.stage = 'create_private_output'
    $security = New-Object Security.AccessControl.DirectorySecurity
    $security.SetSecurityDescriptorSddlForm('O:SYG:SYD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)')
    $null = [IO.Directory]::CreateDirectory($outputRoot, $security)
    [void](Assert-ExistingLocal $outputRoot $true $true)
    $result.stage = 'write_selected_files'
    foreach ($name in @('godusevpn-cli.exe', 'SHA256SUMS', 'ULTRA_PACKAGE.json')) {
        # These three output names have no path separators. No archive-driven
        # directories or executable/install hooks are interpreted.
        $destination = [IO.Path]::Combine($outputRoot, $name)
        $output = [IO.File]::Open($destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        $inputStream = $null
        try {
            $inputStream = $entries[$name].Open()
            $inputStream.CopyTo($output, 1048576)
            $output.Flush($true)
        } finally { if ($null -ne $inputStream) { $inputStream.Dispose() }; $output.Dispose() }
        [void](Assert-ExistingLocal $destination $false $true)
        $verify = [IO.File]::Open($destination, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
        try {
            $expected = $fixedManifestHash
            if ($name -cne 'SHA256SUMS') { $expected = $checksums[$name] }
            if ($verify.Length -ne $entries[$name].Length -or (Get-StreamHash $verify) -cne $expected) { throw 'output_hash_mismatch' }
        } finally { $verify.Dispose() }
        if ($name -ceq 'godusevpn-cli.exe') { $result.cliSha256Match = $true }
        $result.extractedFileCount++
    }
    $result.stage = 'complete'
    $result.status = 'completed'
    $result.smokeReady = $true
    $exitCode = 0
} catch {
    # Keep all partial files for deliberate review. Never print exceptions,
    # archive names, actual paths, ACL identities, or raw command output.
} finally {
    if ($null -ne $archive) { $archive.Dispose() }
    if ($null -ne $zipStream) { $zipStream.Dispose() }
}
$result | ConvertTo-Json -Compress
exit $exitCode
