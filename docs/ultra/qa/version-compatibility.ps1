# Pure package-validation fixtures only. Never run script top-level install paths.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$project = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$tempBase = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$fixture = Join-Path $tempBase ('ultra-version-qa-' + [guid]::NewGuid().ToString('N'))
$checks = 0
$good = @('0.02', '0.01-beta', '0.01-beta-qa.2', '0.01-beta-ci.abcdef0', '0.1.0-ultra.3', '1.2.3')
$bad = @('0', '0-beta', '0.1.2.3', 'v0.01-beta', '0.01-', '../0.01-beta', '0.01-beta/value', '0.01-beta value')
$script:UltraName = 'godusevpn-ultra'
$script:UltraProduct = 'GodUseVPN Ultra'
function Parse-Script([string]$Name) {
    $tokens = $null; $parseErrors = $null
    $ast = [Management.Automation.Language.Parser]::ParseFile((Join-Path $project $Name), [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count) { throw "PowerShell parse failed: $Name" }
    return $ast
}
function Require([bool]$Okay, [string]$Message) {
    if (-not $Okay) { throw $Message }
    $script:checks++
}
try {
    [IO.Directory]::CreateDirectory($fixture) | Out-Null
    # Tiny inert text fixtures stand in for EXEs. Nothing is executed.
    $files = @('godusevpn.exe', 'godusevpn-cli.exe', 'godusevpn-svc.exe', 'Install-Ultra.ps1', 'Uninstall-Ultra.ps1', 'browser-extension/manifest.json')
    [IO.Directory]::CreateDirectory((Join-Path $fixture 'browser-extension')) | Out-Null
    foreach ($name in $files) { [IO.File]::WriteAllText((Join-Path $fixture $name), 'inert version fixture') }
    $arch = switch ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) { 'X64' { 'amd64' } 'Arm64' { 'arm64' } default { throw 'Unsupported fixture host architecture.' } }
    function Write-Descriptor([string]$Version) {
        $descriptor = @{product='GodUseVPN Ultra';namespace='godusevpn-ultra';version=$Version;architecture=$arch;unsigned=$true;sourceBaseline='b51aed4'}
        [IO.File]::WriteAllText((Join-Path $fixture 'ULTRA_PACKAGE.json'), ($descriptor | ConvertTo-Json -Compress))
        $lines = @($files + 'ULTRA_PACKAGE.json' | ForEach-Object {
            '{0}  {1}' -f (Get-FileHash -LiteralPath (Join-Path $fixture $_) -Algorithm SHA256).Hash.ToLowerInvariant(), $_
        })
        [IO.File]::WriteAllLines((Join-Path $fixture 'SHA256SUMS'), $lines)
    }
    foreach ($scriptName in @('installer/ultra/Install-Ultra.ps1', 'installer/ultra/Uninstall-Ultra.ps1')) {
        $ast = Parse-Script $scriptName
        foreach ($name in @('Get-Sha256Hash', 'Assert-LocalPath', 'Assert-NoReparse', 'Get-Inventory', 'Assert-RelativeName', 'Test-Package')) {
            $definitions = @($ast.FindAll({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name }, $true))
            Require ($definitions.Count -eq 1) "Missing/duplicate fixture function: $name"
            # Define only the allowlisted validators; never dot-source the installer.
            . ([scriptblock]::Create($definitions[0].Extent.Text))
        }
        foreach ($version in $good) {
            Write-Descriptor $version
            $result = Test-Package $fixture
            Require ($result.Descriptor.version -ceq $version) "Valid version rejected: $scriptName / $version"
        }
        foreach ($version in $bad) {
            Write-Descriptor $version
            $rejected = $false
            try { $null = Test-Package $fixture } catch { $rejected = $true }
            Require $rejected "Invalid version accepted: $scriptName / $version"
        }
    }
    $build = Parse-Script 'build-ultra.ps1'
    $parameter = @($build.ParamBlock.Parameters | Where-Object { $_.Name.VariablePath.UserPath -eq 'Version' })[0]
    $validation = @($parameter.Attributes | Where-Object { $_.TypeName.Name -eq 'ValidatePattern' })[0].PositionalArguments[0].Value
    foreach ($version in $good) { Require ($version -cmatch $validation) "Build version rejected: $version" }
    foreach ($version in $bad) { Require ($version -cnotmatch $validation) "Invalid build version accepted: $version" }
    # Exercise the real, pure CLI manifest validator copied byte-for-byte to Temp.
    $cliFixture = Join-Path $fixture 'cli-test'
    [IO.Directory]::CreateDirectory($cliFixture) | Out-Null
    [IO.File]::Copy((Join-Path $project 'internal/cli/uninstall_manifest.go'), (Join-Path $cliFixture 'uninstall_manifest.go'))
    [IO.File]::WriteAllText((Join-Path $cliFixture 'go.mod'), "module ultra-version-fixture`ngo 1.24`n")
    [IO.File]::WriteAllText((Join-Path $cliFixture 'version_test.go'), @'
package cli
import("encoding/json";"strings";"testing")
func TestReleaseVersionLabels(t *testing.T){
 names:=[]string{"godusevpn.exe","godusevpn-cli.exe","godusevpn-svc.exe","ULTRA_PACKAGE.json","Install-Ultra.ps1","Uninstall-Ultra.ps1","browser-extension/manifest.json"}
 hash:=strings.Repeat("a",64); hashes:=map[string]string{"SHA256SUMS":hash};var lines []string
 for _,name:=range names{hashes[name]=hash;lines=append(lines,hash+"  "+name)}
 for _,version:=range []string{"0.01-beta","0.01-beta-qa.2","0.01-beta-ci.abcdef0","0.1.0-ultra.3","1.2.3"}{
  d,_:=json.Marshal(map[string]any{"product":"GodUseVPN Ultra","namespace":"godusevpn-ultra","version":version})
  if err:=verifyUninstallManifest([]byte(strings.Join(lines,"\n")+"\n"),d,hashes);err!=nil{t.Fatal(version,err)}
 }
}
'@)
    $previousToolchain = [Environment]::GetEnvironmentVariable('GOTOOLCHAIN', 'Process')
    $previousWorkspace = [Environment]::GetEnvironmentVariable('GOWORK', 'Process')
    Push-Location -LiteralPath $cliFixture
    try {
        $env:GOTOOLCHAIN = 'local'; $env:GOWORK = 'off'
        & go test -count=1 -mod=readonly .
        if ($LASTEXITCODE) { throw 'Pure CLI version fixture failed.' }
        $checks += $good.Count
    } finally {
        [Environment]::SetEnvironmentVariable('GOTOOLCHAIN', $previousToolchain, 'Process')
        [Environment]::SetEnvironmentVariable('GOWORK', $previousWorkspace, 'Process')
        Pop-Location
    }
    [pscustomobject]@{status='passed';checks=$checks;scope='AST-selected package validators and pure CLI manifest; inert Temp files only'} | ConvertTo-Json -Compress
} finally {
    # One shell; target is this exact new Temp fixture, never a product path.
    $resolved = [IO.Path]::GetFullPath($fixture)
    if (-not $resolved.StartsWith($tempBase + '\', [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolved) -notmatch '^ultra-version-qa-[a-f0-9]{32}$') { throw 'Unsafe fixture cleanup target.' }
    if ([IO.Directory]::Exists($resolved)) { [IO.Directory]::Delete($resolved, $true) }
}
