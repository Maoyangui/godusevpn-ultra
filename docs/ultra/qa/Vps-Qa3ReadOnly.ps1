# Windows PowerShell 5.1 x64; fixed 0.01-beta-qa.3 candidate only.
# ROOT must separately approve the first execution of these exact three hashes.
# This script never downloads, extracts, repairs ACLs, creates files/directories,
# installs, elevates, starts the GUI/service/helper, or requests network tests.
# Exactly five child invocations are permitted, sequentially, with 15s each:
# CLI version; privacy report; privacy detection; privacy firewall-list; svc version.
# All other executable files and command lines are refused. No retry on unknown.
#
# The existing package must be a delta-<32 hex> directory inside the fixed QA
# StageID (UUID4 hex) under local ProgramData/GodUseVPN-Ultra-QA. QA directories
# and three executables must have SY/BA owner and no allow ACE for another SID.
# Parent handles are opened root-first, no-follow, read/share-read and retained.
# This is not protection against hostile administrators/kernel/DOS-map changes.
# APPDATA checks concern only this token, never another user's/GUI user's profile.
# Report/error strings, paths, account/SID, IP/MAC and environment values are not
# emitted. Windows may still record ordinary execution/security/AV events.
# JSON-only output assumes a syntactically valid invocation; parameter-binding
# errors before the script body belong to PowerShell, not this output contract.
[CmdletBinding()]
param([string]$StageID, [string]$CandidateLeaf)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$WarningPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$VerbosePreference = 'SilentlyContinue'
$DebugPreference = 'SilentlyContinue'
$fixedVersion = '0.01-beta-qa.3'
$fixedHashes = @{
    cli = '3a17693ee9e80c865b9bee92ee34f403adeb75883a797b821e6123e9edabfae8'
    svc = '2ebd104afd943353f18662510e751acf9421e03fa7cca834d516444b98f5127e'
    gui = '2293b29ed415e7ef972208e54f690437f61a6e195f35c9da0f4bdf9a7c7caa15'
}

