# Pure parser/fixture QA. This test reads the frozen local ZIP and exercises
# the archive/manifest validators in memory; it never invokes a product,
# installer, service, network command or the wrapper's main block.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$source=Join-Path $PSScriptRoot 'Expand-VpsQa4Package.ps1'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($source,[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'parser_failed' }
$functions=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw ('assertion_failed_'+$script:checks) };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }
foreach ($name in @('Get-Qa4Hash','Assert-Qa4ID','Get-Qa4Components','Assert-Qa4PrivateSD','Assert-Qa4Name','Read-Qa4Manifest','Read-Qa4ArchiveEntries','Read-Qa4Bundle')) {
    $found=@($functions | Where-Object {$_.Name -ceq $name})
    Check ($found.Count -eq 1)
    . ([scriptblock]::Create($found[0].Extent.Text))
}
foreach ($statement in $ast.EndBlock.Statements) {
    if ($statement -is [Management.Automation.Language.AssignmentStatementAst] -and
        $statement.Left -is [Management.Automation.Language.VariableExpressionAst] -and
        $statement.Left.VariablePath.UserPath -cin @('script:qa4ZipLength','script:qa4ZipHash','script:qa4ManifestHash','script:qa4Prefix','script:qa4Hashes','script:qa4Lengths')) {
        . ([scriptblock]::Create($statement.Extent.Text))
    }
}
Check ($script:qa4ZipLength -eq 37933559L)
Check ($script:qa4ZipHash -ceq 'fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144')
Check ($script:qa4Hashes.Count -eq 3 -and $script:qa4Lengths.Count -eq 3)
Assert-Qa4ID '123456789abc4def8abcdef012345678';$checks++
foreach ($bad in @('123456789abc3def8abcdef012345678','123456789abc4def1abcdef012345678','../path','')) { Reject { Assert-Qa4ID $bad } }
Check (@(Get-Qa4Components 'C:\ProgramData\GodUseVPN-Ultra-QA\fixture').Count -eq 4)
foreach ($bad in @('\\invalid.example\share\file','\\?\C:\file','C:\folder\..\file','C:\folder\file:stream','C:\folder\NUL.txt','C:\folder\a.')) { Reject { Get-Qa4Components $bad } }
Assert-Qa4PrivateSD 'O:SYG:SYD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)';$checks++
foreach ($sddl in @('O:BUG:SYD:P(A;;FA;;;SY)','O:SYG:SYD:P(A;;FR;;;BU)','O:SYG:SYD:NO_ACCESS_CONTROL')) { Reject { Assert-Qa4PrivateSD $sddl } }
foreach ($bad in @('../outside','safe\name','safe:name','safe..','CON.txt')) { Reject { Assert-Qa4Name $bad } }

$repo=(Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
$zipPath=Join-Path $repo 'dist\godusevpn-ultra-0.01-beta-qa.4-windows-amd64.zip'
Check ([IO.File]::Exists($zipPath))
$zipBytes=[IO.File]::ReadAllBytes($zipPath)
Check ($zipBytes.Length -eq $script:qa4ZipLength)
$bundle=Read-Qa4Bundle $zipBytes
Check ($bundle.payload.Count -eq 407 -and $bundle.hashes.Count -eq 406)
Check ($bundle.payload['SHA256SUMS'].Length -eq 47701)
foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) {
    Check ((Get-Qa4Hash $bundle.payload[$name]) -ceq $script:qa4Hashes[$name])
}
Check ((Get-Qa4Hash $bundle.payload['ULTRA_PACKAGE.json']) -ceq $bundle.hashes['ULTRA_PACKAGE.json'])
Check ($bundle.payload.ContainsKey('docs/ultra/qa/Apply-VpsDelta.ps1'))
Reject { Read-Qa4Bundle ([byte[]]($zipBytes[0..($zipBytes.Length-2)])) }

Add-Type -AssemblyName System.IO.Compression
function New-Qa4HostileZip {
    $memory=[IO.MemoryStream]::new();$zip=[IO.Compression.ZipArchive]::new($memory,[IO.Compression.ZipArchiveMode]::Create,$true)
    try {
        $dir=$zip.CreateEntry($script:qa4Prefix)
        for ($i=0;$i -lt 406;$i++) {
            $entry=$zip.CreateEntry($script:qa4Prefix+('safe{0:D4}.txt' -f $i))
            $stream=$entry.Open();try {$bytes=[Text.Encoding]::ASCII.GetBytes('fixture');$stream.Write($bytes,0,$bytes.Length)}finally{$stream.Dispose()}
        }
        $bad=$zip.CreateEntry($script:qa4Prefix+'../outside.txt')
        $stream=$bad.Open();try {$bytes=[Text.Encoding]::ASCII.GetBytes('fixture');$stream.Write($bytes,0,$bytes.Length)}finally{$stream.Dispose()}
    } finally {$zip.Dispose()}
    $bytes=$memory.ToArray();$memory.Dispose();return ,$bytes
}
Reject { Read-Qa4ArchiveEntries (New-Qa4HostileZip) }
$backslashZip=[IO.MemoryStream]::new();$backslashArchive=[IO.Compression.ZipArchive]::new($backslashZip,[IO.Compression.ZipArchiveMode]::Create,$true)
try {
    $null=$backslashArchive.CreateEntry($script:qa4Prefix.Replace('/','\'))
    for ($i=0;$i -lt 407;$i++) {
        $entry=$backslashArchive.CreateEntry($script:qa4Prefix.Replace('/','\')+('safe{0:D4}.txt' -f $i))
        $stream=$entry.Open();try {$bytes=[Text.Encoding]::ASCII.GetBytes('fixture');$stream.Write($bytes,0,$bytes.Length)}finally{$stream.Dispose()}
    }
} finally {$backslashArchive.Dispose()}
$backslashPayload=Read-Qa4ArchiveEntries $backslashZip.ToArray()
Check ($backslashPayload.Count -eq 407)
$backslashZip.Dispose()
[Console]::Out.WriteLine(('{"scope":"frozen_zip_and_in_memory_hostile_fixtures","checks":'+$checks+',"zipEntries":408,"payloadFiles":407,"manifestFiles":406,"productExecuted":false,"mainExecuted":false}'))
