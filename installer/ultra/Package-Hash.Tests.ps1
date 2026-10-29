# AST-selected validators and tiny inert TEMP fixtures only. No script main,
# installer, service, CIM, ACL change, product executable or network command is invoked.
[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:UltraName = 'godusevpn-ultra'
$script:UltraProduct = 'GodUseVPN Ultra'
$script:checks = 0
$script:legacyHashCalls = 0
$temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$fixture = [IO.Path]::Combine($temporaryRoot, 'ultra-installer-hash-' + [Guid]::NewGuid().ToString('N'))
$utf8 = New-Object Text.UTF8Encoding($false)

function Check([bool]$Condition, [string]$Name) {
    if (-not $Condition) { throw "Hash fixture failed: $Name" }
    $script:checks++
}
function Reject([scriptblock]$Operation, [string]$Name) {
    $failed = $false
    try { & $Operation | Out-Null } catch { $failed = $true }
    Check $failed $Name
}
function Reference-Hash([string]$Path) {
    $digest = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($digest.ComputeHash([IO.File]::ReadAllBytes($Path)))).Replace('-', '') }
    finally { $digest.Dispose() }
}
function Invoke-ValidatorPreview {
    [CmdletBinding(SupportsShouldProcess = $true)]
    param([string]$Path)
    return Test-Package $Path
}
function Invoke-HashPreview {
    [CmdletBinding(SupportsShouldProcess = $true)]
    param([string]$Path)
    return Get-Sha256Hash $Path
}
function Invoke-ColdFile([string]$Shell, [string]$Script, [string]$Path) {
    $info = New-Object Diagnostics.ProcessStartInfo
    $info.FileName = $Shell
    $info.Arguments = '-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $Script + '" -LiteralPath "' + $Path + '" -WhatIf'
    $info.UseShellExecute = $false; $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true; $info.RedirectStandardError = $true
    # PS7 prepends its own incompatible modules to this inherited variable.
    # Let the cold Windows PowerShell process construct its normal module path.
    $info.EnvironmentVariables.Remove('PSModulePath')
    $process = New-Object Diagnostics.Process
    $process.StartInfo = $info
    try {
        if (-not $process.Start()) { throw 'Cold validator process did not start.' }
        $output = $process.StandardOutput.ReadToEndAsync(); $errorText = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(20000)) { $process.Kill(); throw 'Cold validator fixture timed out.' }
        return [pscustomobject]@{ ExitCode=$process.ExitCode; Output=$output.GetAwaiter().GetResult(); Error=$errorText.GetAwaiter().GetResult() }
    } finally { $process.Dispose() }
}
# Model a command that supplies no Hash-bearing result. This recreates the old
# strict-mode member failure without claiming to reproduce the remote PS build.
function Get-FileHash { $script:legacyHashCalls++; return $null }

