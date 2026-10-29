# Read-only CLI smoke test for Windows PowerShell 5.1, 64-bit Windows.
# This script does not run the GUI, service executable, helper, installer,
# detection command, elevation, tunnel commands, or setting changes.
# It starts only the hash-pinned CLI with exactly: version; privacy report.
# No network requests, files, transcripts, or raw command output are produced.
# Windows itself may keep ordinary process/security/antivirus execution records;
# "read-only" describes this script and the audited application command paths.
#
# ExpectedCliSha256 MUST come from the trusted local build/reviewer, separately
# from the untrusted download. A checksum bundled with a download is not proof
# of authenticity. The hash binds this test to that exact audited CLI binary,
# not to the whole ZIP. For ZIP transfer, verify its separately supplied SHA256
# before extraction as a distinct step; this script never downloads anything.
#
# REQUIRED EXTERNAL PRECONDITION: use the separately verified, private QA stage
# created by the stager under ProgramData/GodUseVPN-Ultra-QA. Its owner/write
# ACLs and ancestor namespace must prevent untrusted users from replacing the
# package or ancestors while the test runs. The stager/operator must verify
# those ACLs; this script does NOT verify owner/ACLs or close directory races.
# APPDATA and its checked ancestors must likewise remain stable during the test.
# A leaf EXE read handle is not a lock on the parent directory namespace.
# JSON-only output below assumes a syntactically valid invocation of these
# parameters; PowerShell itself may print binding errors before the body starts.
#
# Example (replace both placeholders; do not use this as a real expected hash):
# powershell.exe -NoProfile -NonInteractive -File .\Vps-ReadOnlySmoke.ps1 `
#   -PackageDirectory 'C:\UltraTest\unpacked' -ExpectedCliSha256 '<trusted 64 hex>'
#
# Run Command usually runs as SYSTEM. HKCU and APPDATA below describe ONLY that
# token, not the RDP/GUI user's profile. No helper or other user's hive is opened.
# No journal is created. BackupHealthy refers only to this token's user journal,
# not to machine backups or a successful restoration. Policy configuration is
# never reported as runtime blocking, browser policy acceptance, or anonymity.
#
# Pending separate validation, intentionally excluded:
# - service version: its tlsroots init opens ROOT using CertOpenSystemStore,
#   without explicit read-only/open-existing flags. A missing store may be
#   created by that API; no actual write has been established by this audit.
# - privacy detection: existing WFP metadata can lead to EXE path resolution.
#   Mapped/reparse targets need a local-path boundary review before inclusion.
# These are potential I/O boundaries, not evidence of an observed leak.
[CmdletBinding()]
param(
    [string]$PackageDirectory,
    [string]$ExpectedCliSha256,
    [string]$TimeoutSeconds = '20'
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$WarningPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$VerbosePreference = 'SilentlyContinue'
$DebugPreference = 'SilentlyContinue'

function Assert-LocalPath([string]$Path, [string]$Kind, [bool]$AllowMissing) {
    # Validate from the fixed local root downward BEFORE checking descendants.
    # Do not resolve UNC, device paths, ADS, junctions, symbolic links, or an
    # existing ancestor through a network share. Missing APPDATA/journal
    # descendants are checked up to the nearest existing parent, never created.
    if ([string]::IsNullOrEmpty($Path) -or $Path -notmatch '^[A-Za-z]:\\' -or
        $Path.Contains('/') -or $Path.Substring(2).Contains(':')) { throw 'local_path_required' }
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if ($part -eq '.' -or $part -eq '..' -or $part.EndsWith('.') -or $part.EndsWith(' ')) {
            throw 'ambiguous_path'
        }
    }
    $full = [IO.Path]::GetFullPath($Path)
    $root = [IO.Path]::GetPathRoot($full)
    $drive = New-Object IO.DriveInfo($root)
    if ($drive.DriveType -ne [IO.DriveType]::Fixed) { throw 'fixed_drive_required' }
    $parts = @($root)
    $current = $root
    foreach ($part in $full.Substring($root.Length).Split([char]'\')) {
        if ($part.Length -eq 0) { continue }
        $current = [IO.Path]::Combine($current, $part)
        $parts += $current
    }
    $lastAttributes = [IO.FileAttributes]::Directory
    foreach ($item in $parts) {
        try { $lastAttributes = [IO.File]::GetAttributes($item) }
        catch [IO.FileNotFoundException] {
            if ($AllowMissing) { return $full }
            throw 'required_item_missing'
        }
        catch [IO.DirectoryNotFoundException] {
            if ($AllowMissing) { return $full }
            throw 'required_item_missing'
        }
        if (($lastAttributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'reparse_path_rejected' }
        if ($item -ne $full -and ($lastAttributes -band [IO.FileAttributes]::Directory) -eq 0) {
            throw 'parent_is_not_directory'
        }
    }
    $isDirectory = ($lastAttributes -band [IO.FileAttributes]::Directory) -ne 0
    if (($Kind -eq 'directory' -and -not $isDirectory) -or ($Kind -eq 'file' -and $isDirectory)) {
        throw 'wrong_item_type'
    }
    return $full
}

function Get-TokenScope {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    try {
        $kind = 'current_token'
        if ($identity.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalSystemSid)) { $kind = 'SYSTEM' }
        elseif ($identity.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalServiceSid)) { $kind = 'service_token' }
        elseif ($identity.User.IsWellKnown([Security.Principal.WellKnownSidType]::NetworkServiceSid)) { $kind = 'service_token' }
        $interactive = $false
        foreach ($group in $identity.Groups) {
            if ($group.IsWellKnown([Security.Principal.WellKnownSidType]::InteractiveSid)) { $interactive = $true; break }
        }
        $principal = New-Object Security.Principal.WindowsPrincipal($identity)
        return [ordered]@{
            kind = $kind
            elevated = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
            interactiveSid = $interactive
            userScope = 'current_token_HKCU_and_APPDATA_only_not_GUI_user'
            helperStarted = $false
        }
    } finally { $identity.Dispose() }
}

