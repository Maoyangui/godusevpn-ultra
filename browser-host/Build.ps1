[CmdletBinding()]
param(
    [switch]$RunPureTests,
    [ValidatePattern('^[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-[A-Za-z0-9.-]+)?$')]
    [string]$Version = '0.01-beta'
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$packageVersion = '1.0.4314.35'
$packageHash = '90778deb1b77e36292828b4b5833c9255c27533f40e0ddd55891ae718944edc4'
$dependencyRoot = Join-Path $projectRoot ".codex\tmp\webview2-managed-$packageVersion"
$outputRoot = Join-Path $projectRoot '.codex\tmp\browser-host-build'
[IO.Directory]::CreateDirectory($dependencyRoot) | Out-Null
[IO.Directory]::CreateDirectory($outputRoot) | Out-Null
$numericParts = @($Version.Split('-')[0].Split('.') | ForEach-Object { [int]$_ })
while ($numericParts.Count -lt 4) { $numericParts += 0 }
if (@($numericParts | Where-Object { $_ -lt 0 -or $_ -gt 65535 }).Count -ne 0) { throw 'Numeric version components must fit a Windows VERSIONINFO word.' }
$fileVersion = $numericParts -join '.'
$assemblyInfo = Join-Path $outputRoot 'AssemblyInfo.generated.cs'
$assemblySource = @"
using System.Reflection;
[assembly: AssemblyTitle("GodUseVPN Ultra Browser Host")]
[assembly: AssemblyProduct("GodUseVPN Ultra")]
[assembly: AssemblyCompany("GodUseVPN Ultra")]
[assembly: AssemblyInformationalVersion("$Version")]
[assembly: AssemblyFileVersion("$fileVersion")]
[assembly: AssemblyVersion("$fileVersion")]
"@
[IO.File]::WriteAllText($assemblyInfo, $assemblySource, (New-Object Text.UTF8Encoding($false)))
$package = Join-Path $dependencyRoot "microsoft.web.webview2.$packageVersion.nupkg"
if (-not [IO.File]::Exists($package)) {
    Invoke-WebRequest -Uri "https://api.nuget.org/v3-flatcontainer/microsoft.web.webview2/$packageVersion/microsoft.web.webview2.$packageVersion.nupkg" -OutFile $package
}
if ((Get-FileHash -LiteralPath $package -Algorithm SHA256).Hash -ine $packageHash) { throw 'Pinned official SDK package hash mismatch.' }
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [IO.Compression.ZipFile]::OpenRead($package)
try {
    foreach ($name in @('lib/net462/Microsoft.Web.WebView2.Core.dll', 'lib/net462/Microsoft.Web.WebView2.WinForms.dll', 'runtimes/win-x64/native/WebView2Loader.dll', 'LICENSE.txt')) {
        $entry = $archive.GetEntry($name)
        if ($null -eq $entry) { throw 'Pinned SDK package lacks a required member.' }
        $target = Join-Path $dependencyRoot $name
        [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null
        [IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $target, $true)
    }
} finally { $archive.Dispose() }
$compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
if (-not [IO.File]::Exists($compiler)) { throw 'The x64 .NET Framework C# compiler is required; this script does not install it.' }
$core = Join-Path $dependencyRoot 'lib/net462/Microsoft.Web.WebView2.Core.dll'
$forms = Join-Path $dependencyRoot 'lib/net462/Microsoft.Web.WebView2.WinForms.dll'
$output = Join-Path $outputRoot 'GodUseVPN.Ultra.BrowserHost.exe'
$sources = @('Contracts.cs', 'Environment.cs', 'Hardware.cs', 'Graphics.cs', 'WindowsGuard.cs', 'RelayProtocol.cs', 'SocksRelay.cs', 'BrowserForm.cs', 'Program.cs') | ForEach-Object { Join-Path $PSScriptRoot $_ }
$sources += $assemblyInfo
& $compiler /nologo /target:exe /platform:x64 /optimize+ "/out:$output" "/win32manifest:$(Join-Path $PSScriptRoot 'host.manifest')" /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Web.Extensions.dll "/reference:$core" "/reference:$forms" $sources
if ($LASTEXITCODE -ne 0) { throw 'Browser host static compilation failed.' }
[IO.File]::Copy($core, (Join-Path $outputRoot 'Microsoft.Web.WebView2.Core.dll'), $true)
[IO.File]::Copy($forms, (Join-Path $outputRoot 'Microsoft.Web.WebView2.WinForms.dll'), $true)
[IO.File]::Copy((Join-Path $dependencyRoot 'runtimes/win-x64/native/WebView2Loader.dll'), (Join-Path $outputRoot 'WebView2Loader.dll'), $true)
[IO.File]::Copy((Join-Path $dependencyRoot 'LICENSE.txt'), (Join-Path $outputRoot 'Microsoft.Web.WebView2.LICENSE.txt'), $true)
[IO.File]::Copy((Join-Path $PSScriptRoot 'host.exe.config'), "$output.config", $true)
if ($RunPureTests) {
    $tests = Join-Path $outputRoot 'BrowserHost.PureTests.exe'
    $golden = Join-Path $projectRoot 'internal/browserhost/testdata/chromium-ua-golden.json'
    & $compiler /nologo /target:exe /platform:x64 "/out:$tests" "/resource:$golden,ChromiumUaGolden.json" /main:GodUseVPN.Ultra.BrowserHost.PureTests /reference:System.Web.Extensions.dll (Join-Path $PSScriptRoot 'Contracts.cs') (Join-Path $PSScriptRoot 'Environment.cs') (Join-Path $PSScriptRoot 'Hardware.cs') (Join-Path $PSScriptRoot 'HardwareTests.cs') (Join-Path $PSScriptRoot 'RelayProtocol.cs') (Join-Path $PSScriptRoot 'EnvironmentRelayTests.cs') (Join-Path $PSScriptRoot 'Graphics.cs') (Join-Path $PSScriptRoot 'GraphicsScriptTests.cs') (Join-Path $PSScriptRoot 'WindowsGuard.cs') (Join-Path $PSScriptRoot 'PureTests.cs')
    if ($LASTEXITCODE -ne 0) { throw 'Pure test compilation failed.' }
    & $tests
    if ($LASTEXITCODE -ne 0) { throw 'Pure host contract tests failed.' }
}
Get-FileHash -LiteralPath $output -Algorithm SHA256
Write-Output 'Host compiled only. Browser host/WebView2 were NOT executed or installed.'
