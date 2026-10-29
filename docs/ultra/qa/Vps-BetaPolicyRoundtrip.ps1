# Windows PowerShell 5.1 x64, SYSTEM, fixed 0.01-beta-qa.2 CLI only.
# Preparation is not authorization to execute this script. ROOT must obtain the
# user's explicit approval for four fixed HKLM apply/readback/restore rounds.
# No GUI/service/helper/UAC/network is started. No ACL repair, archive, deletion,
# force or restore-all command exists. Unknown/timeout stops with no retry.
# A finally restore is permitted once only after this process received confirmed
# changed=true and independently saw the exact active, originally-absent backup.
# Existing conflicting values/journals are retained for deliberate review.
# Native directory/EXE pins protect the reviewed staging path; machine snapshots
# close their read leases before CLI writes. Hostile administrator/namespace
# races are outside this QA helper's guarantee. Only fixed JSON counts/statuses
# are emitted; raw output, journal, account, path and registry values stay local.
[CmdletBinding()]
param([string]$StageID,[string]$CandidateLeaf)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
$VerbosePreference='SilentlyContinue'
$DebugPreference='SilentlyContinue'
$fixedCliHash='7eed52a1e1e6f4424db54f4a2b067346f2dc7bd9df734687529422294cd9673e'

function Get-RoundPolicies {
    return @(
        [pscustomobject]@{id='windows-advertising-id';path='SOFTWARE\Policies\Microsoft\Windows\AdvertisingInfo';name='DisabledByGroupPolicy';desired=1},
        [pscustomobject]@{id='windows-online-speech';path='SOFTWARE\Policies\Microsoft\InputPersonalization';name='AllowInputPersonalization';desired=0},
        [pscustomobject]@{id='windows-clipboard-history';path='SOFTWARE\Policies\Microsoft\Windows\System';name='AllowClipboardHistory';desired=0},
        [pscustomobject]@{id='windows-clipboard-cloud';path='SOFTWARE\Policies\Microsoft\Windows\System';name='AllowCrossDeviceClipboard';desired=0}
    )
}
function Get-RoundPayload($Policy) {
    $controls=[ordered]@{}
    if ($null -ne $Policy) {
        $controls[$Policy.id]=[ordered]@{stage='active';changes=@([ordered]@{
            location=[ordered]@{hive='HKLM';path=$Policy.path;name=$Policy.name}
            before=[ordered]@{exists=$false;type=0}
            desired=[ordered]@{exists=$true;type=4;data=[Convert]::ToBase64String([BitConverter]::GetBytes([uint32]$Policy.desired))}
        })}
    }
    return ([ordered]@{version=1;userKey='machine-policy-store-v1';controls=$controls} | ConvertTo-Json -Compress -Depth 10)
}
function Get-RoundHash([byte[]]$Bytes) {
    $hash=[Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($hash.ComputeHash($Bytes))).Replace('-','').ToLowerInvariant() }
    finally { $hash.Dispose() }
}
function Read-RoundEnvelope([byte[]]$Bytes,$Policy) {
    if ($Bytes.Length -lt 1 -or $Bytes.Length -gt 4194304) { throw 'journal_requires_review' }
    $text=(New-Object Text.UTF8Encoding($false,$true)).GetString($Bytes)
    $match=[regex]::Match($text,'\A\{"payload":(?<payload>\{.*\}),"sha256":"(?<sha>[a-f0-9]{64})"\}\z',[Text.RegularExpressions.RegexOptions]::Singleline)
    if (-not $match.Success) { throw 'journal_requires_review' }
    $payload=$match.Groups['payload'].Value
    if ((Get-RoundHash ([Text.Encoding]::UTF8.GetBytes($payload))) -cne $match.Groups['sha'].Value) { throw 'journal_requires_review' }
    # Strict canonical current writer output: no ambiguous duplicate/unknown
    # fields, user journal or historical account owner can be treated as empty.
    if ($payload -ceq (Get-RoundPayload $null)) { return 'empty' }
    if ($null -ne $Policy -and $payload -ceq (Get-RoundPayload $Policy)) { return 'active' }
    throw 'journal_requires_review'
}
function Assert-RoundAction($Action,$Policy,[string]$Mode) {
    if ($null -eq $Action -or $Action.PSObject.Properties['id'] -eq $null -or $Action.id -cne $Policy.id -or $Action.PSObject.Properties['changed'] -eq $null -or $Action.changed -isnot [bool] -or $Action.PSObject.Properties['status'] -eq $null) { throw 'action_result_unknown' }
    if ($Mode -ceq 'apply' -and ($Action.status -cne 'configured' -or -not $Action.changed)) { throw 'apply_not_confirmed' }
    if ($Mode -ceq 'restore' -and $Action.status -cne 'restored') { throw 'restore_not_confirmed' }
}
function Assert-RoundEmpty($Snapshot) {
    if ($null -eq $Snapshot -or $Snapshot.state -cne 'empty') { throw 'journal_requires_review' }
}

