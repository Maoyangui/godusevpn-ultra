# Install the verified unsigned preview into a fresh fixed ProgramFiles directory.
[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'Medium')]
param([string]$Source = '')

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
# Windows PowerShell 5.1 can evaluate parameter defaults before PSScriptRoot is
# initialized under -File. Resolve the unbound default in the script body.
if (-not $PSBoundParameters.ContainsKey('Source')) { $Source = $PSScriptRoot }
if ([string]::IsNullOrWhiteSpace($Source)) { throw 'Source must be a nonempty local package directory. Invoke the saved installer file, or pass -Source explicitly.' }
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

function Set-ProgramACL([string]$Path) {
    # Program code must be readable/executable by the ordinary UI; only BA/SY can write.
    if ([IO.Directory]::Exists($Path)) {
        $acl = [Security.AccessControl.DirectorySecurity]::new()
        $acl.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;BU)')
    } else {
        $acl = [Security.AccessControl.FileSecurity]::new()
        $acl.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;BU)')
    }
    Set-Acl -LiteralPath $Path -AclObject $acl
}

function Set-ProgramTreeACL([string]$Root) {
    $pending = [Collections.Generic.Queue[string]]::new()
    $pending.Enqueue($Root)
    while ($pending.Count -gt 0) {
        $directory = $pending.Dequeue()
        Set-ProgramACL $directory
        foreach ($item in @(Get-ChildItem -LiteralPath $directory -Force)) {
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Unexpected installed reparse point.' }
            if ($item.PSIsContainer) { $pending.Enqueue($item.FullName) } else { Set-ProgramACL $item.FullName }
        }
    }
}

Assert-Platform
if (-not $WhatIfPreference -and -not (Test-Administrator)) { throw 'Run this installer from an elevated PowerShell. -WhatIf is read-only.' }
$whatIfRequested = [bool]$WhatIfPreference
$WhatIfPreference = $false
try {
    # CIM module discovery can emit its own Set-Alias ShouldProcess messages on
    # first load. These calls only read package/SCM state, so keep their output
    # deterministic and restore the caller's WhatIf preference before the
    # install action is evaluated.
    $package = Test-Package $Source
    $target = Get-FixedTarget
    Assert-NoReparse ([IO.Path]::GetDirectoryName($target))
    if ([IO.Directory]::Exists($target) -or [IO.File]::Exists($target)) { throw "Fresh installations only. Existing path is preserved: $target" }
    if ($null -ne (Get-UltraService)) { throw 'The Ultra service already exists. Upgrade/replacement is not supported by this preview.' }
} finally {
    $WhatIfPreference = $whatIfRequested
}
Write-Host "Verified unsigned Ultra $($package.Descriptor.version), $($package.Hashes.Count) payload files."
Write-Host "Target: $target"
Write-Warning 'Hashes prove package integrity, not publisher authenticity. Use a trusted Ultra release. This installer never touches the original godusevpn service/data.'
if (-not $PSCmdlet.ShouldProcess($target, 'Create protected directory, copy verified payload, install/start godusevpn-ultra service')) { return }
try {
    # Do not merge, overwrite, recursively delete, or kill any process.
    New-Item -ItemType Directory -Path $target -ErrorAction Stop | Out-Null
    Set-ProgramACL $target
    Assert-ProtectedCodeDirectory $target
    foreach ($relative in @($package.Hashes.Keys | Sort-Object)) {
        $sourceFile = Join-Path $package.Root $relative.Replace('/', '\')
        $destFile = Join-Path $target $relative.Replace('/', '\')
        $parent = [IO.Path]::GetDirectoryName($destFile)
        if (-not [IO.Directory]::Exists($parent)) { [IO.Directory]::CreateDirectory($parent) | Out-Null }
        Assert-NoReparse $sourceFile
        Assert-NoReparse $parent
        [IO.File]::Copy($sourceFile, $destFile, $false)
        Set-ProgramACL $destFile
        if ((Get-Sha256Hash $destFile) -ine $package.Hashes[$relative]) { throw "Copied payload changed: $relative" }
    }
    [IO.File]::Copy((Join-Path $package.Root 'SHA256SUMS'), (Join-Path $target 'SHA256SUMS'), $false)
    Set-ProgramACL (Join-Path $target 'SHA256SUMS')
    if ((Get-Sha256Hash (Join-Path $target 'SHA256SUMS')) -ine $package.ManifestHash) { throw 'Manifest changed during copy.' }
    Set-ProgramTreeACL $target
    $verified = Test-Package $target
    Assert-ProtectedCodeDirectory $target
    if ($verified.ManifestHash -ine $package.ManifestHash) { throw 'Installed manifest differs from verified source.' }
    if ($null -ne (Get-UltraService)) { throw 'Ultra service appeared during copy; refusing to overwrite it.' }
    $svcExe = Join-Path $target 'godusevpn-svc.exe'
    & $svcExe 'install'
    if ($LASTEXITCODE -ne 0) { throw "Ultra service install returned $LASTEXITCODE. Files and recovery tools are retained." }
    $service = Get-UltraService
    if ($null -eq $service) { throw 'Service install returned success but Ultra service was not found.' }
    Assert-ServicePath $service $svcExe
    Write-Host "Installed Ultra at $target. Launch godusevpn.exe from an ordinary user session."
    Write-Warning 'Only one VPN tunnel should run at a time. This preview does not guarantee safe simultaneous original/Ultra global guards.'
} catch {
    Write-Warning "Installation did not complete. Partial files/state are retained at $target; no automatic deletion or protection rollback was attempted."
    Write-Warning 'Manual recovery only: see docs/ultra/INSTALL_RECOVERY.zh-CN.md or docs/ultra/INSTALL_RECOVERY.en.md in the release/source. Do not rerun over or delete the partial installation automatically.'
    throw
}
