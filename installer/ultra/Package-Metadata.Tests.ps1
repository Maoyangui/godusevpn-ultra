# Pure AST-selected validators and an inert TEMP package. The already-prepared
# pinned bundle is copied/read as bytes only: no ExifTool, Perl, installer main,
# service, CIM, registry, ACL, network, or browser operation is invoked.
[CmdletBinding(SupportsShouldProcess = $true)]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:UltraName = 'godusevpn-ultra'; $script:UltraProduct = 'GodUseVPN Ultra'
$project = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$tempBase = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$fixture = Join-Path $tempBase ('ultra-metadata-qa-' + [Guid]::NewGuid().ToString('N'))
$utf8 = New-Object Text.UTF8Encoding($false)
$checks = 0
$pinned = 'd1316fddf986e2d17761f85034c753b28375f25bf73235c1084a7c5a38e22faf'
function Check([bool]$Condition, [string]$Name) {
    if (-not $Condition) { throw "Metadata fixture failed: $Name" }
    $script:checks++
}
function Reject([scriptblock]$Operation, [string]$Expected, [string]$Name) {
    $message = ''
    try { & $Operation | Out-Null } catch { $message = $_.Exception.Message }
    Check ($message.Contains($Expected)) ($Name + ' / ' + $message)
}
function Reference-Hash([string]$Path) {
    $sha = [Security.Cryptography.SHA256]::Create(); $stream = [IO.File]::OpenRead($Path)
    try { return ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-', '').ToLowerInvariant() }
    finally { $stream.Dispose(); $sha.Dispose() }
}
function Write-Descriptor([object]$Tool, [bool]$Include = $true, [string]$Architecture = 'amd64') {
    $descriptor = @{ product='GodUseVPN Ultra'; namespace='godusevpn-ultra'; version='0.01-beta-qa.8'; architecture=$Architecture; unsigned=$true; sourceBaseline='b51aed4' }
    if ($Include) { $descriptor.metadataTool = $Tool }
    [IO.File]::WriteAllText((Join-Path $fixture 'ULTRA_PACKAGE.json'), ($descriptor | ConvertTo-Json -Depth 8 -Compress), $utf8)
    $script:outer['ULTRA_PACKAGE.json'] = Reference-Hash (Join-Path $fixture 'ULTRA_PACKAGE.json')
    Write-Outer
}
function Write-Outer {
    $lines = New-Object 'Collections.Generic.List[string]'
    foreach ($name in @($script:outer.Keys | Sort-Object)) { $lines.Add($script:outer[$name] + '  ' + $name) }
    [IO.File]::WriteAllLines((Join-Path $fixture 'SHA256SUMS'), $lines.ToArray(), $utf8)
}
function Valid-Tool { return @{ version='13.59'; implementation='exiftool'; manifestSHA256=$pinned; fileCount=1501; vmValidated=$false } }
function Load-Validators([string]$Name) {
    $tokens=$null; $errors=$null
    $ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $Name), [ref]$tokens, [ref]$errors)
    Check ($errors.Count -eq 0) "parse_$Name"
    foreach ($function in @('Get-Sha256Hash','Assert-LocalPath','Assert-NoReparse','Get-Inventory','Assert-RelativeName','Test-Package')) {
        $definitions=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $function},$true))
        Check ($definitions.Count -eq 1) "validator_$function"
        # Define in the script scope, without running installer top-level code.
        . ([scriptblock]::Create('function script:' + $definitions[0].Name + $definitions[0].Extent.Text.Substring(('function ' + $definitions[0].Name).Length)))
    }
}
try {
    Check ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() -ceq 'X64') 'fixture_requires_prepared_x64_bundle'
    $prepared=Join-Path $project '.codex/tmp/metadata-tool-build/exiftool-13.59'
    $manifestSource=Join-Path $project 'internal/metadatatool/bundle.sha256'
    Check ([IO.Directory]::Exists($prepared)) 'prepared_bundle_required_no_download_or_execution'
    Check ((Reference-Hash $manifestSource) -ceq $pinned) 'real_manifest_pin'
    [IO.Directory]::CreateDirectory((Join-Path $fixture 'browser-extension')) | Out-Null
    $script:outer=@{}
    foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe','Install-Ultra.ps1','Uninstall-Ultra.ps1','browser-extension/manifest.json')) {
        [IO.File]::WriteAllText((Join-Path $fixture $name), 'inert package fixture; never executable', $utf8)
        $script:outer[$name]=Reference-Hash (Join-Path $fixture $name)
    }
    foreach ($scriptName in @('Install-Ultra.ps1','Uninstall-Ultra.ps1')) {
        Load-Validators $scriptName
        Write-Descriptor $null $false
        $old=Test-Package $fixture
        Check ($old.Hashes.Count -eq 7) "legacy_qa7_without_component_$scriptName"
        Write-Descriptor (Valid-Tool)
        Reject { Test-Package $fixture } 'Pinned metadata manifest' "missing_manifest_$scriptName"
        foreach ($kind in @('string_count','decimal_count','string_bool','true_bool','null_tool','wrong_version','wrong_implementation','wrong_pin')) {
            $tool=Valid-Tool
            switch ($kind) {
                'string_count' { $tool.fileCount='1501' }
                'decimal_count' { $tool.fileCount=1501.5 }
                'string_bool' { $tool.vmValidated='false' }
                'true_bool' { $tool.vmValidated=$true }
                'null_tool' { $tool=$null }
                'wrong_version' { $tool.version='13.60' }
                'wrong_implementation' { $tool.implementation='perl' }
                'wrong_pin' { $tool.manifestSHA256='a' * 64 }
            }
            Write-Descriptor $tool
            Reject { Test-Package $fixture } 'Unsupported metadata tool descriptor' ($kind + '_' + $scriptName)
        }
        Write-Descriptor (Valid-Tool) $true 'arm64'
        Reject { Test-Package $fixture } 'Package architecture must match' "wrong_architecture_$scriptName"
    }
    # Copy every pinned member into our own TEMP tree, never alter the source.
    $prefix='metadata-tool/exiftool-13.59/'
    $memberNames=New-Object 'Collections.Generic.List[string]'
    foreach ($line in [IO.File]::ReadAllLines($manifestSource)) {
        if ($line -notmatch '^([a-f0-9]{64})  (.+)$') { throw 'Malformed real fixture manifest.' }
        $hash=$Matches[1]; $relative=$Matches[2]; $outerName=$prefix+$relative
        $source=Join-Path $prepared $relative; $destination=Join-Path $fixture $outerName
        Check ((Reference-Hash $source) -ceq $hash) 'prepared_member_matches_fixed_manifest'
        [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($destination)) | Out-Null
        [IO.File]::Copy($source,$destination,$false)
        $script:outer[$outerName]=$hash; $memberNames.Add($outerName)
    }
    Check ($memberNames.Count -eq 1501) 'real_1501_members'
    $innerName='metadata-tool/exiftool-13.59.sha256'; $innerPath=Join-Path $fixture $innerName
    [IO.File]::Copy($manifestSource,$innerPath,$false); $script:outer[$innerName]=$pinned
    $member=$prefix+'exiftool_files/README'; $memberPath=Join-Path $fixture $member
    $original=[IO.File]::ReadAllBytes($memberPath); $originalHash=$script:outer[$member]
    foreach ($scriptName in @('Install-Ultra.ps1','Uninstall-Ultra.ps1')) {
        Load-Validators $scriptName; Write-Descriptor (Valid-Tool)
        $verified=Test-Package $fixture
        Check ($verified.Hashes.Count -eq 1509) "real_pinned_bundle_accepted_$scriptName"
        [IO.File]::Delete($innerPath); $script:outer.Remove($innerName); Write-Outer
        Reject { Test-Package $fixture } 'Pinned metadata manifest' "missing_inner_manifest_$scriptName"
        [IO.File]::Copy($manifestSource,$innerPath,$false); $script:outer[$innerName]=$pinned
        [IO.File]::AppendAllText($innerPath,"`n",$utf8); $script:outer[$innerName]=Reference-Hash $innerPath; Write-Outer
        Reject { Test-Package $fixture } 'Pinned metadata manifest' "changed_inner_rehashed_outer_$scriptName"
        [IO.File]::WriteAllBytes($innerPath,[IO.File]::ReadAllBytes($manifestSource)); $script:outer[$innerName]=$pinned
        [IO.File]::Delete($memberPath); $script:outer.Remove($member); Write-Outer
        Reject { Test-Package $fixture } 'Metadata member missing or differs' "missing_member_rehashed_outer_$scriptName"
        [IO.File]::WriteAllText($memberPath,'modified inert text',$utf8); $script:outer[$member]=Reference-Hash $memberPath; Write-Outer
        Reject { Test-Package $fixture } 'Metadata member missing or differs' "changed_member_rehashed_outer_$scriptName"
        [IO.File]::WriteAllBytes($memberPath,$original); $script:outer[$member]=$originalHash
        $extra=$prefix+'unlisted-fixture.txt'; [IO.File]::WriteAllText((Join-Path $fixture $extra),'inert extra',$utf8); $script:outer[$extra]=Reference-Hash (Join-Path $fixture $extra); Write-Outer
        Reject { Test-Package $fixture } 'Unexpected metadata tool member' "extra_member_rehashed_outer_$scriptName"
        [IO.File]::Delete((Join-Path $fixture $extra)); $script:outer.Remove($extra); Write-Outer
        Check ((Test-Package $fixture).Hashes.Count -eq 1509) "restored_bundle_accepted_$scriptName"
    }
    [pscustomobject]@{status='passed';checks=$checks;psVersion=$PSVersionTable.PSVersion.ToString();whatIf=[bool]$WhatIfPreference;scope='pure AST validators, copied/read pinned 1501-member TEMP bundle, no product execution or system changes'} | ConvertTo-Json -Compress
} finally {
    $resolved=[IO.Path]::GetFullPath($fixture)
    if (-not $resolved.StartsWith($tempBase+'\',[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolved) -notmatch '^ultra-metadata-qa-[a-f0-9]{32}$') { throw 'Unsafe metadata fixture cleanup target.' }
    if ([IO.Directory]::Exists($resolved)) { [IO.Directory]::Delete($resolved,$true) }
}