# State machine is isolated for synthetic tests. Native callbacks in main are
# fixed below; callers cannot supply executable/path/registry targets via args.
function Invoke-RoundBatch([scriptblock]$ReadPolicy,[scriptblock]$ReadJournal,[scriptblock]$Run) {
    $policies=@(Get-RoundPolicies)
    $results=New-Object 'Collections.Generic.List[object]'
    $summary=[ordered]@{status='rejected';initialAbsentCount=0;initialJournalClean=$false;completed=0;results=@();error=''}
    try {
        foreach ($p in $policies) { $v=&$ReadPolicy $p;if ($v.exists) { throw 'initial_policy_exists' };$summary.initialAbsentCount++ }
        Assert-RoundEmpty (&$ReadJournal $null);$summary.initialJournalClean=$true
    } catch { $summary.error='initial_state_requires_review';return $summary }
    foreach ($p in $policies) {
        $r=[ordered]@{id=$p.id;status='not_started';applyConfirmed=$false;backupObserved=$false;lockObserved=$false;readMatched=$false;restoreAttempted=$false;restoreConfirmed=$false;initialAbsent=$false;recordRemoved=$false;error=''}
        $owned=$false;$failed=$false;$restoreAllowed=$false
        try {
            foreach ($check in $policies) { if ((&$ReadPolicy $check).exists) { throw 'initial_policy_exists' } }
            Assert-RoundEmpty (&$ReadJournal $null)
            $r.status='apply'
            $action=&$Run 'apply' $p
            Assert-RoundAction $action $p 'apply';$r.applyConfirmed=$true
            $journal=&$ReadJournal $p
            if ($journal.state -cne 'active' -or -not $journal.lockPresent) { throw 'backup_not_confirmed' }
            $r.backupObserved=$true;$r.lockObserved=$true;$owned=$true
            $value=&$ReadPolicy $p
            if (-not $value.exists -or -not $value.matches) { throw 'independent_read_mismatch' }
            $r.readMatched=$true
        } catch {
            $failed=$true;$r.error='apply_or_readback_failed'
        } finally {
            if ($owned -and -not $script:roundUnknown) {
                try {
                    $currentJournal=&$ReadJournal $p
                    if ($currentJournal.state -cne 'active' -or -not $currentJournal.lockPresent) { throw 'backup_not_confirmed' }
                    $current=&$ReadPolicy $p
                    if ($current.exists -and -not $current.matches) { throw 'external_conflict' }
                    $restoreAllowed=$true
                    $r.restoreAttempted=$true
                    $action=&$Run 'restore' $p
                    Assert-RoundAction $action $p 'restore';$r.restoreConfirmed=$true
                    $r.initialAbsent=-not (&$ReadPolicy $p).exists
                    $after=&$ReadJournal $null
                    $r.recordRemoved=$after.state -ceq 'empty'
                    if (-not $r.initialAbsent -or -not $r.recordRemoved) { throw 'restore_readback_failed' }
                } catch {
                    $failed=$true;$r.error='restore_or_conflict_pending'
                }
            }
        }
        if (-not $failed -and $r.applyConfirmed -and $r.backupObserved -and $r.readMatched -and $r.restoreConfirmed -and $r.initialAbsent -and $r.recordRemoved) {
            $r.status='completed';$summary.completed++
        } elseif ($r.restoreConfirmed -and $r.initialAbsent -and $r.recordRemoved) { $r.status='recovered_after_failure' }
        elseif ($owned -or $r.applyConfirmed -or $script:roundUnknown) { $r.status='recovery_pending' }
        else { $r.status='rejected' }
        [void]$results.Add([pscustomobject]$r)
        if ($r.status -cne 'completed') { $summary.status=$r.status;$summary.error=$r.error;break }
    }
    if ($summary.completed -eq 4) { $summary.status='completed' }
    $summary.results=@($results.ToArray())
    return $summary
}

