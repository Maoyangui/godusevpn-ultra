# AST-selected package validators and inert TEMP fixtures only. No installer
# main, consumer, service, native location, ACL mutation or network is invoked.
[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:UltraName = 'godusevpn-ultra'
$script:UltraProduct = 'GodUseVPN Ultra'
$script:checks = 0
$temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$fixture = Join-Path $temporaryRoot ('ultra-location-package-' + [Guid]::NewGuid().ToString('N'))
$utf8 = [Text.UTF8Encoding]::new($false)
$runtimeConfig = '<?xml version="1.0"?><configuration><startup><supportedRuntime version="v4.0" sku=".NETFramework,Version=v4.6.2"/></startup></configuration>'
$baseFiles = @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe','Install-Ultra.ps1','Uninstall-Ultra.ps1','browser-extension/manifest.json','ULTRA_PACKAGE.json')
$consumerFiles = @('location-consumer/GodUseVPN.Ultra.LocationConsumer.exe','location-consumer/GodUseVPN.Ultra.LocationConsumer.exe.config')
$architecture = switch ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) { 'X64' {'amd64'} default {throw 'This x64-only component fixture requires x64.'} }
function Check([bool]$Condition, [string]$Name) { if (-not $Condition) {throw "Location package fixture failed: $Name"}; $script:checks++ }
function Reject([scriptblock]$Operation, [string]$Name) { $failed=$false; try {& $Operation | Out-Null} catch {$failed=$true}; Check $failed $Name }
function Reset-Fixture {
    $script:descriptor = @{product='GodUseVPN Ultra';namespace='godusevpn-ultra';version='0.01-beta-qa.12';architecture=$architecture;unsigned=$true;sourceBaseline='b51aed4';locationConsumer=@{schemaVersion=1;implementation='winrt-geolocator';architecture='amd64';minimumFramework='4.6.2';nativeRuntimeBundled=$false;vmValidated=$false}}
    foreach ($relative in @($baseFiles + $consumerFiles)) { [IO.File]::WriteAllText((Join-Path $fixture $relative), 'inert, never executed', $utf8) }
    [IO.File]::WriteAllText((Join-Path $fixture $consumerFiles[1]), $runtimeConfig, $utf8)
}
function Save-Manifest {
    [IO.File]::WriteAllText((Join-Path $fixture 'ULTRA_PACKAGE.json'), ($script:descriptor | ConvertTo-Json -Depth 6 -Compress), $utf8)
    $lines = @(Get-ChildItem -LiteralPath $fixture -File -Recurse | Where-Object {$_.Name -cne 'SHA256SUMS'} | Sort-Object FullName | ForEach-Object {
        '{0}  {1}' -f (Get-Sha256Hash $_.FullName).ToLowerInvariant(), $_.FullName.Substring($fixture.Length + 1).Replace('\','/')
    })
    [IO.File]::WriteAllLines((Join-Path $fixture 'SHA256SUMS'), [string[]]$lines, $utf8)
}
try {
    $null=[IO.Directory]::CreateDirectory((Join-Path $fixture 'browser-extension'))
    $null=[IO.Directory]::CreateDirectory((Join-Path $fixture 'location-consumer'))
    foreach ($name in @('Install-Ultra.ps1','Uninstall-Ultra.ps1')) {
        $tokens=$null; $errors=$null
        $ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $name), [ref]$tokens, [ref]$errors)
        Check ($errors.Count -eq 0) "parse_$name"
        foreach ($functionName in @('Get-Sha256Hash','Assert-LocalPath','Assert-NoReparse','Get-Inventory','Assert-RelativeName','Test-Package','Assert-NoRunningGUI')) {
            $definitions=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $functionName}, $true))
            Check ($definitions.Count -eq 1) "definition_$functionName"
            . ([scriptblock]::Create($definitions[0].Extent.Text))
        }
        Reset-Fixture; Save-Manifest
        $result=Test-Package $fixture
        Check ($result.Hashes.Count -eq 9 -and $result.Descriptor.locationConsumer.minimumFramework -ceq '4.6.2') 'valid_x64_framework462_payload'
        foreach ($case in @('undeclared','newer_framework','other_architecture','false_validation','string_boolean','wrong_implementation','missing_config','extra_dependency','extra_runtime_config')) {
            Reset-Fixture
            switch ($case) {
                'undeclared' {$script:descriptor.Remove('locationConsumer')}
                'newer_framework' {$script:descriptor.locationConsumer.minimumFramework='4.8'}
                'other_architecture' {$script:descriptor.locationConsumer.architecture='arm64'}
                'false_validation' {$script:descriptor.locationConsumer.vmValidated=$true}
                'string_boolean' {$script:descriptor.locationConsumer.nativeRuntimeBundled='false'}
                'wrong_implementation' {$script:descriptor.locationConsumer.implementation='unreviewed'}
                'missing_config' {[IO.File]::Delete((Join-Path $fixture $consumerFiles[1]))}
                'extra_dependency' {[IO.File]::WriteAllText((Join-Path $fixture 'location-consumer/extra.dll'),'inert',$utf8)}
                'extra_runtime_config' {[IO.File]::WriteAllText((Join-Path $fixture $consumerFiles[1]),$runtimeConfig.Replace('</configuration>','<runtime><loadFromRemoteSources enabled="true"/></runtime></configuration>'),$utf8)}
            }
            Save-Manifest
            Reject {Test-Package $fixture} "$name/$case"
            $extra=Join-Path $fixture 'location-consumer/extra.dll'; if ([IO.File]::Exists($extra)) {[IO.File]::Delete($extra)}
        }
        Reset-Fixture; Save-Manifest
        $script:fakeProcesses=@([pscustomobject]@{ExecutablePath=(Join-Path $fixture $consumerFiles[0]);ProcessId=123})
        function Get-CimInstance { return $script:fakeProcesses }
        Reject {Assert-NoRunningGUI $fixture} 'running_installed_consumer_prevents_upgrade'
        $script:fakeProcesses=@([pscustomobject]@{ExecutablePath=(Join-Path $temporaryRoot 'another-install/location-consumer/GodUseVPN.Ultra.LocationConsumer.exe');ProcessId=123})
        Assert-NoRunningGUI $fixture; Check $true 'other_install_is_not_terminated_or_blocked'
        $script:fakeProcesses=@([pscustomobject]@{ExecutablePath='';ProcessId=123})
        Reject {Assert-NoRunningGUI $fixture} 'unverifiable_consumer_prevents_upgrade'
    }
    [pscustomobject]@{status='passed';checks=$script:checks;scope='AST validators, inert TEMP files, fake process inventory only';installerMainExecuted=$false;consumerExecuted=$false;nativeLocationRead=$false} | ConvertTo-Json -Compress
} finally {
    $resolved=[IO.Path]::GetFullPath($fixture)
    if (-not $resolved.StartsWith($temporaryRoot+'\',[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolved) -notmatch '^ultra-location-package-[a-f0-9]{32}$') {throw 'Unsafe location package fixture cleanup target.'}
    if ([IO.Directory]::Exists($resolved)) {[IO.Directory]::Delete($resolved,$true)}
}
