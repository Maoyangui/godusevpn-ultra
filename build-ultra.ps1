# Builds an isolated, unsigned Windows preview. Never installs or starts it.
[CmdletBinding()]
param(
    [ValidatePattern('^[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-[A-Za-z0-9.-]+)?$')]
    [string]$Version = '0.02',
    [ValidateSet('amd64', 'arm64')]
    [string[]]$Architecture = @('amd64'),
    [switch]$SkipTests,
    [switch]$BrowserEngineAcceptance
)
$ErrorActionPreference = 'Stop'
$projectRoot = [IO.Path]::GetFullPath($PSScriptRoot)
$savedEnvironment = @{}
foreach ($key in @('GOOS', 'GOARCH', 'CGO_ENABLED', 'GODUSEVPN_WFP_LIVE', 'GODUSEVPN_WFP_KEEP', 'GODUSEVPN_ULTRA_REGISTRY_LIVE')) {
    $savedEnvironment[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
}
Push-Location -LiteralPath $projectRoot
try {
    $sourceCommit = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $sourceCommit -notmatch '^[0-9a-f]{40}$') { throw 'Cannot identify source commit.' }
    $sourceDirty = -not [string]::IsNullOrWhiteSpace((& git status --porcelain | Out-String))
    $env:GOOS = 'windows'; $env:CGO_ENABLED = '0'
    $env:GODUSEVPN_WFP_LIVE = '0'; $env:GODUSEVPN_WFP_KEEP = '0'; $env:GODUSEVPN_ULTRA_REGISTRY_LIVE = '0'
    $tags = 'with_quic,with_utls,with_clash_api,with_gvisor'
    $ld = "-s -w -X github.com/Maoyangui/godusevpn/internal/buildinfo.Version=$Version"
    # PE numeric versions cannot preserve a leading-zero release label.
    # ProductVersion and the application keep the exact requested version.
    $numericParts = @($Version.Split('-')[0].Split('.') | ForEach-Object { [int]$_ })
    while ($numericParts.Count -lt 4) { $numericParts += 0 }
    if (@($numericParts | Where-Object { $_ -lt 0 -or $_ -gt 65535 }).Count -ne 0) { throw 'Numeric version components must fit a Windows VERSIONINFO word.' }
    $fileVersion = $numericParts -join '.'
    $winres = Get-Command 'go-winres' -ErrorAction SilentlyContinue
    if (-not $winres) {
        $goBin = (& go env GOPATH).Trim()
        if ($LASTEXITCODE -ne 0) { throw 'Cannot locate Go GOPATH.' }
        $candidate = Join-Path $goBin 'bin/go-winres.exe'
        if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) { throw 'Install go-winres v0.3.3 first: go install github.com/tc-hib/go-winres@v0.3.3' }
        $winresPath = $candidate
    } else { $winresPath = $winres.Source }
    $resourceConfigRoot = Join-Path $projectRoot ".codex/tmp/winres-$Version"
    New-Item -ItemType Directory -Force -Path $resourceConfigRoot | Out-Null
    Copy-Item -LiteralPath (Join-Path $projectRoot 'cmd/godusevpn/build/appicon.png') -Destination (Join-Path $resourceConfigRoot 'appicon.png') -Force
    foreach ($name in @('godusevpn', 'godusevpn-svc', 'godusevpn-cli')) {
        $resourceConfig = Get-Content -LiteralPath (Join-Path $projectRoot "winres/$name.json") -Raw | ConvertFrom-Json
        $resourceConfig.RT_MANIFEST.'#1'.'0409'.identity.version = $fileVersion
        $resourceConfig.RT_GROUP_ICON.APP.'0000'[0] = 'appicon.png'
        $resourceConfigPath = Join-Path $resourceConfigRoot "$name.json"
        # Windows PowerShell 5's `-Encoding UTF8` adds a BOM. go-winres expects
        # JSON to start with `{`, so write an explicit BOM-free UTF-8 file.
        $json = $resourceConfig | ConvertTo-Json -Depth 20
        [IO.File]::WriteAllText($resourceConfigPath, $json, [Text.UTF8Encoding]::new($false))
        & $winresPath make --in $resourceConfigPath --out "cmd/$name/rsrc" --arch ($Architecture -join ',') --file-version $fileVersion --product-version $Version
        if ($LASTEXITCODE -ne 0) { throw "Resource build failed: $name" }
    }
    if (-not $SkipTests) {
        $env:GOARCH = (& go env GOHOSTARCH).Trim()
        & go test -count=1 -tags $tags ./internal/... ./cmd/godusevpn-cli ./cmd/godusevpn-svc ./web
        if ($LASTEXITCODE -ne 0) { throw 'Go regression failed.' }
        & go test -count=1 -tags desktop,production ./cmd/godusevpn
        if ($LASTEXITCODE -ne 0) { throw 'Desktop regression failed.' }
        & node browser-extension/tests.cjs
        if ($LASTEXITCODE -ne 0) { throw 'Browser helper regression failed.' }
        & node browser-extension/coordinator-tests.cjs
        if ($LASTEXITCODE -ne 0) { throw 'Browser operation queue regression failed.' }
    }
    # Compile the independent managed host; never execute its WebView2 path.
    # This first host targets x64 only. ARM64 packages do not claim it exists.
    $browserHostOutput = Join-Path $projectRoot '.codex/tmp/browser-host-build'
    $locationConsumerOutput = Join-Path $projectRoot ".codex/tmp/location-consumer-build-$Version"
    if ($Architecture -contains 'amd64') {
        & (Join-Path $projectRoot 'browser-host/Build.ps1') -Version $Version -RunPureTests:(-not $SkipTests)
        if ($LASTEXITCODE -ne 0) { throw 'Independent browser host compilation failed.' }
        & (Join-Path $projectRoot 'location-consumer/Build.ps1') -OutputDirectory $locationConsumerOutput
        if ($LASTEXITCODE -ne 0) { throw 'Independent location consumer compilation failed.' }
        & (Join-Path $projectRoot 'internal/metadatatool/Prepare.ps1')
        if ($LASTEXITCODE -ne 0) { throw 'Pinned metadata tool preparation failed.' }
    }
    foreach ($arch in $Architecture) {
        $env:GOARCH = $arch
        # Release/package names are filesystem-facing identifiers. Keep them
        # lowercase even when a caller uses an uppercase prerelease label for
        # the embedded product version.
        $packageVersion = $Version.ToLowerInvariant()
        $packageName = "godusevpn-ultra-$packageVersion-windows-$arch"
        if ($packageName -cmatch '[A-Z]') { throw "Package name must be lowercase: $packageName" }
        $out = Join-Path $projectRoot "dist/$packageName"
        # Refuse to mix stale files into a release; this script never recursively deletes.
        if (Test-Path -LiteralPath $out) { throw "Output exists; choose a new version or archive it yourself: $out" }
        New-Item -ItemType Directory -Path $out | Out-Null
        foreach ($name in @('godusevpn-svc', 'godusevpn-cli', 'godusevpn')) {
            $buildTags = $tags; $buildLD = $ld
            if ($name -eq 'godusevpn') { $buildTags = 'desktop,production'; if ($BrowserEngineAcceptance) { $buildTags += ',browserengine_acceptance' }; $buildLD += ' -H windowsgui' }
            & go build -tags $buildTags -trimpath -ldflags $buildLD -o (Join-Path $out "$name.exe") "./cmd/$name"
            if ($LASTEXITCODE -ne 0) { throw "Build failed: $name ($arch). Partial output retained." }
        }
        if ($arch -eq 'amd64') {
            $hostDestination = Join-Path $out 'browser-host'
            New-Item -ItemType Directory -Path $hostDestination | Out-Null
            foreach ($hostFile in @('GodUseVPN.Ultra.BrowserHost.exe', 'GodUseVPN.Ultra.BrowserHost.exe.config', 'Microsoft.Web.WebView2.Core.dll', 'Microsoft.Web.WebView2.WinForms.dll', 'WebView2Loader.dll', 'Microsoft.Web.WebView2.LICENSE.txt')) {
                Copy-Item -LiteralPath (Join-Path $browserHostOutput $hostFile) -Destination $hostDestination
            }
            Copy-Item -LiteralPath (Join-Path $projectRoot 'browser-host/README.md') -Destination $hostDestination
            $locationDestination = Join-Path $out 'location-consumer'
            New-Item -ItemType Directory -Path $locationDestination | Out-Null
            foreach ($consumerFile in @('GodUseVPN.Ultra.LocationConsumer.exe', 'GodUseVPN.Ultra.LocationConsumer.exe.config')) {
                Copy-Item -LiteralPath (Join-Path $locationConsumerOutput $consumerFile) -Destination $locationDestination
            }
            $metadataDestination = Join-Path $out 'metadata-tool'
            New-Item -ItemType Directory -Path $metadataDestination | Out-Null
            Copy-Item -LiteralPath (Join-Path $projectRoot '.codex/tmp/metadata-tool-build/exiftool-13.59') -Destination $metadataDestination -Recurse
            Copy-Item -LiteralPath (Join-Path $projectRoot 'internal/metadatatool/bundle.sha256') -Destination (Join-Path $metadataDestination 'exiftool-13.59.sha256')
        }
        foreach ($file in @('LICENSE', 'NOTICE', 'README.md', 'README.zh-CN.md', 'README-ULTRA.zh-CN.md', 'PROJECT_PLAN.zh-CN.md')) {
            Copy-Item -LiteralPath (Join-Path $projectRoot $file) -Destination $out
        }
        $engineNotices = Join-Path $projectRoot 'internal/browserengine/third_party'
        if (Test-Path -LiteralPath $engineNotices -PathType Container) {
            $noticesDestination = Join-Path $out 'third-party-notices'
            New-Item -ItemType Directory -Path $noticesDestination | Out-Null
            Copy-Item -LiteralPath $engineNotices -Destination (Join-Path $noticesDestination 'browserengine') -Recurse
        }
        Copy-Item -LiteralPath (Join-Path $projectRoot 'browser-extension') -Destination $out -Recurse
        Copy-Item -LiteralPath (Join-Path $projectRoot 'installer/ultra/Install-Ultra.ps1') -Destination $out
        Copy-Item -LiteralPath (Join-Path $projectRoot 'installer/ultra/Uninstall-Ultra.ps1') -Destination $out
        $packageDocs = Join-Path $out 'docs/ultra'
        New-Item -ItemType Directory -Path (Join-Path $out 'docs') | Out-Null
        # Keep the audit's relative evidence links usable in the delivered ZIP.
        # QA scripts are documentation, not install/start hooks; none are run.
        Copy-Item -LiteralPath (Join-Path $projectRoot 'docs/ultra') -Destination (Join-Path $out 'docs') -Recurse
        $descriptor = [ordered]@{ product = 'GodUseVPN Ultra'; namespace = 'godusevpn-ultra'; version = $Version; architecture = $arch; unsigned = $true; sourceBaseline = 'b51aed4'; sourceCommit = $sourceCommit; sourceDirty = $sourceDirty }
        $descriptor.nativeBrowser = [ordered]@{ acceptanceBuild = [bool]$BrowserEngineAcceptance; vmValidated = $false; engines = @('chrome-for-testing','firefox') }
        if ($arch -eq 'amd64') { $descriptor.browserHost = [ordered]@{ schemaVersion = 1; implementation = 'webview2-winforms'; architecture = 'amd64'; sdkVersion = '1.0.4314.35'; nativeRuntimeBundled = $false } }
        if ($arch -eq 'amd64') { $descriptor.locationConsumer = [ordered]@{ schemaVersion = 1; implementation = 'winrt-geolocator'; architecture = 'amd64'; minimumFramework = '4.6.2'; nativeRuntimeBundled = $false; vmValidated = $false } }
        if ($arch -eq 'amd64') { $descriptor.metadataTool = [ordered]@{ version = '13.59'; implementation = 'exiftool'; manifestSHA256 = 'd1316fddf986e2d17761f85034c753b28375f25bf73235c1084a7c5a38e22faf'; fileCount = 1501; vmValidated = $false } }
        $descriptor | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $out 'ULTRA_PACKAGE.json') -Encoding UTF8
        $peResult = & node tools/verify-ultra-package.cjs $out
        if ($LASTEXITCODE -ne 0) { throw 'PE resource/identity verification failed.' }
        $peResult | Set-Content -LiteralPath (Join-Path $packageDocs 'build-results.json') -Encoding UTF8
        $hashes = @(Get-ChildItem -LiteralPath $out -File -Recurse | Sort-Object FullName | ForEach-Object {
            $relative = $_.FullName.Substring($out.Length + 1).Replace('\', '/')
            '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $relative
        })
        $hashes | Set-Content -LiteralPath (Join-Path $out 'SHA256SUMS') -Encoding ASCII
        $archive = "$out.zip"
        if (Test-Path -LiteralPath $archive) { throw "Archive exists: $archive" }
        # Use the bundled standard-library ZIP writer so member names use `/`
        # and no platform-specific UID/GID extra fields are emitted.
        & python (Join-Path $projectRoot 'tools/create-normalized-zip.py') $out $archive
        if ($LASTEXITCODE -ne 0) { throw 'ZIP archive creation failed.' }
        # GNU sha256sum on the Linux release runner treats CR as part of the name.
        [IO.File]::WriteAllText("$archive.sha256", ('{0}  {1}' -f (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant(), ([IO.Path]::GetFileName($archive))) + "`n", [Text.Encoding]::ASCII)
        & node tools/audit-built-package.cjs $out
        if ($LASTEXITCODE -ne 0) { throw 'Final payload/ZIP/embedded frontend audit failed. Output retained for review.' }
        Write-Host "Built $archive (unsigned; no installation or network-policy changes)."
    }
} finally {
    foreach ($key in $savedEnvironment.Keys) { [Environment]::SetEnvironmentVariable($key, $savedEnvironment[$key], 'Process') }
    Pop-Location
}
