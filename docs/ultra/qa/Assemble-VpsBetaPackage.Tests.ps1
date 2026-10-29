# AST-selected pure functions, in-memory ZIP/manifest fixtures only.
# No main/native filesystem method, product, installer or package assembly.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Assemble-VpsBetaPackage.ps1'),[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'parser_failed' }
$allowed=@('Get-AssembleHash','Assert-AssembleID','Get-AssembleComponents','Assert-AssemblePrivateSD','New-AssembleNative','Assert-AssembleKeys','Assert-AssembleInteger','Assert-AssembleName','Read-AssembleManifest','Read-AssembleEntries','Read-AssembleBundle')
foreach ($name in $allowed) {
    $found=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
    if ($found.Count -ne 1) { throw 'function_missing' }
    . ([scriptblock]::Create($found[0].Extent.Text))
}
foreach ($name in @('script:assembleZipHash','script:assembleManifestHash','script:assembleHashes')) {
    $found=@($ast.EndBlock.Statements | Where-Object { $_ -is [Management.Automation.Language.AssignmentStatementAst] -and $_.Left -is [Management.Automation.Language.VariableExpressionAst] -and $_.Left.VariablePath.UserPath -ceq $name })
    if ($found.Count -ne 1) { throw 'constant_missing' }
    . ([scriptblock]::Create($found[0].Extent.Text))
}
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw 'assertion_failed' };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }
$native=New-AssembleNative
$import=@($native.GetMethod('CreateDirectoryW').GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
Check ($import.SetLastError -and $import.ExactSpelling)
Check ($native.GetMethod('CreateFileW').ReturnType -eq [IntPtr])
Assert-AssembleID '123456789abc4def8abcdef012345678';$checks++
Reject { Assert-AssembleID '123456789abc3def8abcdef012345678' }
Check (@(Get-AssembleComponents 'C:\ProgramData\GodUseVPN-Ultra-QA\fixture').Count -eq 4)
foreach ($name in @('../outside','/absolute','a\b','a:stream','folder/NUL.txt','trailing.','space ','folder//file',('COM'+[char]0xb9+'.txt'))) { Reject { Assert-AssembleName $name } }
foreach ($path in @('\\invalid.example\share','C:relative','C:\folder\..\file',('C:\folder\COM'+[char]0xb9+'.txt'))) { Reject { Get-AssembleComponents $path } }
Assert-AssemblePrivateSD 'O:SYG:SYD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)';$checks++
Reject { Assert-AssemblePrivateSD 'O:BUG:SYD:P(A;;FA;;;SY)' }
Reject { Assert-AssemblePrivateSD 'O:SYG:SYD:P(A;;FR;;;BU)' }
Reject { Assert-AssembleInteger '256' 256 256 }
$nonEXEs=@('ULTRA_PACKAGE.json')+@(0..297 | ForEach-Object { 'docs/fixture-'+$_+'.txt' })
$lines=@($nonEXEs | ForEach-Object { ('a'*64)+'  '+$_ })
foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) { $lines+=$script:assembleHashes[$name]+'  '+$name }
$manifest=[Text.Encoding]::UTF8.GetBytes(($lines -join "`n")+"`n")
$hashes=Read-AssembleManifest $manifest
Check ($hashes.Count -eq 302)
Reject { Read-AssembleManifest ([Text.Encoding]::UTF8.GetBytes(($lines+$lines[0] -join "`n")+"`n")) }
Reject { Read-AssembleManifest ([Text.Encoding]::UTF8.GetBytes(($lines -join "`n").Replace('ULTRA_PACKAGE.json','../outside'))) }
Reject { Read-AssembleManifest ([Text.Encoding]::UTF8.GetBytes(($lines -join "`n").Replace($script:assembleHashes['godusevpn.exe'],('0'*64)))) }
Add-Type -AssemblyName System.IO.Compression
function New-AssembleZipFixture([string[]]$Names,[bool]$Link=$false) {
    $memory=[IO.MemoryStream]::new();$zip=[IO.Compression.ZipArchive]::new($memory,[IO.Compression.ZipArchiveMode]::Create,$true)
    try {
        foreach ($name in $Names) {
            $entry=$zip.CreateEntry($name);if ($Link) { $entry.ExternalAttributes=[int](-1610612736) }
            $stream=$entry.Open();try { $data=[Text.Encoding]::ASCII.GetBytes('inert fixture');$stream.Write($data,0,$data.Length) } finally { $stream.Dispose() }
        }
    } finally { $zip.Dispose() }
    $bytes=$memory.ToArray();$memory.Dispose();return ,$bytes
}
$members=@('SHA256SUMS')+$nonEXEs
$zip=New-AssembleZipFixture $members
Check ((Read-AssembleEntries $zip).Count -eq 300)
Reject { Read-AssembleEntries (New-AssembleZipFixture ($members+'extra')) }
$duplicates=@($members);$duplicates[1]=$duplicates[0]
Reject { Read-AssembleEntries (New-AssembleZipFixture $duplicates) }
$bad=@($members);$bad[1]='../outside'
Reject { Read-AssembleEntries (New-AssembleZipFixture $bad) }
$bad=@($members);$bad[1]='unapproved.exe'
Reject { Read-AssembleEntries (New-AssembleZipFixture $bad) }
Reject { Read-AssembleEntries (New-AssembleZipFixture $members $true) }
Reject { Read-AssembleBundle $zip }
Check (@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -match 'Start-Process|Invoke-Expression'},$true)).Count -eq 0)
[Console]::Out.WriteLine(('{"scope":"AST_in_memory_only","checks":'+$checks+',"productExecuted":false,"installerExecuted":false,"mainExecuted":false,"packageAssembled":false}'))