function Invoke-FixedCli([string]$Exe, [string]$Directory, [string]$Command) {
    if ($Command -eq 'version') { $arguments = 'version' }
    elseif ($Command -eq 'privacy_report') { $arguments = 'privacy report' }
    else { throw 'command_not_allowed' }
    $info = New-Object Diagnostics.ProcessStartInfo
    $info.FileName = $Exe
    $info.Arguments = $arguments
    $info.WorkingDirectory = $Directory
    $info.UseShellExecute = $false # No shell, ShellExecute/runas, file association, or UAC.
    $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $info.StandardOutputEncoding = New-Object Text.UTF8Encoding($false, $true)
    $info.StandardErrorEncoding = New-Object Text.UTF8Encoding($false, $true)
    $process = New-Object Diagnostics.Process
    $process.StartInfo = $info
    try {
        if (-not $process.Start()) { throw 'cli_start_failed' }
        # Concurrent draining avoids stdout/stderr pipe deadlocks. The exact
        # trusted CLI has bounded report output; raw streams stay in memory.
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit($timeoutMilliseconds)) {
            try { $process.Kill(); [void]$process.WaitForExit(3000) } catch { }
            throw 'cli_timeout'
        }
        if (-not $stdout.Wait(3000) -or -not $stderr.Wait(3000)) { throw 'cli_output_timeout' }
        if ($process.ExitCode -ne 0) { throw 'cli_nonzero_exit' }
        if ($stderr.Result.Length -ne 0) { throw 'cli_stderr_present' }
        if ($stdout.Result.Length -gt 524288) { throw 'cli_output_too_large' }
        return $stdout.Result
    } finally { $process.Dispose() }
}

function Get-Property($Object, [string]$Name) {
    if ($null -eq $Object) { throw 'invalid_report_shape' }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) { throw 'missing_report_field' }
    return $property.Value
}

function Get-Boolean($Object, [string]$Name) {
    $value = Get-Property $Object $Name
    if ($value -isnot [bool]) { throw 'invalid_boolean_field' }
    return $value
}

function Measure-PolicyReport($Report) {
    # No raw title/detail/guide/warning/value/path/ID from the process is copied
    # to output. Only exact known IDs contribute to the policy aggregate.
    if ((Get-Property $Report 'platform') -cne 'windows') { throw 'unexpected_report_platform' }
    $allowed = @{}
    foreach ($browser in @('chrome', 'edge')) {
        foreach ($suffix in @('webrtc', 'prediction', 'suggestions', 'metrics', 'location', 'camera',
                'microphone', 'notifications', 'sensors', 'third-party-cookies', 'webgl')) {
            $allowed[($browser + '-' + $suffix)] = $true
        }
    }
    foreach ($id in @('windows-advertising-id', 'windows-online-speech', 'windows-clipboard-history', 'windows-clipboard-cloud')) {
        $allowed[$id] = $true
    }
    $statuses = [ordered]@{}
    foreach ($name in @('available', 'manual', 'configured', 'policy_conflict', 'unknown',
            'recovery_pending', 'external_change', 'unsupported', 'requires_admin', 'error', 'other')) { $statuses[$name] = 0 }
    $reads = [ordered]@{}
    foreach ($name in @('not_checked', 'different', 'match', 'machine_override', 'unknown', 'other')) { $reads[$name] = 0 }
    $controls = @(Get-Property $Report 'controls')
    if ($controls.Count -gt 256) { throw 'too_many_controls' }
    $seen = @{}
    $recognized = 0
    $other = 0
    $managed = 0
    $canApply = 0
    foreach ($control in $controls) {
        $id = Get-Property $control 'id'
        if ($id -isnot [string]) { throw 'invalid_control_id' }
        if (-not $allowed.ContainsKey($id)) { $other++; continue }
        if ($seen.ContainsKey($id)) { throw 'duplicate_control' }
        $seen[$id] = $true
        $recognized++
        $status = Get-Property $control 'status'
        if ($status -isnot [string] -or -not $statuses.Contains($status)) { $status = 'other' }
        $statuses[$status]++
        $read = Get-Property $control 'readStatus'
        if ($read -isnot [string] -or -not $reads.Contains($read)) { $read = 'other' }
        $reads[$read]++
        if (Get-Boolean $control 'managed') { $managed++ }
        if (Get-Boolean $control 'canApply') { $canApply++ }
    }
    return [ordered]@{
        platform = 'windows'
        observerElevated = Get-Boolean $Report 'observerElevated'
        observerAdmin = Get-Boolean $Report 'admin'
        backupHealthyForCurrentTokenUserJournal = Get-Boolean $Report 'backupHealthy'
        expectedPolicyCount = $allowed.Count
        recognizedPolicyCount = $recognized
        missingPolicyCount = $allowed.Count - $recognized
        guidanceOrOtherCount = $other
        managedCount = $managed
        canApplyCount = $canApply
        statusCounts = $statuses
        readStatusCounts = $reads
        runtimeProtection = 'not_tested'
        machineBackupInspection = 'not_performed'
        browserAcceptance = 'not_tested'
    }
}