try {
    [IO.Directory]::CreateDirectory($fixture) | Out-Null
    [IO.Directory]::CreateDirectory([IO.Path]::Combine($fixture, 'browser-extension')) | Out-Null
    [IO.Directory]::CreateDirectory([IO.Path]::Combine($fixture, 'nested')) | Out-Null
    $payload = @('godusevpn.exe', 'godusevpn-cli.exe', 'godusevpn-svc.exe', 'Install-Ultra.ps1', 'Uninstall-Ultra.ps1', 'browser-extension/manifest.json', 'NOTICE', 'nested/literal[1].txt', 'ULTRA_PACKAGE.json')
    foreach ($name in $payload) { [IO.File]::WriteAllText([IO.Path]::Combine($fixture, $name), 'inert hash fixture', $utf8) }
    [IO.File]::WriteAllBytes([IO.Path]::Combine($fixture, 'NOTICE'), [byte[]]@())
    [IO.File]::WriteAllBytes([IO.Path]::Combine($fixture, 'nested/literal[1].txt'), [Text.Encoding]::ASCII.GetBytes('abc'))
    $architecture = switch ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) { 'X64' { 'amd64' } 'Arm64' { 'arm64' } default { throw 'Unsupported fixture architecture.' } }
    $descriptor = @{ product='GodUseVPN Ultra'; namespace='godusevpn-ultra'; version='0.01-beta-qa.8'; architecture=$architecture; unsigned=$true; sourceBaseline='b51aed4' }
    [IO.File]::WriteAllText([IO.Path]::Combine($fixture, 'ULTRA_PACKAGE.json'), ($descriptor | ConvertTo-Json -Compress), $utf8)
    $lines = @($payload | ForEach-Object { '{0}  {1}' -f (Reference-Hash ([IO.Path]::Combine($fixture, $_))), $_ })
    $manifest = [IO.Path]::Combine($fixture, 'SHA256SUMS')
    [IO.File]::WriteAllLines($manifest, [string[]]$lines, $utf8)
    Reject { (Get-FileHash -LiteralPath ([IO.Path]::Combine($fixture, 'NOTICE')) -Algorithm SHA256).Hash } 'old_null_hash_member_failure'
    $script:legacyHashCalls = 0
    $windowsShell = [IO.Path]::Combine($env:WINDIR, 'System32/WindowsPowerShell/v1.0/powershell.exe')
    Check ([IO.File]::Exists($windowsShell)) 'cold_windows_powershell_available'
    $legacyProbe = [IO.Path]::Combine($temporaryRoot, 'ultra-installer-hash-' + [Guid]::NewGuid().ToString('N') + '.ps1')
    [IO.File]::WriteAllText($legacyProbe, @'
[CmdletBinding(SupportsShouldProcess = $true)]
param([string]$LiteralPath)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$hash = (Get-FileHash -LiteralPath $LiteralPath -Algorithm SHA256).Hash
[Console]::Out.WriteLine($hash)
'@, $utf8)
    try {
        foreach ($relative in @('NOTICE', 'nested/literal[1].txt')) {
            $failure = Invoke-ColdFile $windowsShell $legacyProbe ([IO.Path]::Combine($fixture, $relative))
            Check ($failure.ExitCode -ne 0 -and $failure.Error.Contains('PropertyNotFoundStrict') -and $failure.Output.Contains('ProviderPath')) 'cold_file_level_legacy_whatif_reproduced'
        }
    } finally { if ([IO.File]::Exists($legacyProbe)) { [IO.File]::Delete($legacyProbe) } }

    foreach ($name in @('Install-Ultra.ps1', 'Uninstall-Ultra.ps1')) {
        $tokens = $null; $parseErrors = $null
        $source = [IO.Path]::Combine($PSScriptRoot, $name)
        $ast = [Management.Automation.Language.Parser]::ParseFile($source, [ref]$tokens, [ref]$parseErrors)
        Check ($parseErrors.Count -eq 0) "parse_$name"
        $definitionText = @()
        foreach ($function in @('Get-Sha256Hash', 'Assert-LocalPath', 'Assert-NoReparse', 'Get-Inventory', 'Assert-RelativeName', 'Test-Package')) {
            $definitions = @($ast.FindAll({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $function }, $true))
            Check ($definitions.Count -eq 1) "validator_$function"
            # Never dot-source the installer/uninstaller top level.
            . ([scriptblock]::Create($definitions[0].Extent.Text))
            $definitionText += $definitions[0].Extent.Text
        }
        $coldValidator = [IO.Path]::Combine($temporaryRoot, 'ultra-installer-hash-' + [Guid]::NewGuid().ToString('N') + '.ps1')
        $coldSource = @'
[CmdletBinding(SupportsShouldProcess = $true)]
param([string]$LiteralPath)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:UltraName = 'godusevpn-ultra'
$script:UltraProduct = 'GodUseVPN Ultra'
'@
        $coldSource += "`r`n" + ($definitionText -join "`r`n") + "`r`n" + @'
$result = Test-Package $LiteralPath
if ($result.Hashes.Count -ne 9 -or $result.ManifestHash -notmatch '^[A-F0-9]{64}$') { throw 'Cold package validator result is incomplete.' }
[Console]::Out.WriteLine('COLD_PACKAGE_HASH_PASSED')
'@
        [IO.File]::WriteAllText($coldValidator, $coldSource, $utf8)
        try {
            $success = Invoke-ColdFile $windowsShell $coldValidator $fixture
            Check ($success.ExitCode -eq 0 -and $success.Output.Contains('COLD_PACKAGE_HASH_PASSED') -and $success.Error.Length -eq 0) "cold_file_level_fixed_whatif_$name"
        } finally { if ([IO.File]::Exists($coldValidator)) { [IO.File]::Delete($coldValidator) } }
        foreach ($preview in @($false, $true)) {
            $emptyHash = @(Invoke-HashPreview -Path ([IO.Path]::Combine($fixture, 'NOTICE')) -WhatIf:$preview)
            Check ($emptyHash.Count -eq 1 -and $emptyHash[0] -ceq 'E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855') "empty_hash_whatif_$preview"
            Check ((Invoke-HashPreview -Path ([IO.Path]::Combine($fixture, 'nested/literal[1].txt')) -WhatIf:$preview) -ceq 'BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD') "literal_hash_whatif_$preview"
            $result = Invoke-ValidatorPreview -Path $fixture -WhatIf:$preview
            Check ($result.Hashes.Count -eq $payload.Count -and $result.ManifestHash -ceq (Reference-Hash $manifest)) "package_hashes_whatif_$preview"
            Check ($script:legacyHashCalls -eq 0) 'no_GetFileHash_dependency'
        }
        $notice = [IO.Path]::Combine($fixture, 'NOTICE')
        [IO.File]::WriteAllText($notice, 'changed notice', $utf8)
        Reject { Invoke-ValidatorPreview -Path $fixture -WhatIf } 'tampered_payload_rejected'
        [IO.File]::WriteAllBytes($notice, [byte[]]@())
        $extra = [IO.Path]::Combine($fixture, 'unlisted.txt')
        [IO.File]::WriteAllText($extra, 'unlisted inert fixture', $utf8)
        Reject { Invoke-ValidatorPreview -Path $fixture -WhatIf } 'unlisted_payload_rejected'
        [IO.File]::Delete($extra)
        Reject { Invoke-HashPreview -Path ([IO.Path]::Combine($fixture, 'missing.txt')) -WhatIf } 'missing_hash_fails'
        $locked = [IO.File]::Open($notice, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
        try { Reject { Invoke-HashPreview -Path $notice -WhatIf } 'unreadable_hash_fails' }
        finally { $locked.Dispose() }
        Check ((Invoke-HashPreview -Path $notice -WhatIf) -ceq (Reference-Hash $notice)) 'stream_released_after_failure'
    }
    [pscustomobject]@{ status='passed'; checks=$script:checks; psVersion=$PSVersionTable.PSVersion.ToString(); scope='AST validators, real cold WindowsPowerShell -File -WhatIf legacy failure and fixed validation, inert TEMP files only'; scriptMainExecuted=$false; productExecuted=$false; serviceChanged=$false } | ConvertTo-Json -Compress
} finally {
    $resolved = [IO.Path]::GetFullPath($fixture)
    if (-not $resolved.StartsWith($temporaryRoot + '\', [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolved) -notmatch '^ultra-installer-hash-[a-f0-9]{32}$') { throw 'Unsafe hash fixture cleanup target.' }
    if ([IO.Directory]::Exists($resolved)) { [IO.Directory]::Delete($resolved, $true) }
}
