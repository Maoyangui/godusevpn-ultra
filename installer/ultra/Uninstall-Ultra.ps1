# Removes only the fixed Ultra service after privacy cleanup succeeds; retains all files.
[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'Medium')]
param(
    # Test fixtures may be inspected only with -WhatIf; this never changes the real target.
    [string]$ValidationPackage = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:UltraName = 'godusevpn-ultra'
$script:UltraProduct = 'GodUseVPN Ultra'

function Assert-Platform {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT -or -not [Environment]::Is64BitProcess) {
        throw 'Use 64-bit PowerShell on Windows.'
    }
}

function Test-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    try { return ([Security.Principal.WindowsPrincipal]::new($identity)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) }
    finally { $identity.Dispose() }
}

function Get-Sha256Hash([string]$Path) {
    # WinPS 5.1 Get-FileHash resolves paths with ForEach-Object ProviderPath,
    # which a script-level -WhatIf suppresses. Hash directly without that module.
    $sha = [Security.Cryptography.SHA256]::Create()
    $stream = $null
    try {
        $stream = [IO.File]::OpenRead($Path)
        return ([BitConverter]::ToString($sha.ComputeHash($stream)) -replace '-', '')
    } finally {
        if ($null -ne $stream) { $stream.Dispose() }
        $sha.Dispose()
    }
}

function Get-FixedTarget {
    $programFiles = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles)
    if ([string]::IsNullOrWhiteSpace($programFiles)) { throw 'Windows ProgramFiles known folder is unavailable.' }
    return [IO.Path]::GetFullPath((Join-Path $programFiles $script:UltraProduct)).TrimEnd('\')
}

function Assert-LocalPath([string]$Path) {
    $full = [IO.Path]::GetFullPath($Path).TrimEnd('\')
    if ($full -notmatch '^[A-Za-z]:\\' -or $full.Substring(2).Contains(':') -or $full.StartsWith('\\')) {
        throw "Only local drive paths are accepted: $Path"
    }
    $drive = [IO.DriveInfo]::new([IO.Path]::GetPathRoot($full))
    if ($drive.DriveType -notin @([IO.DriveType]::Fixed, [IO.DriveType]::Removable)) {
        throw "Network/unknown drives are not accepted: $Path"
    }
    return $full
}

function Assert-NoReparse([string]$Path) {
    $part = [IO.Path]::GetFullPath($Path)
    while ($part) {
        $attributes = [IO.File]::GetAttributes($part)
        if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Reparse points are not accepted: $part"
        }
        $parent = [IO.Directory]::GetParent($part)
        if ($null -eq $parent) { break }
        $part = $parent.FullName
    }
}