function New-BetaNative {
    # Reflection.Emit is in-memory; no Add-Type compiler or temporary source.
    $an = New-Object Reflection.AssemblyName('UltraBetaReadOnly')
    $ab = [AppDomain]::CurrentDomain.DefineDynamicAssembly($an, [Reflection.Emit.AssemblyBuilderAccess]::Run)
    $tb = $ab.DefineDynamicModule('Native').DefineType('UltraBetaReadOnlyNative', [Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $specs = @(
        @('CreateFileW', [IntPtr], [type[]]@([string],[uint32],[uint32],[IntPtr],[uint32],[uint32],[IntPtr])),
        @('GetFileInformationByHandleEx', [bool], [type[]]@([IntPtr],[int],[IntPtr],[uint32])),
        @('CloseHandle', [bool], [type[]]@([IntPtr])),
        @('QueryDosDeviceW', [uint32], [type[]]@([string],[Text.StringBuilder],[uint32]))
    )
    $ctor = [Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields = [Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'), [Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'), [Runtime.InteropServices.DllImportAttribute].GetField('CharSet'), [Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    foreach ($spec in $specs) {
        $m = $tb.DefineMethod($spec[0], [Reflection.MethodAttributes]'Public, Static, PinvokeImpl', $spec[1], $spec[2])
        $m.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $attribute = New-Object Reflection.Emit.CustomAttributeBuilder($ctor, [object[]]@('kernel32.dll'), $fields, [object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi))
        $m.SetCustomAttribute($attribute)
    }
    return $tb.CreateType()
}

function Get-BetaComponents([string]$Path) {
    if (-not $Path -or $Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or $Path.Substring(2).Contains(':') -or $Path.Length -gt 30000) { throw 'path_rejected' }
    $root = $Path.Substring(0,3)
    $parts = @($root); $current = $root
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if (-not $part.Length -or $part -eq '.' -or $part -eq '..' -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'path_rejected' }
        foreach ($ch in $part.ToCharArray()) { if ([int]$ch -lt 32) { throw 'path_rejected' } }
        $base = $part.Split([char]'.')[0].ToUpperInvariant()
        if ($base -match '^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])$') { throw 'path_rejected' }
        $current = [IO.Path]::Combine($current,$part); $parts += $current
    }
    return $parts
}

function Assert-BetaLocalDrive([string]$Root) {
    $mapping = New-Object Text.StringBuilder(32768)
    if ($script:betaNative::QueryDosDeviceW($Root.Substring(0,2),$mapping,32768) -eq 0 -or $mapping.ToString() -notmatch '^\\Device\\HarddiskVolume[0-9]+$') { throw 'drive_rejected' }
    if ((New-Object IO.DriveInfo($Root)).DriveType -ne [IO.DriveType]::Fixed) { throw 'drive_rejected' }
}

function Get-BetaHandleAttributes([IntPtr]$Handle) {
    $buffer = [Runtime.InteropServices.Marshal]::AllocHGlobal(8)
    try {
        if (-not $script:betaNative::GetFileInformationByHandleEx($Handle,9,$buffer,8)) { throw 'handle_check_failed' }
        return [Runtime.InteropServices.Marshal]::ReadInt32($buffer)
    } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
}

function Assert-BetaPrivateSD([string]$Sddl) {
    $sd = New-Object Security.AccessControl.RawSecurityDescriptor($Sddl)
    if ($null -eq $sd.Owner -or $sd.Owner.Value -notin @('S-1-5-18','S-1-5-32-544') -or $null -eq $sd.DiscretionaryAcl) { throw 'private_acl_rejected' }
    foreach ($ace in $sd.DiscretionaryAcl) {
        if ($ace -isnot [Security.AccessControl.CommonAce] -or $ace.IsCallback) { throw 'private_acl_rejected' }
        if ($ace.AceQualifier -eq [Security.AccessControl.AceQualifier]::AccessAllowed -and $ace.SecurityIdentifier.Value -notin @('S-1-5-18','S-1-5-32-544')) { throw 'private_acl_rejected' }
    }
}

function Assert-BetaPrivatePath([string]$Path) {
    $acl = Get-Acl -LiteralPath $Path -ErrorAction Stop
    $parts = [Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access
    Assert-BetaPrivateSD ($acl.GetSecurityDescriptorSddlForm($parts))
}

function Lock-BetaDirectories([string]$Path,[bool]$AllowMissing,[string]$PrivateRoot) {
    $parts = @(Get-BetaComponents $Path)
    Assert-BetaLocalDrive $parts[0]
    for ($i=0; $i -lt $parts.Count; $i++) {
        $part = $parts[$i]
        $h = $script:betaNative::CreateFileW($part,0x81,1,[IntPtr]::Zero,3,0x02200000,[IntPtr]::Zero)
        if ($h -eq [IntPtr](-1)) {
            $code = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
            if ($AllowMissing -and $i -gt 0 -and $code -in @(2,3)) { return }
            throw 'directory_open_failed'
        }
        [void]$script:betaHeld.Add($h)
        $attrs = Get-BetaHandleAttributes $h
        if (($attrs -band 0x400) -ne 0 -or ($attrs -band 0x10) -eq 0) { throw 'directory_rejected' }
        if ($PrivateRoot -and ($part.Equals($PrivateRoot,[StringComparison]::OrdinalIgnoreCase) -or $part.StartsWith($PrivateRoot+'\',[StringComparison]::OrdinalIgnoreCase))) { Assert-BetaPrivatePath $part }
    }
}

function Get-BetaToken {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    try {
        $kind = 'current_token'
        if ($id.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalSystemSid)) { $kind='SYSTEM' }
        elseif ($id.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalServiceSid) -or $id.User.IsWellKnown([Security.Principal.WellKnownSidType]::NetworkServiceSid)) { $kind='service_token' }
        $interactive=$false
        foreach ($group in $id.Groups) { if ($group.IsWellKnown([Security.Principal.WellKnownSidType]::InteractiveSid)) { $interactive=$true; break } }
        $principal=New-Object Security.Principal.WindowsPrincipal($id)
        return [ordered]@{kind=$kind;elevated=$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator);interactiveSid=$interactive}
    } finally { $id.Dispose() }
}

function Get-BetaPlan {
    return @(
        [pscustomobject]@{id='cli_version';role='cli';arguments='version'},
        [pscustomobject]@{id='privacy_report';role='cli';arguments='privacy report'},
        [pscustomobject]@{id='privacy_detection';role='cli';arguments='privacy detection'},
        [pscustomobject]@{id='privacy_firewall_list';role='cli';arguments='privacy firewall-list'},
        [pscustomobject]@{id='svc_version';role='svc';arguments='version'}
    )
}

function Invoke-BetaProcess([string]$Exe,[string]$Directory,$Step) {
    $allowed = @(Get-BetaPlan | Where-Object { $_.id -ceq $Step.id -and $_.role -ceq $Step.role -and $_.arguments -ceq $Step.arguments })
    if ($allowed.Count -ne 1) { throw 'command_rejected' }
    $info=New-Object Diagnostics.ProcessStartInfo
    $info.FileName=$Exe; $info.Arguments=$Step.arguments; $info.WorkingDirectory=$Directory
    $info.UseShellExecute=$false; $info.CreateNoWindow=$true
    $info.RedirectStandardOutput=$true; $info.RedirectStandardError=$true
    $process=New-Object Diagnostics.Process; $process.StartInfo=$info
    $memory=New-Object IO.MemoryStream; $started=$false; $complete=$false
    try {
        if (-not $process.Start()) { throw 'process_start_failed' }
        $started=$true; $script:betaStarted++
        $watch=[Diagnostics.Stopwatch]::StartNew()
        $outBuffer=New-Object byte[] 65536; $errBuffer=New-Object byte[] 4096
        $outTask=$process.StandardOutput.BaseStream.ReadAsync($outBuffer,0,$outBuffer.Length)
        $errTask=$process.StandardError.BaseStream.ReadAsync($errBuffer,0,$errBuffer.Length)
        while ($null -ne $outTask -or $null -ne $errTask -or -not $process.HasExited) {
            if ($watch.ElapsedMilliseconds -ge 15000) { throw 'process_timeout' }
            if ($null -ne $outTask -and $outTask.IsCompleted) {
                $n=$outTask.GetAwaiter().GetResult()
                if ($n -eq 0) { $outTask=$null }
                else {
                    if ($memory.Length+$n -gt 1048576) { throw 'output_limit' }
                    $memory.Write($outBuffer,0,$n)
                    $outTask=$process.StandardOutput.BaseStream.ReadAsync($outBuffer,0,$outBuffer.Length)
                }
            }
            if ($null -ne $errTask -and $errTask.IsCompleted) {
                $n=$errTask.GetAwaiter().GetResult()
                if ($n -gt 0) { throw 'stderr_present' }
                $errTask=$null
            }
            if ($null -ne $outTask -or $null -ne $errTask -or -not $process.HasExited) { [Threading.Thread]::Sleep(10) }
        }
        $process.WaitForExit()
        if ($process.ExitCode -ne 0) { throw 'process_nonzero' }
        $complete=$true
        return (New-Object Text.UTF8Encoding($false,$true)).GetString($memory.ToArray())
    } finally {
        if ($started -and -not $complete) {
            try {
                if (-not $process.HasExited) { $process.Kill() }
                if (-not $process.WaitForExit(3000) -or -not $process.HasExited) { $script:betaTerminationUnknown=$true }
            } catch { $script:betaTerminationUnknown=$true }
        }
        $memory.Dispose(); $process.Dispose()
    }
}

function Get-BetaField($Value,[string]$Name) {
    if ($null -eq $Value -or $null -eq $Value.PSObject.Properties[$Name]) { throw 'report_shape_rejected' }
    return ,$Value.PSObject.Properties[$Name].Value
}
function Get-BetaBool($Value,[string]$Name) {
    $v=Get-BetaField $Value $Name
    if ($v -isnot [bool]) { throw 'report_shape_rejected' }
    return $v
}
function Get-BetaCount($Value) {
    if (($Value -isnot [int] -and $Value -isnot [long]) -or $Value -lt 0 -or $Value -gt 1000000) { throw 'report_shape_rejected' }
    return [int]$Value
}
function Assert-BetaUserScope($Value) {
    if ((Get-BetaField $Value 'backupScope') -cne 'current_process_user' -or (Get-BetaField $Value 'managedScope') -cne 'current_process_user_journal' -or (Get-BetaBool $Value 'machineJournalChecked') -or (Get-BetaField $Value 'machineJournalStatus') -cne 'not_inspected') { throw 'report_scope_rejected' }
}
function Measure-BetaPolicy($Value) {
    if ((Get-BetaField $Value 'platform') -cne 'windows') { throw 'report_shape_rejected' }
    Assert-BetaUserScope $Value
    $controls=Get-BetaField $Value 'controls'
    if ($controls -isnot [array] -or $controls.Count -gt 256) { throw 'report_shape_rejected' }
    $managed=0; $canApply=0; $unknown=0
    foreach ($c in $controls) {
        if (Get-BetaBool $c 'managed') { $managed++ }
        if (Get-BetaBool $c 'canApply') { $canApply++ }
        if ((Get-BetaField $c 'status') -notin @('available','manual','configured','policy_conflict','recovery_pending','external_change','unsupported','requires_admin')) { $unknown++ }
    }
    return [ordered]@{controls=$controls.Count;managed=$managed;canApply=$canApply;unknownOrOther=$unknown;observerElevated=(Get-BetaBool $Value 'observerElevated');backupHealthyCurrentTokenOnly=(Get-BetaBool $Value 'backupHealthy');machineJournalChecked=$false}
}
function Measure-BetaDetection($Value) {
    Assert-BetaUserScope $Value
    if ((Get-BetaField $Value 'platform') -cne 'windows' -or (Get-BetaField $Value 'executionMode') -cne 'direct_cli' -or (Get-BetaBool $Value 'helperAuthorized')) { throw 'report_scope_rejected' }
    $findings=Get-BetaField $Value 'findings'
    if ($findings -isnot [array] -or $findings.Count -gt 1024) { throw 'report_shape_rejected' }
    $counts=[ordered]@{observed=0;allowed=0;possible=0;restricted=0;unknown=0;manual=0;other=0}
    foreach ($f in $findings) { $s=Get-BetaField $f 'status'; if ($s -isnot [string] -or -not $counts.Contains($s)) { $s='other' }; $counts[$s]++ }
    return [ordered]@{findings=$findings.Count;statuses=$counts;observerElevated=(Get-BetaBool $Value 'admin');helperAuthorized=$false;machineJournalChecked=$false}
}
function Measure-BetaFirewall($Value) {
    $rules=Get-BetaField $Value 'rules'
    if ($rules -isnot [array] -or $rules.Count -gt 10000) { throw 'report_shape_rejected' }
    $installed=0; $partial=0; $other=0
    foreach ($r in $rules) {
        $s=Get-BetaField $r 'state'
        if ($s -ceq 'filters_installed') { $installed++ } elseif ($s -ceq 'partial') { $partial++ } else { $other++ }
    }
    return [ordered]@{supported=(Get-BetaBool $Value 'supported');observerElevated=(Get-BetaBool $Value 'elevated');rules=$rules.Count;unknownFilters=(Get-BetaCount (Get-BetaField $Value 'unknownFilters'));filtersInstalled=$installed;partial=$partial;other=$other;runtimeTested=$false}
}
function Confirm-BetaVersion([string]$Text) {
    if ($Text -cnotmatch '\AGodUseVPN Ultra 0\.01-beta-qa\.3\r?\n?\z') { throw 'version_rejected' }
    return '0.01-beta-qa.3'
}

$result=[ordered]@{schema='ultra-qa3-readonly-v1';candidate=$fixedVersion;status='rejected';stage='preflight';error='';hashesMatched=0;staticVersionsMatched=0;commandsStarted=0;commandsPassed=0;terminationUnknown=$false;token=$null;cliVersion=$null;svcVersion=$null;policy=$null;detection=$null;firewall=$null;guiStarted=$false;serviceStarted=$false;settingsOrNetworkChangesRequested=$false;runtimeProtectionTested=$false;scope='current_token_not_GUI_user';machineJournalChecked=$false}
$script:betaHeld=New-Object 'Collections.Generic.List[IntPtr]'
$script:betaStarted=0; $script:betaTerminationUnknown=$false
$streams=@(); $exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'host_rejected' }
    if ($StageID -cnotmatch '^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$' -or $CandidateLeaf -cnotmatch '^delta-[a-f0-9]{32}$') { throw 'stage_rejected' }
    $script:betaNative=New-BetaNative
    $programData=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData,[Environment+SpecialFolderOption]::DoNotVerify)
    $qaRoot=[IO.Path]::Combine($programData,'GodUseVPN-Ultra-QA')
    $candidate=[IO.Path]::Combine([IO.Path]::Combine($qaRoot,$StageID),$CandidateLeaf)
    Lock-BetaDirectories $candidate $false $qaRoot
    $userJournalParent=[IO.Path]::Combine($env:APPDATA,'godusevpn-ultra\privacy-user')
    Lock-BetaDirectories $userJournalParent $true ''
    $result.token=Get-BetaToken
    $paths=@{}
    foreach ($role in @('cli','svc','gui')) {
        $name=@{cli='godusevpn-cli.exe';svc='godusevpn-svc.exe';gui='godusevpn.exe'}[$role]
        $exe=[IO.Path]::Combine($candidate,$name)
        # The parent has already been pinned root-first. The leaf is opened
        # no-follow before path-based ACL/version reads, then retained read-only.
        $h=$script:betaNative::CreateFileW($exe,[uint32]2147483776,1,[IntPtr]::Zero,3,0x00200000,[IntPtr]::Zero)
        if ($h -eq [IntPtr](-1)) { throw 'binary_open_failed' }
        [void]$script:betaHeld.Add($h)
        $attrs=Get-BetaHandleAttributes $h
        if (($attrs -band 0x410) -ne 0) { throw 'binary_rejected' }
        Assert-BetaPrivatePath $exe
        $stream=[IO.File]::Open($exe,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
        $streams += $stream
        if ($stream.Length -lt 1 -or $stream.Length -gt 134217728) { throw 'binary_rejected' }
        $sha=[Security.Cryptography.SHA256]::Create()
        try { $hash=([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
        if ($hash -cne $fixedHashes[$role]) { throw 'hash_rejected' }
        $result.hashesMatched++
        if ([Diagnostics.FileVersionInfo]::GetVersionInfo($exe).ProductVersion -cne $fixedVersion) { throw 'static_version_rejected' }
        $result.staticVersionsMatched++
        $paths[$role]=$exe
    }
    foreach ($step in @(Get-BetaPlan)) {
        $result.stage=$step.id
        $text=Invoke-BetaProcess $paths[$step.role] $candidate $step
        switch ($step.id) {
            'cli_version' { $result.cliVersion=Confirm-BetaVersion $text }
            'svc_version' { $result.svcVersion=Confirm-BetaVersion $text }
            'privacy_report' { $result.policy=Measure-BetaPolicy (ConvertFrom-Json -InputObject $text -ErrorAction Stop) }
            'privacy_detection' { $result.detection=Measure-BetaDetection (ConvertFrom-Json -InputObject $text -ErrorAction Stop) }
            'privacy_firewall_list' { $result.firewall=Measure-BetaFirewall (ConvertFrom-Json -InputObject $text -ErrorAction Stop) }
            default { throw 'command_rejected' }
        }
        $result.commandsPassed++
    }
    $result.status='completed';$result.stage='complete';$exitCode=0
} catch {
    $known=@('host_rejected','stage_rejected','path_rejected','drive_rejected','handle_check_failed','private_acl_rejected','directory_open_failed','directory_rejected','binary_open_failed','binary_rejected','hash_rejected','static_version_rejected','command_rejected','process_start_failed','process_timeout','output_limit','stderr_present','process_nonzero','report_shape_rejected','report_scope_rejected','version_rejected')
    $code=$_.Exception.Message
    if ($known -cnotcontains $code) { $code='check_failed' }
    $result.error=$code
} finally {
    foreach ($stream in $streams) { $stream.Dispose() }
    for ($i=$script:betaHeld.Count-1;$i -ge 0;$i--) { [void]$script:betaNative::CloseHandle($script:betaHeld[$i]) }
    $result.commandsStarted=$script:betaStarted; $result.terminationUnknown=$script:betaTerminationUnknown
}
$json=$result | ConvertTo-Json -Compress -Depth 8
if ($json.Length -gt 3000) { $json='{"schema":"ultra-qa3-readonly-v1","status":"rejected","error":"summary_limit"}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode
