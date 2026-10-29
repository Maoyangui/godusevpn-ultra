# Read-only, local preflight for Windows PowerShell 5.1. Run without -Verbose.
# Prints one JSON object. No usernames, paths, machine names, SIDs, addresses,
# credentials, user profiles/history, setting contents, GUI, or network requests.
# Run in the intended interactive user's token: HKCU and user-folder checks
# describe this token only. Elevation is observed, never requested.
# Missing values and access failures are "unknown"; confirmed absent fixed
# services/files are false. Existence is not policy support or enforcement.
# Default local install/data locations only; custom data overrides, portable
# browsers and application-specific fixed-version WebView2 are not searched.
# Prefer 64-bit Windows PowerShell on a 64-bit OS. A 32-bit host may not see
# native ProgramFiles: browser results it cannot find are then unknown.
# An interactive session means session ID > 0 in Active or Disconnected state;
# neither account names nor the native station-name pointer are read.
# Data backup checks inspect existence only, not integrity or restorability.
[CmdletBinding()]
param()

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$WarningPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'

function New-ReadOnlyNativeType {
    # Reflection.Emit uses memory only. Unlike Add-Type on PowerShell 5.1,
    # this does not compile C# through temporary source/assembly files.
    $assemblyName = New-Object Reflection.AssemblyName('UltraReadOnlyPreflight')
    $assembly = [AppDomain]::CurrentDomain.DefineDynamicAssembly($assemblyName, [Reflection.Emit.AssemblyBuilderAccess]::Run)
    $module = $assembly.DefineDynamicModule('Native')
    $type = $module.DefineType('UltraReadOnlyPreflightNative', [Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $signatures = @(
        @('GetTokenInformation', 'advapi32.dll', [bool], [type[]]@([IntPtr], [int], [uint32].MakeByRefType(), [int], [int].MakeByRefType())),
        @('WTSEnumerateSessionsW', 'wtsapi32.dll', [bool], [type[]]@([IntPtr], [int], [int], [IntPtr].MakeByRefType(), [int].MakeByRefType())),
        @('WTSFreeMemory', 'wtsapi32.dll', [void], [type[]]@([IntPtr])),
        @('GetNativeSystemInfo', 'kernel32.dll', [void], [type[]]@([IntPtr]))
    )
    foreach ($signature in $signatures) {
        $method = $type.DefinePInvokeMethod($signature[0], $signature[1],
            [Reflection.MethodAttributes]'Public, Static, PinvokeImpl',
            [Reflection.CallingConventions]::Standard, $signature[2], $signature[3],
            [Runtime.InteropServices.CallingConvention]::Winapi, [Runtime.InteropServices.CharSet]::Unicode)
        $method.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
    }
    return $type.CreateType()
}

function Get-KnownLocalFolder([Environment+SpecialFolder]$Folder) {
    try {
        $value = [Environment]::GetFolderPath($Folder, [Environment+SpecialFolderOption]::DoNotVerify)
        if ($value -match '^[A-Za-z]:\\' -and -not $value.Substring(2).Contains(':')) { return $value }
    } catch { }
    return $null
}

function Join-FixedPath([string]$Root, [string]$Child) {
    if ([string]::IsNullOrEmpty($Root)) { return $null }
    try { return [IO.Path]::Combine($Root, $Child) } catch { return $null }
}

function Test-FixedLocalItem([string]$Path, [bool]$Directory) {
    if ([string]::IsNullOrEmpty($Path)) { return 'unknown' }
    try {
        $full = [IO.Path]::GetFullPath($Path)
        if ($full -notmatch '^[A-Za-z]:\\' -or $full.Substring(2).Contains(':')) { return 'unknown' }
        $root = [IO.Path]::GetPathRoot($full)
        $drive = New-Object IO.DriveInfo($root)
        if ($drive.DriveType -ne [IO.DriveType]::Fixed) { return 'unknown' }
        # Check ancestors from root downward before touching the child. Never
        # follow a junction/symlink or redirected folder onto a network share.
        $current = $root
        $parts = @($full.Substring($root.Length).Split('\') | Where-Object { $_ -ne '' })
        $attributes = [IO.File]::GetAttributes($current)
        if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { return 'unknown' }
        for ($index = 0; $index -lt $parts.Count; $index++) {
            $current = [IO.Path]::Combine($current, $parts[$index])
            $attributes = [IO.File]::GetAttributes($current)
            if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { return 'unknown' }
            if ($index -lt ($parts.Count - 1) -and ($attributes -band [IO.FileAttributes]::Directory) -eq 0) { return $false }
        }
        return ((($attributes -band [IO.FileAttributes]::Directory) -ne 0) -eq $Directory)
    } catch [IO.FileNotFoundException] { return $false
    } catch [IO.DirectoryNotFoundException] { return $false
    } catch { return 'unknown' }
}

function Read-FixedRegistryValue([Microsoft.Win32.RegistryHive]$Hive, [Microsoft.Win32.RegistryView]$View, [string]$Key, [string]$Name) {
    $base = $null
    $item = $null
    try {
        $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey($Hive, $View)
        $item = $base.OpenSubKey($Key, $false)
        if ($null -eq $item) { return [pscustomobject]@{ state = 'absent'; value = $null } }
        $value = $item.GetValue($Name, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        if ($null -eq $value) { return [pscustomobject]@{ state = 'absent'; value = $null } }
        return [pscustomobject]@{ state = 'read'; value = $value }
    } catch { return [pscustomobject]@{ state = 'unknown'; value = $null }
    } finally {
        if ($null -ne $item) { $item.Dispose() }
        if ($null -ne $base) { $base.Dispose() }
    }
}

function ConvertTo-SafeVersion($Value) {
    if ($Value -is [string] -and $Value -match '^\d{1,5}(\.\d{1,5}){1,3}$' -and $Value -ne '0.0.0.0') { return $Value }
    return 'unknown'
}

function Read-FixedBrowser([string[]]$Candidates) {
    $found = $false
    $uncertain = $false
    $versions = @()
    foreach ($candidate in $Candidates) {
        $exists = Test-FixedLocalItem $candidate $false
        if ($exists -is [bool] -and $exists) {
            $found = $true
            try {
                $version = ConvertTo-SafeVersion ([Diagnostics.FileVersionInfo]::GetVersionInfo($candidate).ProductVersion)
                if ($version -ne 'unknown') { $versions += $version }
            } catch { }
        } elseif ($exists -isnot [bool]) { $uncertain = $true }
    }
    $presence = $false
    if ($found) { $presence = $true } elseif ($uncertain -or $Candidates.Count -eq 0) { $presence = 'unknown' }
    $versionResult = 'unknown'
    if ($versions.Count -gt 0) { $versionResult = @($versions | Sort-Object -Unique) }
    return [ordered]@{ exists = $presence; versions = $versionResult }
}

function Read-FixedService([string]$Name) {
    try {
        $service = Get-Service -Name $Name -ErrorAction Stop
        $status = switch ([int]$service.Status) {
            1 { 'stopped' }; 2 { 'start_pending' }; 3 { 'stop_pending' }
            4 { 'running' }; 5 { 'continue_pending' }; 6 { 'pause_pending' }
            7 { 'paused' }; default { 'unknown' }
        }
        return [ordered]@{ exists = $true; status = $status }
    } catch {
        if ($_.FullyQualifiedErrorId -like 'NoServiceFoundForGivenName*') { return [ordered]@{ exists = $false; status = 'absent' } }
        return [ordered]@{ exists = 'unknown'; status = 'unknown' }
    }
}

function Read-InteractiveSession($Native) {
    if ($null -eq $Native) { return 'unknown' }
    $buffer = [IntPtr]::Zero
    $count = 0
    try {
        if (-not $Native::WTSEnumerateSessionsW([IntPtr]::Zero, 0, 1, [ref]$buffer, [ref]$count)) { return 'unknown' }
        if ($count -lt 0 -or $count -gt 4096 -or ($count -gt 0 -and $buffer -eq [IntPtr]::Zero)) { return 'unknown' }
        $rowSize = 12
        if ([IntPtr]::Size -eq 8) { $rowSize = 24 }
        for ($index = 0; $index -lt $count; $index++) {
            $row = [IntPtr]::Add($buffer, $index * $rowSize)
            $session = [Runtime.InteropServices.Marshal]::ReadInt32($row, 0)
            $state = [Runtime.InteropServices.Marshal]::ReadInt32($row, 2 * [IntPtr]::Size)
            if ($session -gt 0 -and $state -in @(0, 4)) { return $true }
        }
        return $false
    } catch { return 'unknown'
    } finally { if ($buffer -ne [IntPtr]::Zero) { try { $Native::WTSFreeMemory($buffer) } catch { } } }
}

$report = [ordered]@{
    schemaVersion = 1
    os = [ordered]@{ product = 'unknown'; edition = 'unknown'; build = 'unknown'; architecture = 'unknown' }
    token = [ordered]@{ elevated = 'unknown'; interactiveSidPresent = 'unknown' }
    bfe = [ordered]@{ exists = 'unknown'; running = 'unknown' }
    services = [ordered]@{ original = [ordered]@{ exists = 'unknown'; status = 'unknown' }; ultra = [ordered]@{ exists = 'unknown'; status = 'unknown' } }
    browsers = [ordered]@{ chrome = [ordered]@{ exists = 'unknown'; versions = 'unknown' }; edge = [ordered]@{ exists = 'unknown'; versions = 'unknown' }; webview2 = [ordered]@{ exists = 'unknown'; versions = 'unknown' } }
    interactiveSessionExists = 'unknown'
    ultraNamespace = [ordered]@{ dataDirectoryExists = 'unknown'; machineBackupExists = 'unknown'; userBackupExists = 'unknown'; timeBackupExists = 'unknown' }
}

if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    $native = $null
    try { $native = New-ReadOnlyNativeType } catch { }
    $view = [Microsoft.Win32.RegistryView]::Registry32
    if ([Environment]::Is64BitOperatingSystem) { $view = [Microsoft.Win32.RegistryView]::Registry64 }
    $osKey = 'SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    $product = Read-FixedRegistryValue LocalMachine $view $osKey 'ProductName'
    if ($product.state -eq 'read' -and $product.value -is [string] -and $product.value -match '^Windows [A-Za-z0-9 .()_-]{1,100}$') { $report.os.product = $product.value }
    $edition = Read-FixedRegistryValue LocalMachine $view $osKey 'EditionID'
    if ($edition.state -eq 'read' -and $edition.value -is [string] -and $edition.value -match '^[A-Za-z0-9_-]{1,64}$') { $report.os.edition = $edition.value }
    $build = Read-FixedRegistryValue LocalMachine $view $osKey 'CurrentBuildNumber'
    $revision = Read-FixedRegistryValue LocalMachine $view $osKey 'UBR'
    if ([string]$build.value -match '^\d{1,6}$') {
        $report.os.build = [string]$build.value
        if ([string]$revision.value -match '^\d{1,6}$') { $report.os.build += '.' + [string]$revision.value }
    }
    $systemInfo = [IntPtr]::Zero
    try {
        if ($null -ne $native) {
            $systemInfo = [Runtime.InteropServices.Marshal]::AllocHGlobal(64)
            $native::GetNativeSystemInfo($systemInfo)
            $report.os.architecture = switch ([Runtime.InteropServices.Marshal]::ReadInt16($systemInfo)) { 0 { 'x86' }; 9 { 'x64' }; 12 { 'arm64' }; 5 { 'arm' }; default { 'unknown' } }
        }
    } catch { } finally { if ($systemInfo -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::FreeHGlobal($systemInfo) } }
    $identity = $null
    try {
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $groups = $identity.Groups
        if ($null -ne $groups) {
            $hasInteractive = $false
            foreach ($group in $groups) {
                if ($group.IsWellKnown([Security.Principal.WellKnownSidType]::InteractiveSid)) { $hasInteractive = $true; break }
            }
            $report.token.interactiveSidPresent = $hasInteractive
        }
        if ($null -ne $native) {
            $elevation = [uint32]0
            $returned = 0
            if ($native::GetTokenInformation($identity.Token, 20, [ref]$elevation, 4, [ref]$returned) -and $returned -eq 4) { $report.token.elevated = ($elevation -ne 0) }
        }
    } catch { } finally { if ($null -ne $identity) { $identity.Dispose() } }
    $report.interactiveSessionExists = Read-InteractiveSession $native
    $bfe = Read-FixedService 'BFE'
    $report.bfe.exists = $bfe.exists
    if ($bfe.status -ne 'unknown') { $report.bfe.running = ($bfe.status -eq 'running') }
    $report.services.original = Read-FixedService 'godusevpn'
    $report.services.ultra = Read-FixedService 'godusevpn-ultra'

    $programFiles = Get-KnownLocalFolder ProgramFiles
    $programFilesX86 = Get-KnownLocalFolder ProgramFilesX86
    $localData = Get-KnownLocalFolder LocalApplicationData
    $commonData = Get-KnownLocalFolder CommonApplicationData
    $roamingData = Get-KnownLocalFolder ApplicationData
    $browserRoots = @(@($programFiles, $programFilesX86, $localData) | Where-Object { -not [string]::IsNullOrEmpty($_) } | Sort-Object -Unique)
    $chromeCandidates = @($browserRoots | ForEach-Object { Join-FixedPath $_ 'Google\Chrome\Application\chrome.exe' })
    $edgeCandidates = @($browserRoots | ForEach-Object { Join-FixedPath $_ 'Microsoft\Edge\Application\msedge.exe' })
    $report.browsers.chrome = Read-FixedBrowser $chromeCandidates
    $report.browsers.edge = Read-FixedBrowser $edgeCandidates
    $limitedBrowserHost = [Environment]::Is64BitOperatingSystem -and -not [Environment]::Is64BitProcess
    if ($limitedBrowserHost) {
        foreach ($browserName in @('chrome', 'edge')) {
            if ($report.browsers[$browserName].exists -is [bool] -and -not $report.browsers[$browserName].exists) { $report.browsers[$browserName].exists = 'unknown' }
        }
    }

    # Official Evergreen registration key; pv is accepted only as a numeric
    # version and is never executed. Verify the matching fixed executable too.
    # https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution
    $runtimeKey = 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
    $runtimeSources = @(
        @('LocalMachine', [Microsoft.Win32.RegistryView]::Registry32, $programFilesX86),
        @('CurrentUser', $view, $localData)
    )
    if ([Environment]::Is64BitOperatingSystem) { $runtimeSources += ,@('LocalMachine', [Microsoft.Win32.RegistryView]::Registry64, $programFiles) }
    else { $runtimeSources[0][2] = $programFiles }
    $runtimeCandidates = @()
    $runtimeUncertain = $limitedBrowserHost
    foreach ($source in $runtimeSources) {
        $registration = Read-FixedRegistryValue $source[0] $source[1] $runtimeKey 'pv'
        if ($registration.state -eq 'unknown') { $runtimeUncertain = $true; continue }
        if ($registration.state -ne 'read') { continue }
        $runtimeVersion = ConvertTo-SafeVersion $registration.value
        if ($runtimeVersion -eq 'unknown') {
            if ($registration.value -notin @('', '0.0.0.0')) { $runtimeUncertain = $true }
            continue
        }
        $runtimeCandidates += Join-FixedPath $source[2] ('Microsoft\EdgeWebView\Application\' + $runtimeVersion + '\msedgewebview2.exe')
    }
    if ($runtimeCandidates.Count -gt 0) { $report.browsers.webview2 = Read-FixedBrowser $runtimeCandidates }
    elseif (-not $runtimeUncertain) { $report.browsers.webview2.exists = $false }
    if ($runtimeUncertain -and $report.browsers.webview2.exists -is [bool] -and -not $report.browsers.webview2.exists) { $report.browsers.webview2.exists = 'unknown' }

    $dataRoot = Join-FixedPath $commonData 'godusevpn-ultra'
    $userRoot = Join-FixedPath $roamingData 'godusevpn-ultra'
    $report.ultraNamespace.dataDirectoryExists = Test-FixedLocalItem $dataRoot $true
    $report.ultraNamespace.machineBackupExists = Test-FixedLocalItem (Join-FixedPath $dataRoot 'privacy-machine\privacy-journal.json') $false
    $report.ultraNamespace.userBackupExists = Test-FixedLocalItem (Join-FixedPath $userRoot 'privacy-user\privacy-journal.json') $false
    $report.ultraNamespace.timeBackupExists = Test-FixedLocalItem (Join-FixedPath $userRoot 'privacy-time\timezone-journal.json') $false
}

$report | ConvertTo-Json -Depth 8 -Compress