function Get-Inventory([string]$Root) {
    $items = [Collections.Generic.Dictionary[string, object]]::new([StringComparer]::OrdinalIgnoreCase)
    $pending = [Collections.Generic.Queue[string]]::new()
    $pending.Enqueue($Root)
    while ($pending.Count -gt 0) {
        $dir = $pending.Dequeue()
        foreach ($item in @(Get-ChildItem -LiteralPath $dir -Force)) {
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Payload reparse point: $($item.FullName)" }
            if ($item.PSIsContainer) { $pending.Enqueue($item.FullName); continue }
            $relative = $item.FullName.Substring($Root.Length + 1).Replace('\', '/')
            Assert-RelativeName $relative
            if ($items.ContainsKey($relative)) { throw "Duplicate payload path: $relative" }
            $items.Add($relative, $item)
        }
    }
    return ,$items
}

function Assert-RelativeName([string]$Name) {
    if ([string]::IsNullOrWhiteSpace($Name) -or $Name -ne $Name.Trim() -or $Name.Contains('\') -or $Name.Contains(':') -or $Name.StartsWith('/')) {
        throw "Invalid manifest path: $Name"
    }
    foreach ($part in $Name.Split('/')) {
        if (-not $part -or $part -in @('.', '..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or
            $part.IndexOfAny([IO.Path]::GetInvalidFileNameChars()) -ge 0 -or $part -match '^(?i:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)') {
            throw "Unsafe manifest component: $Name"
        }
    }
}

function Test-Package([string]$Root) {
    $Root = Assert-LocalPath $Root
    Assert-NoReparse $Root
    if (-not [IO.Directory]::Exists($Root)) { throw "Package directory does not exist: $Root" }
    $inventory = Get-Inventory $Root
    if (-not $inventory.ContainsKey('SHA256SUMS')) { throw 'SHA256SUMS is missing.' }
    $manifestPath = Join-Path $Root 'SHA256SUMS'
    if ((Get-Item -LiteralPath $manifestPath).Length -gt 4MB) { throw 'Hash manifest exceeds limit.' }
    $hashes = [Collections.Generic.Dictionary[string, string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($line in [IO.File]::ReadAllLines($manifestPath)) {
        if ($line -notmatch '^([0-9a-fA-F]{64})  (.+)$') { throw 'Malformed SHA256SUMS line.' }
        $hash = $Matches[1].ToLowerInvariant(); $relative = $Matches[2]
        Assert-RelativeName $relative
        if ($relative -ieq 'SHA256SUMS' -or $hashes.ContainsKey($relative)) { throw "Duplicate/self manifest entry: $relative" }
        if (-not $inventory.ContainsKey($relative)) { throw "Missing listed payload: $relative" }
        $actual = Get-Sha256Hash $inventory[$relative].FullName
        if ($actual -ine $hash) { throw "Payload hash mismatch: $relative" }
        $hashes.Add($relative, $hash)
    }
    if ($inventory.Count -ne $hashes.Count + 1) { throw 'Unlisted payload files are present; package rejected.' }
    foreach ($relative in $inventory.Keys) {
        if ($relative -ine 'SHA256SUMS' -and -not $hashes.ContainsKey($relative)) { throw "Unlisted payload: $relative" }
    }
    foreach ($required in @('godusevpn.exe', 'godusevpn-svc.exe', 'godusevpn-cli.exe', 'ULTRA_PACKAGE.json', 'Install-Ultra.ps1', 'Uninstall-Ultra.ps1', 'browser-extension/manifest.json')) {
        if (-not $hashes.ContainsKey($required)) { throw "Required payload is missing: $required" }
    }
    $descriptorFile = Join-Path $Root 'ULTRA_PACKAGE.json'
    if ((Get-Item -LiteralPath $descriptorFile).Length -gt 64KB) { throw 'Package descriptor exceeds limit.' }
    $descriptor = [IO.File]::ReadAllText($descriptorFile) | ConvertFrom-Json
    if ($descriptor.product -cne $script:UltraProduct -or $descriptor.namespace -cne $script:UltraName -or
        $descriptor.version -notmatch '^[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-[A-Za-z0-9.-]+)?$' -or
        $descriptor.architecture -notin @('amd64', 'arm64') -or $descriptor.unsigned -isnot [bool] -or -not $descriptor.unsigned -or
        $descriptor.sourceBaseline -notmatch '^[0-9a-fA-F]{7,64}$') { throw 'Descriptor is not a supported unsigned Ultra release.' }
    $native = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    $expected = switch ($native) { 'X64' { 'amd64' } 'Arm64' { 'arm64' } default { throw "Unsupported OS architecture: $native" } }
    if ($descriptor.architecture -cne $expected) { throw "Package architecture must match the OS: $expected" }
    if ($null -ne $descriptor.PSObject.Properties['browserHost']) {
        $hostDescriptor = $descriptor.browserHost
        if ($descriptor.architecture -cne 'amd64' -or $hostDescriptor.schemaVersion -ne 1 -or $hostDescriptor.implementation -cne 'webview2-winforms' -or
            $hostDescriptor.architecture -cne 'amd64' -or $hostDescriptor.sdkVersion -cne '1.0.4314.35' -or
            $hostDescriptor.nativeRuntimeBundled -isnot [bool] -or $hostDescriptor.nativeRuntimeBundled) { throw 'Unsupported independent browser host descriptor.' }
        foreach ($hostFile in @('GodUseVPN.Ultra.BrowserHost.exe', 'GodUseVPN.Ultra.BrowserHost.exe.config', 'Microsoft.Web.WebView2.Core.dll', 'Microsoft.Web.WebView2.WinForms.dll', 'WebView2Loader.dll', 'Microsoft.Web.WebView2.LICENSE.txt', 'README.md')) {
            if (-not $hashes.ContainsKey('browser-host/' + $hostFile)) { throw 'Independent browser host payload is incomplete.' }
        }
    }
    if ($null -ne $descriptor.PSObject.Properties['locationConsumer']) {
        $consumer = $descriptor.locationConsumer
        if ($null -eq $consumer -or $descriptor.architecture -cne 'amd64' -or $consumer.schemaVersion -ne 1 -or
            $consumer.implementation -cne 'winrt-geolocator' -or $consumer.architecture -cne 'amd64' -or $consumer.minimumFramework -cne '4.6.2' -or
            $consumer.nativeRuntimeBundled -isnot [bool] -or $consumer.nativeRuntimeBundled -or
            $consumer.vmValidated -isnot [bool] -or $consumer.vmValidated) { throw 'Unsupported independent location consumer descriptor.' }
        foreach ($consumerFile in @('GodUseVPN.Ultra.LocationConsumer.exe', 'GodUseVPN.Ultra.LocationConsumer.exe.config')) {
            if (-not $hashes.ContainsKey('location-consumer/' + $consumerFile)) { throw 'Independent location consumer payload is incomplete.' }
        }
        if (@($hashes.Keys | Where-Object { $_.StartsWith('location-consumer/', [StringComparison]::OrdinalIgnoreCase) }).Count -ne 2) { throw 'Unexpected independent location consumer payload.' }
        $consumerConfig = Join-Path $Root 'location-consumer/GodUseVPN.Ultra.LocationConsumer.exe.config'
        if ((Get-Item -LiteralPath $consumerConfig).Length -gt 1024) { throw 'Location consumer runtime configuration exceeds limit.' }
        $expectedConfig = '<?xml version="1.0"?><configuration><startup><supportedRuntime version="v4.0" sku=".NETFramework,Version=v4.6.2"/></startup></configuration>'
        if ([IO.File]::ReadAllText($consumerConfig, [Text.UTF8Encoding]::new($false, $true)) -cne $expectedConfig) { throw 'Location consumer runtime configuration mismatch.' }
    } elseif (@($hashes.Keys | Where-Object { $_.StartsWith('location-consumer/', [StringComparison]::OrdinalIgnoreCase) }).Count -ne 0) { throw 'Undeclared location consumer payload.' }
    if ($null -ne $descriptor.PSObject.Properties['metadataTool']) {
        $tool = $descriptor.metadataTool
        $pinnedManifest = 'd1316fddf986e2d17761f85034c753b28375f25bf73235c1084a7c5a38e22faf'
        if ($null -eq $tool -or $descriptor.architecture -cne 'amd64' -or $tool.version -cne '13.59' -or
            $tool.implementation -cne 'exiftool' -or $tool.manifestSHA256 -cne $pinnedManifest -or
            ($tool.fileCount -isnot [int] -and $tool.fileCount -isnot [long]) -or $tool.fileCount -ne 1501 -or
            $tool.vmValidated -isnot [bool] -or $tool.vmValidated) { throw 'Unsupported metadata tool descriptor.' }
        $innerName = 'metadata-tool/exiftool-13.59.sha256'
        if (-not $hashes.ContainsKey($innerName) -or $hashes[$innerName] -cne $pinnedManifest) { throw 'Pinned metadata manifest is missing or changed.' }
        $innerPath = Join-Path $Root $innerName
        if ((Get-Item -LiteralPath $innerPath).Length -gt 1MB) { throw 'Metadata manifest exceeds limit.' }
        $members = [Collections.Generic.Dictionary[string, string]]::new([StringComparer]::OrdinalIgnoreCase)
        $prefix = 'metadata-tool/exiftool-13.59/'
        foreach ($line in [IO.File]::ReadAllLines($innerPath)) {
            if ($line -notmatch '^([0-9a-f]{64})  (.+)$') { throw 'Malformed metadata manifest.' }
            $memberHash = $Matches[1]; $member = $Matches[2]
            Assert-RelativeName $member
            if ($members.ContainsKey($member)) { throw 'Duplicate metadata member.' }
            $members.Add($member, $memberHash)
            $outerName = $prefix + $member
            if (-not $hashes.ContainsKey($outerName) -or $hashes[$outerName] -cne $memberHash) { throw 'Metadata member missing or differs from pinned manifest.' }
        }
        if ($members.Count -ne 1501) { throw 'Incomplete metadata manifest.' }
        $subtreeCount = @($hashes.Keys | Where-Object { $_.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) }).Count
        if ($subtreeCount -ne $members.Count) { throw 'Unexpected metadata tool member.' }
    }
    return [pscustomobject]@{ Root = $Root; Hashes = $hashes; Descriptor = $descriptor; ManifestHash = (Get-Sha256Hash $manifestPath) }
}

function Assert-CodeNode([string]$Path, [bool]$IsRoot = $false) {
    Assert-NoReparse $Path
    $acl = Get-Acl -LiteralPath $Path
    $owner = $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value
    if ($owner -notin @('S-1-5-18', 'S-1-5-32-544') -or ($IsRoot -and -not $acl.AreAccessRulesProtected)) {
        throw "Code must be owned by Administrators/SYSTEM; the installation root must have a protected ACL: $Path"
    }
    $dangerous = [int64]0x500D0156
    foreach ($rule in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
        if ($rule.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and
            (([int64]$rule.FileSystemRights -band $dangerous) -ne 0) -and
            $rule.IdentityReference.Value -notin @('S-1-5-18', 'S-1-5-32-544')) {
            throw "An untrusted account can modify installed code; privileged execution refused: $Path"
        }
    }
}

function Assert-ProtectedCodeDirectory([string]$Path) {
    Assert-CodeNode $Path $true
    $pending = [Collections.Generic.Queue[string]]::new()
    $pending.Enqueue($Path)
    while ($pending.Count -gt 0) {
        foreach ($item in @(Get-ChildItem -LiteralPath $pending.Dequeue() -Force)) {
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Installed code reparse point: $($item.FullName)" }
            Assert-CodeNode $item.FullName
            if ($item.PSIsContainer) { $pending.Enqueue($item.FullName) }
        }
    }
}

function Get-UltraService {
    $found = @(Get-CimInstance -ClassName Win32_Service -Filter "Name='godusevpn-ultra'")
    if ($found.Count -gt 1) { throw 'Ambiguous Ultra service identity.' }
    if ($found.Count -eq 1) { return $found[0] }
    return $null
}

function Assert-ServicePath($Service, [string]$ExpectedExe) {
    if ($null -eq $Service) { return }
    $raw = ([string]$Service.PathName).Trim()
    if ($raw -notmatch '^"([^"]+)"\s+service$') { throw "Ultra service command line is not the fixed installation command; no changes made: $raw" }
    $binary = [IO.Path]::GetFullPath($Matches[1])
    if (-not [string]::Equals($binary, $ExpectedExe, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Ultra service belongs to another directory; no changes made: $raw"
    }
    Assert-NoReparse $binary
}

function Assert-NoRunningGUI([string]$Target) {
    $gui = Join-Path $Target 'godusevpn.exe'
    $browserHost = Join-Path $Target 'browser-host/GodUseVPN.Ultra.BrowserHost.exe'
    $locationConsumer = Join-Path $Target 'location-consumer/GodUseVPN.Ultra.LocationConsumer.exe'
    $running = @(Get-CimInstance -ClassName Win32_Process -Filter "Name='godusevpn.exe' OR Name='GodUseVPN.Ultra.BrowserHost.exe' OR Name='GodUseVPN.Ultra.LocationConsumer.exe'")
    foreach ($process in $running) {
        if ([string]::IsNullOrEmpty([string]$process.ExecutablePath)) { throw "Cannot verify godusevpn.exe PID $($process.ProcessId); close Ultra and retry from an administrator session. No process was killed." }
        $actual = [IO.Path]::GetFullPath([string]$process.ExecutablePath)
        if ([string]::Equals($actual, $gui, [StringComparison]::OrdinalIgnoreCase) -or
            [string]::Equals($actual, $browserHost, [StringComparison]::OrdinalIgnoreCase) -or
            [string]::Equals($actual, $locationConsumer, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Close the installed Ultra GUI, browser and privacy helper first (PID $($process.ProcessId)). No process was killed."
        }
    }
}

Assert-Platform
if ($ValidationPackage -and -not $WhatIfPreference) { throw '-ValidationPackage is allowed only with -WhatIf.' }
if (-not $WhatIfPreference -and -not (Test-Administrator)) { throw 'Run this uninstaller from an elevated PowerShell. -WhatIf is read-only.' }
$target = Get-FixedTarget
$validationRoot = $target
if ($ValidationPackage) { $validationRoot = $ValidationPackage }
$package = Test-Package $validationRoot
if (-not $ValidationPackage) { Assert-ProtectedCodeDirectory $target }
if (-not $WhatIfPreference -and -not [string]::Equals([IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\'), $target, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Run the installed Uninstall-Ultra.ps1 from the fixed directory: $target"
}
Assert-NoReparse ([IO.Path]::GetDirectoryName($target))
$svcExe = Join-Path $target 'godusevpn-svc.exe'
$cliExe = Join-Path $target 'godusevpn-cli.exe'
$service = Get-UltraService
Assert-ServicePath $service $svcExe
Assert-NoRunningGUI $target
Write-Host "Verified Ultra descriptor $($package.Descriptor.version). Fixed target: $target"
Write-Warning 'Each user must first restore browser/current-user privacy settings in their ordinary UI. This elevated script never restores HKCU or another account timezone/language.'
Write-Warning 'All installed files, logs, data and backups are retained for manual review/removal. No recursive deletion or basename process kill is performed.'
if (-not $PSCmdlet.ShouldProcess($target, 'Run fixed machine privacy cleanup, then uninstall only the matching godusevpn-ultra service; retain files/data')) { return }
try {
    # Recheck private on-disk code immediately before running privileged commands.
    $null = Test-Package $target
    Assert-ProtectedCodeDirectory $target
    Assert-NoRunningGUI $target
    Assert-ServicePath (Get-UltraService) $svcExe
    Assert-ProtectedCodeDirectory $target
    # One process owns the machine-maintenance lease through cleanup, final
    # verification and the fixed matching service uninstall. Other copies,
    # helpers, timezone writes and CLIs receive busy instead of racing this gap.
    & $cliExe 'privacy' 'uninstall-machine'
    if ($LASTEXITCODE -ne 0) { throw "Machine cleanup/uninstall returned $LASTEXITCODE. Files/data/recovery tools are retained." }
    if ($null -ne (Get-UltraService)) { throw 'Ultra service still exists or is pending deletion. Files and recovery tools are retained; retry after closing service management tools.' }
    Write-Host 'Ultra service and machine privacy cleanup completed. Installed files and all per-user/machine data were intentionally retained.'
    Write-Host "Review files manually at: $target"
    Write-Warning 'Do not delete remaining backups until each user confirms their browser/privacy settings and network adapters are correct. The original godusevpn installation was not managed by this script.'
} catch {
    Write-Warning 'Uninstall did not complete. No files or backups were deleted; recovery tools remain available.'
    Write-Warning 'For manual review and recovery, see docs/ultra/INSTALL_RECOVERY.zh-CN.md or docs/ultra/INSTALL_RECOVERY.en.md in the release/source. This beta has no automatic takeover, resume or deletion.'
    throw
}
