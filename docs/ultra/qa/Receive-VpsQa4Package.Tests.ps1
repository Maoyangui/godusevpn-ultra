# AST-selected pure helpers and in-memory manifest fixtures only.
# This test never invokes the receiver main body, Win32/registry APIs, a
# process, a product, a URL or a real ProgramData path.
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$source = Join-Path $PSScriptRoot 'Receive-VpsQa4Package.ps1'
$tokens = $null
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($source, [ref]$tokens, [ref]$errors)
if ($errors.Count -ne 0) { throw 'receiver_parser_failed' }

$functions = @($ast.FindAll({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] }, $true))
$allow = @(
    'Assert-ReceiveStageID', 'Get-ReceiveHash', 'Test-ReceiveGistUrl',
    'Assert-ReceiveManifest'
)
foreach ($name in $allow) {
    $found = @($functions | Where-Object { $_.Name -ceq $name })
    if ($found.Count -ne 1) { throw 'pure_function_not_found' }
    . ([scriptblock]::Create($found[0].Extent.Text))
}

$script:ReceiveFixedHash = 'fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144'
$script:ReceiveFixedSize = [long]37933559
$script:ReceiveChunkBytes = 2097152
$script:ReceiveMaxParts = 256
$script:ReceiveMaxManifestBytes = 131072

function Check([bool]$Value, [string]$Code) {
    if (-not $Value) { throw $Code }
}
function Reject([scriptblock]$Body, [string]$Code) {
    $failed = $false
    try { & $Body | Out-Null } catch { $failed = $true }
    Check $failed $Code
}

$raw = $ast.Extent.Text.Replace("`r`n", "`n")
Check (-not [regex]::IsMatch($raw, '[^\x00-\x7f]')) 'source_must_be_ascii'
Check ($raw.Contains($script:ReceiveFixedHash) -and
       $raw.Contains('37933559') -and $raw.Contains('2097152') -and
       $raw.Contains('184823')) 'fixed_package_constants'
Check ($raw.Contains('AllowAutoRedirect = $false') -and
       $raw.Contains('UseDefaultCredentials = $false') -and
       $raw.Contains('CreateNew')) 'safe_download_and_create_new'
foreach ($forbidden in @('Start-Process', 'Invoke-Expression', 'Invoke-WebRequest',
                         'Expand-Archive', 'InstallUtil', 'Remove-Item', 'Set-Acl')) {
    Check (-not $raw.Contains($forbidden)) ('forbidden_command_' + $forbidden)
}
Check (@($ast.FindAll({
    param($n)
    $n -is [Management.Automation.Language.CommandAst] -and
        $n.GetCommandName() -match 'Start-Process|Invoke-Expression|Invoke-WebRequest|Expand-Archive|Remove-Item|Set-Acl'
}, $true)).Count -eq 0) 'forbidden_command_ast'
Check (@($functions | Group-Object Name | Where-Object { $_.Count -ne 1 }).Count -eq 0) 'duplicate_functions'

Assert-ReceiveStageID '0123456789ab4cde80123456789abcde'
foreach ($bad in @(
    '0123456789ab4cde70123456789abcde',
    '0123456789ab4cde8g0123456789abcde',
    '0123456789ab4cde80123456789abcd',
    '0123456789ab4cde80123456789abcdef',
    '../unsafe'
)) {
    Reject { Assert-ReceiveStageID $bad } 'stage_id_rejected'
}

Check ((Get-ReceiveHash ([Text.Encoding]::UTF8.GetBytes('abc'))) -ceq
       'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad') 'hash_helper'

$goodPrefix = 'https://gist.githubusercontent.com/Maoyangui/0123456789abcdef0123456789abcdef/raw/'
Check (Test-ReceiveGistUrl ($goodPrefix + 'part-0000.b64')) 'gist_url_accept'
foreach ($badUrl in @(
    'http://gist.githubusercontent.com/Maoyangui/0123456789abcdef0123456789abcdef/raw/part.b64',
    'https://gist.githubusercontent.com/maoyangui/0123456789abcdef0123456789abcdef/raw/part.b64',
    'https://gist.githubusercontent.com/Maoyangui/0123456789abcdef0123456789abcdef/raw/part.b64?x=1',
    'https://gist.githubusercontent.com.evil.example/Maoyangui/0123456789abcdef0123456789abcdef/raw/part.b64',
    'https://gist.githubusercontent.com/Maoyangui/0123456789abcdef0123456789abcdef/raw/../part.b64',
    'https://gist.githubusercontent.com/Maoyangui/0123456789abcdef0123456789abcdef/raw/'
)) {
    Check (-not (Test-ReceiveGistUrl $badUrl)) 'gist_url_rejected'
}

$parts = @()
for ($i = 0; $i -lt 19; $i++) {
    $id = ('{0:x32}' -f ($i + 1))
    $parts += [ordered]@{
        url = ('https://gist.githubusercontent.com/Maoyangui/' + $id + '/raw/part-' + ('{0:D4}' -f $i) + '.b64')
        size = $(if ($i -eq 18) { 184823 } else { 2097152 })
        sha256 = ('{0:x64}' -f ($i + 1))
    }
}
$manifestObject = [ordered]@{
    version = 1
    kind = 'GodUseVPN-Ultra-QA-stage'
    sha256 = $script:ReceiveFixedHash
    size = $script:ReceiveFixedSize
    chunkBytes = $script:ReceiveChunkBytes
    parts = $parts
}
$manifestText = $manifestObject | ConvertTo-Json -Compress -Depth 5
$manifestBytes = [Text.Encoding]::UTF8.GetBytes($manifestText)
$manifestBase64 = [Convert]::ToBase64String($manifestBytes)
$validated = Assert-ReceiveManifest $manifestBase64
Check ($validated.Parts.Count -eq 19 -and $validated.Bytes.Length -eq $manifestBytes.Length) 'manifest_accept'
Check ($validated.Parts[18].size -eq 184823) 'manifest_last_part'

Reject {
    $bad = $manifestObject | ConvertTo-Json -Compress -Depth 5
    $bad = $bad.Replace($script:ReceiveFixedHash, ('0' * 64))
    Assert-ReceiveManifest ([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($bad)))
} 'manifest_hash_rejected'
Reject {
    $badObject = $manifestObject | ConvertTo-Json -Compress -Depth 5 | ConvertFrom-Json
    $badObject.parts[0].url = $badObject.parts[1].url
    $bad = $badObject | ConvertTo-Json -Compress -Depth 5
    Assert-ReceiveManifest ([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($bad)))
} 'manifest_duplicate_url_rejected'
Reject {
    $badObject = $manifestObject | ConvertTo-Json -Compress -Depth 5 | ConvertFrom-Json
    $badObject.parts[17].size = 1698002
    $bad = $badObject | ConvertTo-Json -Compress -Depth 5
    Assert-ReceiveManifest ([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($bad)))
} 'manifest_size_rejected'

[Console]::Out.WriteLine('{"scope":"AST_and_synthetic_only","parser":true,"fixedPackage":true,"gistAllowlist":true,"manifestValidation":true,"redirectBlocked":true,"noOverwrite":true,"networkCalled":false,"productExecuted":false}')
