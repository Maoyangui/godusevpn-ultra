$ErrorActionPreference = 'Stop'
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$outputDirectory = Join-Path $workspace '.codex\tmp\browser-readonly-diagnostic'
$compiler = 'C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe'
if (-not [IO.File]::Exists($compiler)) { throw 'Fixed .NET Framework x64 compiler is unavailable.' }
[IO.Directory]::CreateDirectory($outputDirectory) | Out-Null
$output = Join-Path $outputDirectory 'BrowserReadOnlyDiagnostic.exe'
Push-Location -LiteralPath $PSScriptRoot
try {
    & $compiler /nologo /warnaserror+ /optimize+ /target:exe /platform:x64 /reference:System.Web.Extensions.dll /win32manifest:app.manifest "/out:$output" BrowserReadOnlyDiagnostic.cs
    if ($LASTEXITCODE -ne 0) { throw 'Diagnostic compilation failed.' }
} finally { Pop-Location }
# Compilation only. Never invoke the produced executable here.
[pscustomobject]@{ bytes = (Get-Item -LiteralPath $output).Length; sha256 = (Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash.ToLowerInvariant() }
