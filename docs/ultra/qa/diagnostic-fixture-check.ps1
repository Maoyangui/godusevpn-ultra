# Exercise the existing diagnostic settings-copy test without collecting host
# interfaces or resolving the sample subscription. Product sources stay intact.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$fixtureDir = Join-Path ([IO.Path]::GetTempPath()) ('ultra-diag-fixture-' + [Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($fixtureDir) | Out-Null
$utf8 = New-Object Text.UTF8Encoding($false)
$source = [IO.File]::ReadAllText((Join-Path $repo 'internal/daemon/diag_test.go'))
$source = $source.Replace('"encoding/json"', '"encoding/json"' + "`n" + '"errors"' + "`n" + '"net/http"')
$needle = 's.Profiles = []settings.Profile{{ID: "a", Name: "a", URL: "https://panel.example/sub/secret-token"}}'
if ($source.Split(@($needle), [StringSplitOptions]::None).Count -ne 2) { throw 'Expected a single fixed diagnostic test fixture.' }
$source = $source.Replace($needle, 'd.http = &http.Client{Transport: ultraDiagFixtureTransport{}}' + "`n" + $needle)
$source += @'

// The original test still checks that export leaves the real settings copy intact.
// Only the synthetic follow-up fetch is prevented from reaching the network.
type ultraDiagFixtureTransport struct{}
func (ultraDiagFixtureTransport) RoundTrip(*http.Request) (*http.Response, error) {
    return nil, errors.New("synthetic diagnostic fixture: network disabled")
}
'@
[IO.File]::WriteAllText((Join-Path $fixtureDir 'diag_test.go'), $source, $utf8)
$diag = @'
package daemon
func sysDiag(add func(name, content string)) {
    add("route.txt", "SYNTHETIC ROUTE FIXTURE")
    add("ipconfig.txt", "SYNTHETIC INTERFACE FIXTURE")
    add("netsh-interfaces.txt", "SYNTHETIC INTERFACE LIST")
}
'@
[IO.File]::WriteAllText((Join-Path $fixtureDir 'diag_windows.go'), $diag, $utf8)
$replace = @{}
$replace[(Join-Path $repo 'internal/daemon/diag_windows.go')] = Join-Path $fixtureDir 'diag_windows.go'
$replace[(Join-Path $repo 'internal/daemon/diag_test.go')] = Join-Path $fixtureDir 'diag_test.go'
[IO.File]::WriteAllText((Join-Path $fixtureDir 'overlay.json'), (@{ Replace = $replace } | ConvertTo-Json -Depth 4), $utf8)
$saved = @{}
foreach ($key in @('GODUSEVPN_WFP_LIVE','GODUSEVPN_WFP_KEEP','GODUSEVPN_ULTRA_REGISTRY_LIVE')) {
    $saved[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
    [Environment]::SetEnvironmentVariable($key, '0', 'Process')
}
Push-Location -LiteralPath $repo
try {
    & go test -count=1 -overlay (Join-Path $fixtureDir 'overlay.json') -tags with_quic,with_utls,with_clash_api,with_gvisor -run '^TestExportDiagKeepsSettingsIntact$' ./internal/daemon
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic diagnostic settings-copy check failed.' }
    Write-Output 'PASS: original settings-copy test with synthetic system diagnostics and a network-disabled fetch; real diagnostic privacy is not certified.'
} finally {
    foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key, $saved[$key], 'Process') }
    Pop-Location
}
