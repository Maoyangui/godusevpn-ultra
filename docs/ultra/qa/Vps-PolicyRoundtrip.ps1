# Windows PowerShell 5.1 x64. Explicit, separately approved disposable-VPS test.
# Only windows-clipboard-history: apply -> independent HKLM read -> restore.
# No GUI/helper/service/install/browser/network/tunnel/detection commands.
# Requires an elevated INTERACTIVE token; SYSTEM is deliberately refused.
# Refuses an existing target value or machine journal. Never removes files,
# edits a registry value directly, resets firewall, or overwrites a conflict.
# Product backup/lock directories may remain after successful restoration.
# A trusted administrator must keep the local namespace stable during testing.
[CmdletBinding()]
param([string]$StageID)
Set-StrictMode -Version 2
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$WarningPreference = 'SilentlyContinue'

function Assert-Local([string]$Path, [bool]$AllowMissing, [bool]$Private, [bool]$AllowTestAdmin = $false) {
    if ($Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or $Path.Substring(2).Contains(':')) { throw 'local_path_required' }
    $full = [IO.Path]::GetFullPath($Path)
    if ($full -cne $Path) { throw 'ambiguous_path' }
    $root = [IO.Path]::GetPathRoot($full)
    if ((New-Object IO.DriveInfo($root)).DriveType -ne [IO.DriveType]::Fixed) { throw 'fixed_drive_required' }
    $current = $root
    $parts = @($root)
    foreach ($part in $full.Substring($root.Length).Split([char]'\')) {
        if (-not $part.Length -or $part.EndsWith('.') -or $part.EndsWith(' ')) { throw 'ambiguous_path' }
        $current = [IO.Path]::Combine($current, $part)
        $parts += $current
    }
    foreach ($item in $parts) {
        try { $attributes = [IO.File]::GetAttributes($item) }
        catch [IO.FileNotFoundException] { if ($AllowMissing) { return $full }; throw }
        catch [IO.DirectoryNotFoundException] { if ($AllowMissing) { return $full }; throw }
        if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'reparse_rejected' }
        if ($item -ne $full -and ($attributes -band [IO.FileAttributes]::Directory) -eq 0) { throw 'invalid_parent' }
    }
    if ($Private) {
        $acl = Get-Acl -LiteralPath $full
        $trusted = @('S-1-5-18', 'S-1-5-32-544')
        # v1 privacy.lockStore adds the executing administrator's SID. Accept
        # that exact pre-checked identity only for its generated journal below.
        # QA stage/package paths never use this exception; no ACL is changed.
        if ($AllowTestAdmin) { $trusted += $script:testAdminSID }
        if ($acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -notin $trusted) { throw 'untrusted_owner' }
        $write = [Security.AccessControl.FileSystemRights]::Write -bor [Security.AccessControl.FileSystemRights]::Delete -bor
            [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
            [Security.AccessControl.FileSystemRights]::TakeOwnership
        foreach ($rule in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) {
            if ($rule.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and
                ($rule.FileSystemRights -band $write) -ne 0 -and $rule.IdentityReference.Value -notin $trusted) { throw 'untrusted_write_acl' }
        }
    }
    return $full
}

function Invoke-Policy([string]$Action) {
    if ($Action -notin @('apply','restore')) { throw 'action_not_allowed' }
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $script:cli
    $start.Arguments = 'privacy ' + $Action + ' windows-clipboard-history'
    $start.WorkingDirectory = [IO.Path]::GetDirectoryName($script:cli)
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.StandardOutputEncoding = New-Object Text.UTF8Encoding($false,$true)
    $start.StandardErrorEncoding = New-Object Text.UTF8Encoding($false,$true)
    $process = New-Object Diagnostics.Process
    $process.StartInfo = $start
    try {
        if (-not $process.Start()) { throw 'cli_start_failed' }
        $out = $process.StandardOutput.ReadToEndAsync()
        $err = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(20000)) {
            $script:terminationUnknown = $true
            try {
                $process.Kill()
                if ($process.WaitForExit(3000) -and $process.HasExited) { $script:terminationUnknown = $false }
            } catch { }
            throw 'cli_timeout'
        }
        if (-not $out.Wait(3000) -or -not $err.Wait(3000)) { throw 'output_timeout' }
        if ($process.ExitCode -ne 0 -or $err.Result.Length -ne 0 -or $out.Result.Length -gt 8192) { throw 'cli_failed' }
        $data = ConvertFrom-Json -InputObject $out.Result
        if ($data.id -cne 'windows-clipboard-history' -or $data.changed -isnot [bool]) { throw 'invalid_result' }
        if (($Action -eq 'apply' -and $data.status -cne 'configured') -or
            ($Action -eq 'restore' -and $data.status -cne 'restored')) { throw 'unexpected_status' }
        return $data
    } finally { $process.Dispose() }
}

function Read-Policy {
    $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine,[Microsoft.Win32.RegistryView]::Registry64)
    try {
        $key = $base.OpenSubKey('SOFTWARE\Policies\Microsoft\Windows\System',$false)
        if ($null -eq $key) { return @{exists=$false;isZero=$false} }
        try {
            if ($key.GetValueNames() -notcontains 'AllowClipboardHistory') { return @{exists=$false;isZero=$false} }
            $zero = $key.GetValueKind('AllowClipboardHistory') -eq [Microsoft.Win32.RegistryValueKind]::DWord -and
                $key.GetValue('AllowClipboardHistory',$null,[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) -eq 0
            return @{exists=$true;isZero=[bool]$zero}
        } finally { $key.Dispose() }
    } finally { $base.Dispose() }
}