function New-RoundNative {
    $assembly=New-Object Reflection.AssemblyName('UltraPolicyRoundtrip')
    $builder=[AppDomain]::CurrentDomain.DefineDynamicAssembly($assembly,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $type=$builder.DefineDynamicModule('Native').DefineType('UltraPolicyRoundtripNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $specs=@(
        @('CreateFileW',[IntPtr],[type[]]@([string],[uint32],[uint32],[IntPtr],[uint32],[uint32],[IntPtr])),
        @('GetFileInformationByHandleEx',[bool],[type[]]@([IntPtr],[int],[IntPtr],[uint32])),
        @('CloseHandle',[bool],[type[]]@([IntPtr])),
        @('QueryDosDeviceW',[uint32],[type[]]@([string],[Text.StringBuilder],[uint32]))
    )
    $ctor=[Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields=[Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'),[Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'),[Runtime.InteropServices.DllImportAttribute].GetField('CharSet'),[Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    foreach ($s in $specs) {
        $method=$type.DefineMethod($s[0],[Reflection.MethodAttributes]'Public, Static, PinvokeImpl',$s[1],$s[2]);$method.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $attribute=New-Object Reflection.Emit.CustomAttributeBuilder($ctor,[object[]]@('kernel32.dll'),$fields,[object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi))
        $method.SetCustomAttribute($attribute)
    }
    return $type.CreateType()
}
function Get-RoundComponents([string]$Path) {
    if (-not $Path -or $Path -notmatch '^[A-Za-z]:\\' -or $Path.Contains('/') -or $Path.Substring(2).Contains(':')) { throw 'path_rejected' }
    $parts=@($Path.Substring(0,3));$current=$parts[0]
    foreach ($part in $Path.Substring(3).Split([char]'\')) {
        if (-not $part.Length -or $part -in @('.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or $part.IndexOfAny([char[]]'<>"|?*') -ge 0) { throw 'path_rejected' }
        foreach ($ch in $part.ToCharArray()) { if ([int]$ch -lt 32) { throw 'path_rejected' } }
        if ($part.Split([char]'.')[0].ToUpperInvariant() -match '^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])$') { throw 'path_rejected' }
        $current=[IO.Path]::Combine($current,$part);$parts+=$current
    }
    return $parts
}
function Assert-RoundPrivate([string]$Path,[bool]$Protected) {
    $acl=Get-Acl -LiteralPath $Path
    $parts=[Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access
    Assert-RoundSD ($acl.GetSecurityDescriptorSddlForm($parts)) $Protected
}
function Assert-RoundSD([string]$Sddl,[bool]$Protected) {
    $sd=New-Object Security.AccessControl.RawSecurityDescriptor($Sddl)
    if ($null -eq $sd.Owner -or $sd.Owner.Value -notin @('S-1-5-18','S-1-5-32-544') -or $null -eq $sd.DiscretionaryAcl -or ($Protected -and ($sd.ControlFlags -band [Security.AccessControl.ControlFlags]::DiscretionaryAclProtected) -eq 0)) { throw 'private_acl_rejected' }
    foreach ($ace in $sd.DiscretionaryAcl) {
        if ($ace -isnot [Security.AccessControl.CommonAce] -or $ace.IsCallback -or ($ace.AceQualifier -eq [Security.AccessControl.AceQualifier]::AccessAllowed -and $ace.SecurityIdentifier.Value -notin @('S-1-5-18','S-1-5-32-544'))) { throw 'private_acl_rejected' }
    }
}
function Close-RoundHandles($Handles) { for ($i=$Handles.Count-1;$i -ge 0;$i--) { [void]$script:roundNative::CloseHandle($Handles[$i]) } }
function Get-RoundAttributes([IntPtr]$Handle,[bool]$File) {
    $buffer=[Runtime.InteropServices.Marshal]::AllocHGlobal(24)
    try {
        if (-not $script:roundNative::GetFileInformationByHandleEx($Handle,9,$buffer,8)) { throw 'path_check_failed' }
        $attributes=[Runtime.InteropServices.Marshal]::ReadInt32($buffer)
        if (($attributes -band 0x400) -ne 0 -or ((($attributes -band 0x10) -eq 0) -ne $File)) { throw 'path_rejected' }
        if ($File) {
            if (-not $script:roundNative::GetFileInformationByHandleEx($Handle,1,$buffer,24) -or [Runtime.InteropServices.Marshal]::ReadInt32($buffer,16) -ne 1) { throw 'path_rejected' }
        }
    } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buffer) }
}
function Open-RoundDirectories([string]$Path,[string]$PrivateRoot,[bool]$AllowMissing,$Handles) {
    $parts=@(Get-RoundComponents $Path)
    $mapping=New-Object Text.StringBuilder(32768)
    if ($script:roundNative::QueryDosDeviceW($parts[0].Substring(0,2),$mapping,32768) -eq 0 -or $mapping.ToString() -notmatch '^\\Device\\HarddiskVolume[0-9]+$' -or (New-Object IO.DriveInfo($parts[0])).DriveType -ne [IO.DriveType]::Fixed) { throw 'drive_rejected' }
    foreach ($part in $parts) {
        $handle=$script:roundNative::CreateFileW($part,0x20081,1,[IntPtr]::Zero,3,0x02200000,[IntPtr]::Zero)
        if ($handle -eq [IntPtr](-1)) {
            $code=[Runtime.InteropServices.Marshal]::GetLastWin32Error()
            if ($AllowMissing -and $part -cne $parts[0] -and $code -in @(2,3)) { return $false }
            throw 'directory_open_failed'
        }
        [void]$Handles.Add($handle);Get-RoundAttributes $handle $false
        if ($PrivateRoot -and ($part.Equals($PrivateRoot,[StringComparison]::OrdinalIgnoreCase) -or $part.StartsWith($PrivateRoot+'\',[StringComparison]::OrdinalIgnoreCase))) { Assert-RoundPrivate $part $false }
    }
    return $true
}
function Read-RoundFile([string]$Path,[int]$Limit,[bool]$AllowMissing,$Handles) {
    $handle=$script:roundNative::CreateFileW($Path,[uint32]2147614848,1,[IntPtr]::Zero,3,0x00200000,[IntPtr]::Zero)
    if ($handle -eq [IntPtr](-1)) {
        $code=[Runtime.InteropServices.Marshal]::GetLastWin32Error()
        if ($AllowMissing -and $code -in @(2,3)) { return $null }
        throw 'file_open_failed'
    }
    [void]$Handles.Add($handle);Get-RoundAttributes $handle $true;Assert-RoundPrivate $Path $false
    $stream=[IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
    try {
        if ($stream.Length -gt $Limit) { throw 'file_size_rejected' }
        $bytes=New-Object byte[] ([int]$stream.Length);$position=0
        while ($position -lt $bytes.Length) { $read=$stream.Read($bytes,$position,$bytes.Length-$position);if ($read -le 0) { throw 'file_read_failed' };$position+=$read }
        return ,$bytes
    } finally { $stream.Dispose() }
}
function Read-RoundJournal($Policy) {
    $handles=New-Object 'Collections.Generic.List[IntPtr]'
    try {
        if (-not (Open-RoundDirectories $script:roundMachine $script:roundProduct $true $handles)) { return [pscustomobject]@{state='empty';lockPresent=$false} }
        Assert-RoundPrivate $script:roundMachine $true
        foreach ($entry in @(Get-ChildItem -LiteralPath $script:roundMachine -Force)) { if ($entry.Name -cnotin @('privacy-journal.json','privacy.lock')) { throw 'journal_requires_review' } }
        $lock=Read-RoundFile ([IO.Path]::Combine($script:roundMachine,'privacy.lock')) 0 $true $handles
        $lockPresent=$null -ne $lock
        $bytes=Read-RoundFile ([IO.Path]::Combine($script:roundMachine,'privacy-journal.json')) 4194304 $true $handles
        $state='empty'
        if ($null -ne $bytes) { $state=Read-RoundEnvelope $bytes $Policy }
        return [pscustomobject]@{state=$state;lockPresent=$lockPresent}
    } finally { Close-RoundHandles $handles }
}
function Read-RoundPolicy($Policy) {
    $hive=[Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine,[Microsoft.Win32.RegistryView]::Registry64)
    try {
        $key=$hive.OpenSubKey($Policy.path,$false)
        if ($null -eq $key) { return [pscustomobject]@{exists=$false;matches=$false} }
        try {
            if ($key.GetValueNames() -notcontains $Policy.name) { return [pscustomobject]@{exists=$false;matches=$false} }
            $matches=$key.GetValueKind($Policy.name) -eq [Microsoft.Win32.RegistryValueKind]::DWord -and $key.GetValue($Policy.name,$null,[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) -eq $Policy.desired
            return [pscustomobject]@{exists=$true;matches=[bool]$matches}
        } finally { $key.Dispose() }
    } finally { $hive.Dispose() }
}
function Invoke-RoundProcess([string]$Action,$Policy) {
    $allowed=@(Get-RoundPolicies | Where-Object { $_.id -ceq $Policy.id })
    if ($Action -cnotin @('apply','restore') -or $allowed.Count -ne 1) { throw 'command_rejected' }
    $info=New-Object Diagnostics.ProcessStartInfo
    $info.FileName=$script:roundCli;$info.Arguments='privacy '+$Action+' '+$allowed[0].id;$info.WorkingDirectory=$script:roundCandidate
    $info.UseShellExecute=$false;$info.CreateNoWindow=$true;$info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
    $process=New-Object Diagnostics.Process;$process.StartInfo=$info
    $memory=New-Object IO.MemoryStream;$started=$false;$complete=$false
    try {
        if (-not $process.Start()) { throw 'process_start_failed' };$started=$true;$script:roundStarted++
        $watch=[Diagnostics.Stopwatch]::StartNew();$outBuffer=New-Object byte[] 65536;$errBuffer=New-Object byte[] 4096
        $outTask=$process.StandardOutput.BaseStream.ReadAsync($outBuffer,0,$outBuffer.Length);$errTask=$process.StandardError.BaseStream.ReadAsync($errBuffer,0,$errBuffer.Length)
        while ($null -ne $outTask -or $null -ne $errTask -or -not $process.HasExited) {
            if ($watch.ElapsedMilliseconds -ge 15000) { throw 'process_timeout' }
            if ($null -ne $outTask -and $outTask.IsCompleted) {
                $read=$outTask.GetAwaiter().GetResult()
                if ($read -eq 0) { $outTask=$null } else { if ($memory.Length+$read -gt 65536) { throw 'output_limit' };$memory.Write($outBuffer,0,$read);$outTask=$process.StandardOutput.BaseStream.ReadAsync($outBuffer,0,$outBuffer.Length) }
            }
            if ($null -ne $errTask -and $errTask.IsCompleted) { $read=$errTask.GetAwaiter().GetResult();if ($read -gt 0) { throw 'stderr_present' };$errTask=$null }
            if ($null -ne $outTask -or $null -ne $errTask -or -not $process.HasExited) { [Threading.Thread]::Sleep(10) }
        }
        $process.WaitForExit();if ($process.ExitCode -ne 0) { throw 'process_nonzero' }
        $text=(New-Object Text.UTF8Encoding($false,$true)).GetString($memory.ToArray())
        $value=ConvertFrom-Json -InputObject $text -ErrorAction Stop
        Assert-RoundAction $value $Policy $Action
        $complete=$true;return $value
    } finally {
        if ($started -and -not $complete) {
            $script:roundUnknown=$true
            try { if (-not $process.HasExited) { $process.Kill() };if (-not $process.WaitForExit(3000) -or -not $process.HasExited) { $script:roundTerminationUnknown=$true } } catch { $script:roundTerminationUnknown=$true }
        }
        $memory.Dispose();$process.Dispose()
    }
}

$result=[ordered]@{schema='ultra-beta-machine-roundtrip-v1';candidate='0.01-beta-qa.2';status='rejected';systemToken=$false;cliHashMatched=$false;commandsStarted=0;resultUnknown=$false;terminationUnknown=$false;roundtrip=$null;error='';settingsScope='four_fixed_HKLM_values';runtimeBehaviorTested=$false;networkChangesRequested=$false;guiOrServiceStarted=$false}
$script:roundStarted=0;$script:roundUnknown=$false;$script:roundTerminationUnknown=$false;$script:roundNative=$null
$held=New-Object 'Collections.Generic.List[IntPtr]';$exitCode=1
try {
    if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or -not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'host_rejected' }
    if ($StageID -cnotmatch '^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$' -or $CandidateLeaf -cnotmatch '^delta-[a-f0-9]{32}$') { throw 'stage_rejected' }
    $identity=[Security.Principal.WindowsIdentity]::GetCurrent()
    try {
        $principal=New-Object Security.Principal.WindowsPrincipal($identity)
        if (-not $identity.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalSystemSid) -or -not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'system_required' }
        $result.systemToken=$true
    } finally { $identity.Dispose() }
    $script:roundNative=New-RoundNative
    $programData=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData,[Environment+SpecialFolderOption]::DoNotVerify)
    $qaRoot=[IO.Path]::Combine($programData,'GodUseVPN-Ultra-QA')
    $script:roundCandidate=[IO.Path]::Combine([IO.Path]::Combine($qaRoot,$StageID),$CandidateLeaf)
    $null=Open-RoundDirectories $script:roundCandidate $qaRoot $false $held
    $script:roundCli=[IO.Path]::Combine($script:roundCandidate,'godusevpn-cli.exe')
    $bytes=Read-RoundFile $script:roundCli 134217728 $false $held
    if ($bytes.Length -lt 1 -or (Get-RoundHash $bytes) -cne $fixedCliHash -or [Diagnostics.FileVersionInfo]::GetVersionInfo($script:roundCli).ProductVersion -cne '0.01-beta-qa.2') { throw 'cli_rejected' }
    $result.cliHashMatched=$true;$bytes=$null
    # CLI routes fixed HKLM IDs via the direct elevated machine manager. Its
    # ordinary report used for ID classification must not follow APPDATA away.
    $currentAppData=[Environment]::GetFolderPath([Environment+SpecialFolder]::ApplicationData,[Environment+SpecialFolderOption]::DoNotVerify)
    if (-not $env:APPDATA -or -not $env:APPDATA.Equals($currentAppData,[StringComparison]::OrdinalIgnoreCase)) { throw 'user_scope_rejected' }
    $null=Open-RoundDirectories ([IO.Path]::Combine($env:APPDATA,'godusevpn-ultra\privacy-user')) '' $true $held
    $script:roundProduct=[IO.Path]::Combine($programData,'godusevpn-ultra')
    $script:roundMachine=[IO.Path]::Combine($script:roundProduct,'privacy-machine')
    $result.roundtrip=Invoke-RoundBatch {param($p) Read-RoundPolicy $p} {param($p) Read-RoundJournal $p} {param($a,$p) Invoke-RoundProcess $a $p}
    $result.status=$result.roundtrip.status
    if ($result.status -ceq 'completed') { $exitCode=0 }
} catch {
    $known=@('host_rejected','stage_rejected','system_required','path_rejected','drive_rejected','private_acl_rejected','path_check_failed','directory_open_failed','file_open_failed','file_size_rejected','file_read_failed','cli_rejected','user_scope_rejected')
    if ($known -ccontains $_.Exception.Message) { $result.error=$_.Exception.Message } else { $result.error='preflight_failed' }
} finally {
    if ($null -ne $script:roundNative) { Close-RoundHandles $held }
    $result.commandsStarted=$script:roundStarted;$result.resultUnknown=$script:roundUnknown;$result.terminationUnknown=$script:roundTerminationUnknown
}
$json=$result | ConvertTo-Json -Compress -Depth 8
if ($json.Length -gt 3000) { $json='{"schema":"ultra-beta-machine-roundtrip-v1","status":"rejected","error":"summary_limit"}';$exitCode=1 }
[Console]::Out.WriteLine($json)
exit $exitCode
