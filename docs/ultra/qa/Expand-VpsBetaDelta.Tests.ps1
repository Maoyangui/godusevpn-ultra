# AST-selected pure functions and in-memory ZIP fixtures only. No main,
# native filesystem call, child process, decoder, file reconstruction or EXE.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Expand-VpsBetaDelta.ps1'),[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'parser_failed' }
$names=@('Get-ExpandHash','Assert-ExpandID','Get-ExpandComponents','Assert-ExpandPrivateSD','New-ExpandNative','Assert-ExpandKeys','Assert-ExpandInteger','Read-ExpandArchiveEntries','Read-ExpandBundle')
foreach ($name in $names) {
    $found=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
    if ($found.Count -ne 1) { throw 'function_not_found' }
    . ([scriptblock]::Create($found[0].Extent.Text))
}
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw 'assertion_failed' };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }
$native=New-ExpandNative
$import=@($native.GetMethod('CreateDirectoryW').GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
Check ($import.SetLastError -and $import.ExactSpelling)
Check ($native.GetMethod('CreateFileW').ReturnType -eq [IntPtr])
Assert-ExpandID '123456789abc4def8abcdef012345678';$checks++
foreach ($bad in @('123456789abc3def8abcdef012345678','123456789abc4def1abcdef012345678','../path','')) { Reject { Assert-ExpandID $bad } }
Check (@(Get-ExpandComponents 'C:\ProgramData\GodUseVPN-Ultra-QA\fixture').Count -eq 4)
foreach ($bad in @('\\invalid.example\share\file','\\?\C:\file','C:\folder\..\file','C:\folder\file:stream','C:\folder\NUL.txt','C:\folder\a.')) { Reject { Get-ExpandComponents $bad } }
Assert-ExpandPrivateSD 'O:SYG:SYD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)';$checks++
foreach ($sddl in @('O:BUG:SYD:P(A;;FA;;;SY)','O:SYG:SYD:P(A;;FR;;;BU)','O:SYG:SYD:NO_ACCESS_CONTROL')) { Reject { Assert-ExpandPrivateSD $sddl } }
Assert-ExpandInteger 256 256 256;$checks++
foreach ($bad in @('256',256.0,-1)) { Reject { Assert-ExpandInteger $bad 256 256 } }
Reject { Assert-ExpandKeys ([pscustomobject]@{safe=1;extra=2}) @('safe') }
Add-Type -AssemblyName System.IO.Compression
function New-ZipFixture([string[]]$Members,[bool]$Link=$false) {
    $memory=[IO.MemoryStream]::new();$zip=[IO.Compression.ZipArchive]::new($memory,[IO.Compression.ZipArchiveMode]::Create,$true)
    try {
        foreach ($name in $Members) {
            $entry=$zip.CreateEntry($name)
            if ($Link) { $entry.ExternalAttributes=[int](-1610612736) }
            $stream=$entry.Open();try { $bytes=[Text.Encoding]::UTF8.GetBytes('inert fixture');$stream.Write($bytes,0,$bytes.Length) } finally { $stream.Dispose() }
        }
    } finally { $zip.Dispose() }
    $bytes=$memory.ToArray();$memory.Dispose();return ,$bytes
}
$prefix='vps-beta-qa2-transfer-v2/'
$members=@('manifest.json','Apply-VpsDelta.ps1','godusevpn.exe.delta.gz','godusevpn-cli.exe.delta.gz','godusevpn-svc.exe.delta.gz') | ForEach-Object { $prefix+$_ }
$zip=New-ZipFixture $members
$payload=Read-ExpandArchiveEntries $zip
Check ($payload.Count -eq 5 -and $payload['manifest.json'].Length -eq 13)
Reject { Read-ExpandArchiveEntries (New-ZipFixture ($members+'extra')) }
Reject { Read-ExpandArchiveEntries (New-ZipFixture @($members[0],$members[0],$members[2],$members[3],$members[4])) }
Reject { Read-ExpandArchiveEntries (New-ZipFixture @($members[0],$members[1],$members[2],$members[3],($prefix+'../outside'))) }
Reject { Read-ExpandArchiveEntries (New-ZipFixture $members $true) }
$script:expandZipHash='0'*64
Reject { Read-ExpandBundle $zip }
Check ((Get-ExpandHash ([Text.Encoding]::ASCII.GetBytes('abc'))) -ceq 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
[Console]::Out.WriteLine(('{"scope":"AST_in_memory_only","checks":'+$checks+',"productExecuted":false,"decoderExecuted":false,"mainExecuted":false}'))