$result = [ordered]@{schemaVersion=1;test='single_machine_policy_roundtrip';policy='windows-clipboard-history';status='preflight_failed';stage='preflight';
    cliHashMatched=$false;interactiveAdmin=$false;initialValueAbsent=$false;initialJournalAbsent=$false;
    applyConfirmed=$false;independentReadMatched=$false;backupObserved=$false;restoreAttempted=$false;
    restoreConfirmed=$false;initialValueRestored=$false;journalEntryRemoved=$false;
    tunnelTested=$false;runtimeClipboardBehaviorTested=$false;networkChanged=$false}
$binary = $null
$attempted = $false
$script:terminationUnknown = $false
$exitCode = 1
try {
    if (-not [Environment]::Is64BitProcess -or $StageID -notmatch '^[a-f0-9]{32}$') { throw 'invalid_host_or_stage' }
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    try {
        $principal = New-Object Security.Principal.WindowsPrincipal($id)
        $interactive = @($id.Groups | Where-Object { $_.IsWellKnown([Security.Principal.WellKnownSidType]::InteractiveSid) }).Count -gt 0
        if (-not $interactive -or -not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) -or
            $id.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalSystemSid)) { throw 'interactive_admin_required' }
        $script:testAdminSID = $id.User.Value # Local ACL comparison only; never exported.
        $result.interactiveAdmin = $true
    } finally { $id.Dispose() }
    $osBase = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine,[Microsoft.Win32.RegistryView]::Registry64)
    try {
        $osKey = $osBase.OpenSubKey('SOFTWARE\Microsoft\Windows NT\CurrentVersion',$false)
        try { $edition = [string]$osKey.GetValue('EditionID','') } finally { $osKey.Dispose() }
    } finally { $osBase.Dispose() }
    if ($edition -notmatch '^(Professional|Enterprise|Education|IoTEnterprise)' -or [Environment]::OSVersion.Version.Build -lt 17763) { throw 'unsupported_policy_edition_or_build' }
    $programData = [Environment]::GetFolderPath('CommonApplicationData')
    $qa = Assert-Local ([IO.Path]::Combine($programData,'GodUseVPN-Ultra-QA')) $false $true
    $stage = Assert-Local ([IO.Path]::Combine($qa,$StageID)) $false $true
    $package = Assert-Local ([IO.Path]::Combine($stage,'cli-smoke')) $false $true
    $script:cli = Assert-Local ([IO.Path]::Combine($package,'godusevpn-cli.exe')) $false $true
    [void](Assert-Local $env:APPDATA $true $false)
    [void](Assert-Local ([IO.Path]::Combine($env:APPDATA,'godusevpn-ultra\privacy-user\privacy-journal.json')) $true $false)
    $machine = Assert-Local ([IO.Path]::Combine($programData,'godusevpn-ultra\privacy-machine')) $true $true
    $journal = Assert-Local ([IO.Path]::Combine($machine,'privacy-journal.json')) $true $true
    if ([IO.File]::Exists($journal) -or [IO.Directory]::Exists($journal)) { throw 'existing_machine_journal_requires_review' }
    $result.initialJournalAbsent = $true
    if ((Read-Policy).exists) { throw 'existing_policy_requires_review' }
    $result.initialValueAbsent = $true
    $binary = [IO.File]::Open($script:cli,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
    $hash = [Security.Cryptography.SHA256]::Create()
    try { $actual = ([BitConverter]::ToString($hash.ComputeHash($binary))).Replace('-','').ToLowerInvariant() }
    finally { $hash.Dispose() }
    if ($actual -cne 'bf728ab1a000b4a8aae5329d7ba76cc25aae4c7a87667fecc0bfdf4d9f5ffaa1') { throw 'cli_hash_mismatch' }
    $result.cliHashMatched = $true
    $result.stage = 'apply'
    $result.status = 'test_failed'
    $attempted = $true
    $action = Invoke-Policy 'apply'
    if (-not $action.changed) { throw 'expected_new_write' }
    $result.applyConfirmed = $true
    $result.stage = 'independent_read'
    if (-not (Read-Policy).isZero) { throw 'policy_read_mismatch' }
    $result.independentReadMatched = $true
    [void](Assert-Local $journal $false $true $true)
    if (([IO.File]::GetAttributes($journal) -band [IO.FileAttributes]::Directory) -ne 0) { throw 'journal_not_file' }
    $saved = ConvertFrom-Json -InputObject ([IO.File]::ReadAllText($journal))
    if ($saved.payload.controls.'windows-clipboard-history'.stage -cne 'active') { throw 'backup_not_active' }
    $result.backupObserved = $true
} catch {
    # Fixed fields only. No exception messages, registry contents, SID or paths.
} finally {
    if ($attempted -and -not $script:terminationUnknown) {
        $result.restoreAttempted = $true
        try {
            $restore = Invoke-Policy 'restore'
            $result.restoreConfirmed = $true
            $result.initialValueRestored = -not (Read-Policy).exists
            [void](Assert-Local $journal $false $true $true)
            $savedAfter = ConvertFrom-Json -InputObject ([IO.File]::ReadAllText($journal))
            $result.journalEntryRemoved = $null -eq $savedAfter.payload.controls.PSObject.Properties['windows-clipboard-history']
        } catch { $result.status = 'recovery_pending' }
    }
    if ($script:terminationUnknown) { $result.status = 'recovery_pending'; $result.stage = 'process_termination_unknown' }
    if ($null -ne $binary) { $binary.Dispose() }
}
if ($result.applyConfirmed -and $result.independentReadMatched -and $result.backupObserved -and
    $result.restoreConfirmed -and $result.initialValueRestored -and $result.journalEntryRemoved) {
    $result.status = 'completed'; $result.stage = 'complete'; $exitCode = 0
}
$result | ConvertTo-Json -Compress
exit $exitCode
