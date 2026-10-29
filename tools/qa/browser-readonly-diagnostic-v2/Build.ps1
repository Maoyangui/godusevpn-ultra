$ErrorActionPreference = 'Stop'
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$outputDirectory = Join-Path $workspace '.codex\tmp\browser-readonly-diagnostic-v2'
$compiler = 'C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe'
if (-not [IO.File]::Exists($compiler)) { throw 'Fixed .NET Framework x64 compiler is unavailable.' }
[IO.Directory]::CreateDirectory($outputDirectory) | Out-Null
$output = Join-Path $outputDirectory 'BrowserReadOnlyDiagnosticV2.exe'
Push-Location -LiteralPath $PSScriptRoot
try {
    & $compiler /nologo /warnaserror+ /optimize+ /target:exe /platform:x64 /reference:System.Web.Extensions.dll /win32manifest:app.manifest "/out:$output" BrowserReadOnlyDiagnosticV2.cs
    if ($LASTEXITCODE -ne 0) { throw 'Diagnostic compilation failed.' }
} finally { Pop-Location }
# Compilation only. A rebuild is a new candidate and requires a fresh byte/hash
# review and launcher constant update before staging; never run the tool here.
[pscustomobject]@{ bytes = (Get-Item -LiteralPath $output).Length; sha256 = (Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash.ToLowerInvariant() }
