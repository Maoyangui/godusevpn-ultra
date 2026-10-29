[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$OutputDirectory)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) { throw 'Output directory required' }
$out=[IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $out) { throw 'Output directory already exists' }
$compiler=Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
$metadata=Join-Path $env:WINDIR 'System32\WinMetadata'
$runtime=Join-Path $env:WINDIR 'Microsoft.NET\assembly\GAC_MSIL\System.Runtime\v4.0_4.0.0.0__b03f5f7f11d50a3a\System.Runtime.dll'
foreach($path in @($compiler,(Join-Path $metadata 'Windows.Devices.winmd'),(Join-Path $metadata 'Windows.Foundation.winmd'),$runtime)) { if(-not(Test-Path -LiteralPath $path -PathType Leaf)){throw 'Required official Windows compiler metadata missing'} }
$null=[IO.Directory]::CreateDirectory($out)
$arguments=@('/nologo','/codepage:65001','/target:exe','/platform:x64','/optimize+','/warnaserror+',('/out:'+(Join-Path $out 'GodUseVPN.Ultra.LocationConsumer.exe')),('/win32manifest:'+(Join-Path $PSScriptRoot 'app.manifest')),('/reference:'+(Join-Path $metadata 'Windows.Devices.winmd')),('/reference:'+(Join-Path $metadata 'Windows.Foundation.winmd')),('/reference:'+$runtime),'/reference:System.Web.Extensions.dll','/reference:System.Windows.Forms.dll','/reference:System.Drawing.dll',(Join-Path $PSScriptRoot 'LocationConsumer.cs'))
& $compiler @arguments
if($LASTEXITCODE -ne 0){throw 'Location consumer static compilation failed'}
[IO.File]::WriteAllText((Join-Path $out 'GodUseVPN.Ultra.LocationConsumer.exe.config'),'<?xml version="1.0"?><configuration><startup><supportedRuntime version="v4.0" sku=".NETFramework,Version=v4.6.2"/></startup></configuration>',(New-Object Text.UTF8Encoding($false)))
Write-Output 'Compiled independent read-only location consumer; not executed.'