$result = [ordered]@{
    schemaVersion = 1
    mode = 'hash_pinned_CLI_version_and_privacy_report_only'
    status = 'preflight_rejected'
    stage = 'preflight'
    cliHashMatched = $false
    localPathsChecked = $false
    token = $null
    version = $null
    report = $null
    limits = @('current_token_not_GUI_user', 'no_service_or_helper_contact',
        'no_detection_or_network_test', 'configuration_not_runtime_protection',
        'hash_not_package_signature', 'no_report_or_error_text_exported',
        'trusted_stage_and_ancestor_ACLs_verified_externally', 'leaf_handle_does_not_lock_parent_namespace')
}
$exitCode = 1
$binaryHandle = $null
try {
    if ($PSVersionTable.PSVersion -lt [version]'5.1' -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT -or
        -not [Environment]::Is64BitProcess) { throw 'unsupported_host' }
    # Validate inside the protected body so absent/invalid values do not make
    # parameter-validation errors print invocation paths or command contents.
    $timeoutValue = 0
    if (-not [int]::TryParse($TimeoutSeconds, [ref]$timeoutValue) -or $timeoutValue -lt 5 -or $timeoutValue -gt 30) {
        throw 'invalid_timeout'
    }
    $timeoutMilliseconds = $timeoutValue * 1000
    if ($ExpectedCliSha256 -notmatch '^[A-Fa-f0-9]{64}$') { throw 'invalid_trusted_hash' }
    $directory = Assert-LocalPath $PackageDirectory 'directory' $false
    $exe = Assert-LocalPath ([IO.Path]::Combine($directory, 'godusevpn-cli.exe')) 'file' $false
    # A fresh SYSTEM profile may not yet have its Roaming directory. Checking
    # its existing ancestors is enough for the read-only not-found journal path;
    # neither this script nor privacy report creates missing directories.
    $appData = Assert-LocalPath $env:APPDATA 'directory' $true
    $journal = [IO.Path]::Combine($appData, 'godusevpn-ultra\privacy-user\privacy-journal.json')
    [void](Assert-LocalPath $journal 'file' $true)
    $result.localPathsChecked = $true
    $result.token = Get-TokenScope

    # Hold a read/share-read handle until both invocations finish, so ordinary
    # writers cannot modify/delete this file between hashing and execution.
    # This does not prevent ANY permitted writer from changing a parent namespace;
    # private stage/ancestor ACLs are a separately verified external precondition.
    $binaryHandle = [IO.File]::Open($exe, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    if ($binaryHandle.Length -le 0 -or $binaryHandle.Length -gt 536870912) { throw 'unexpected_binary_size' }
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $actualHash = ([BitConverter]::ToString($sha.ComputeHash($binaryHandle))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
    if ($actualHash -cne $ExpectedCliSha256.ToLowerInvariant()) { throw 'trusted_hash_mismatch' }
    $result.cliHashMatched = $true

    $result.stage = 'cli_version'
    $result.status = 'command_failed'
    $versionText = Invoke-FixedCli $exe $directory 'version'
    $versionMatch = [regex]::Match($versionText, '\AGodUseVPN Ultra ([A-Za-z0-9][A-Za-z0-9.+_-]{0,79})\r?\n?\z')
    if (-not $versionMatch.Success) { throw 'unexpected_version_format' }
    $result.version = $versionMatch.Groups[1].Value

    $result.stage = 'privacy_report'
    $reportText = Invoke-FixedCli $exe $directory 'privacy_report'
    $result.stage = 'aggregate'
    $result.status = 'invalid_report'
    $report = ConvertFrom-Json -InputObject $reportText -ErrorAction Stop
    $result.report = Measure-PolicyReport $report
    $result.stage = 'complete'
    $result.status = 'completed'
    $exitCode = 0
} catch {
    # Never emit exception messages, invocation information, stderr, raw JSON,
    # paths, registry values, token identities, hostnames, or command output.
    # status/stage identify the failed fixed step without exporting its content.
} finally {
    if ($null -ne $binaryHandle) { $binaryHandle.Dispose() }
}
$result | ConvertTo-Json -Depth 8 -Compress
exit $exitCode
