# AST, allocated metadata and in-memory ZIP fixtures / real package bytes only.
# Never wrapper main, native filesystem, decoder, process, installer or EXE.
param([string]$TransferZip,[string]$SupportZip)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$checks=0
function Check([bool]$Okay) { if (-not $Okay) { throw ('assertion_failed_'+$script:checks) };$script:checks++ }
function Reject([scriptblock]$Body) { $failed=$false;try { &$Body | Out-Null } catch { $failed=$true };Check $failed }
function Read-TestAst([string]$Name) {
    $tokens=$null;$errors=$null
    $ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $Name),[ref]$tokens,[ref]$errors)
    Check ($errors.Count -eq 0)
    $names=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true) | ForEach-Object {$_.Name})
    Check (@($names | Select-Object -Unique).Count -eq $names.Count)
    Check (-not [regex]::IsMatch($ast.Extent.Text,'[^\x00-\x7f]'))
    return $ast
}
$expand=Read-TestAst 'Expand-VpsQa3Delta.ps1'
$assemble=Read-TestAst 'Assemble-VpsQa3Package.ps1'
$functions=@('Get-ExpandHash','Assert-ExpandID','Assert-ExpandStagePair','Get-ExpandComponents','Assert-ExpandPrivateSD','New-ExpandNative','Assert-ExpandKeys','Assert-ExpandInteger','Read-ExpandArchiveEntries','Read-ExpandBundle','Get-AssembleHash','Assert-AssembleID','Get-AssembleComponents','Assert-AssemblePrivateSD','New-AssembleNative','Assert-AssembleKeys','Assert-AssembleInteger','Assert-AssembleName','Read-AssembleManifest','Read-AssembleEntries','Read-AssembleBundle')
foreach ($ast in @($expand,$assemble)) {
    foreach ($definition in @($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))) {
        if ($functions -ccontains $definition.Name) { . ([scriptblock]::Create($definition.Extent.Text)) }
    }
    foreach ($statement in $ast.EndBlock.Statements) {
        if ($statement -isnot [Management.Automation.Language.AssignmentStatementAst] -or $statement.Left -isnot [Management.Automation.Language.VariableExpressionAst]) { continue }
        $name=$statement.Left.VariablePath.UserPath
        if ($name -cmatch '^script:(expand|assemble)(ZipLength|ZipHash|ManifestHash|DecoderHash|Hashes|Lengths)$') { . ([scriptblock]::Create($statement.Extent.Text)) }
    }
}
Check ((Get-ExpandHash ([IO.File]::ReadAllBytes((Join-Path $PSScriptRoot 'Expand-VpsBetaDelta.ps1')))) -ceq 'd141eb20c54afd3f75d1464e53dbb59bfe480b6988235a0faf670257b4010efd')
Check ((Get-ExpandHash ([IO.File]::ReadAllBytes((Join-Path $PSScriptRoot 'Assemble-VpsBetaPackage.ps1')))) -ceq '60be1700086fd8bc9e6b1b3eab1881abac6f224a5dd4a814440521ff043cce4e')
# Critical OS/private/create/timeout mechanics are unchanged from frozen sources.
foreach ($pair in @(@($expand,'Expand-VpsBetaDelta.ps1',@('New-ExpandNative','Get-ExpandAttributes','Lock-ExpandDirectories','Read-ExpandFile','Invoke-ExpandDecoder','Assert-ExpandPrivateSD','Assert-ExpandPrivatePath')), @($assemble,'Assemble-VpsBetaPackage.ps1',@('New-AssembleNative','Get-AssembleAttributes','Lock-AssembleDirectories','New-AssembleParents','Assert-AssemblePrivateSD','Assert-AssemblePrivatePath')))) {
    $oldTokens=$null;$oldErrors=$null
    $old=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $pair[1]),[ref]$oldTokens,[ref]$oldErrors)
    foreach ($name in $pair[2]) {
        $before=@($old.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
        $after=@($pair[0].FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
        Check ($before.Count -eq 1 -and $after.Count -eq 1 -and $before[0].Extent.Text.Replace("`r`n","`n") -ceq $after[0].Extent.Text.Replace("`r`n","`n"))
    }
}
$a='123456789abc4def8abcdef012345678';$b='abcdef0123454abc9abcdef012345678';$leaf='delta-'+('a'*32)
Assert-ExpandStagePair $a $leaf $b ('delta-'+('b'*32));$checks++
Reject { Assert-ExpandStagePair $a $leaf $a ('delta-'+('b'*32)) }
foreach ($bad in @('', '../file','delta-'+('A'*32),'delta-abc','package-beta-qa2')) { Reject { Assert-ExpandStagePair $a $leaf $b $bad } }
foreach ($point in @(0x00b9,0x00b2,0x00b3)) {
    $bad='C:\fixture\COM'+[char]$point+'.txt'
    Reject { Get-ExpandComponents $bad };Reject { Get-AssembleComponents $bad }
    $bad='docs/LPT'+[char]$point+'.txt';Reject { Assert-AssembleName $bad }
}
foreach ($bad in @('\\invalid.example\share','C:relative','C:\fixture\..\file','C:\fixture\file:stream')) { Reject { Get-ExpandComponents $bad };Reject { Get-AssembleComponents $bad } }
foreach ($test in @('Assert-ExpandPrivateSD','Assert-AssemblePrivateSD')) {
    &$test 'O:BAG:BAD:P(A;OICI;FA;;;BA)(A;OICI;FA;;;SY)';$checks++
    Reject { &$test 'O:BUG:BUD:P(A;;FA;;;SY)' };Reject { &$test 'O:BAG:BAD:P(A;;FR;;;BU)' }
}
foreach ($factory in @('New-ExpandNative','New-AssembleNative')) {
    $native=&$factory
    $import=@($native.GetMethod('CreateDirectoryW').GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
    Check ($import.SetLastError -and $import.ExactSpelling -and $native.GetMethod('CreateFileW').ReturnType -eq [IntPtr])
}
Add-Type -AssemblyName System.IO.Compression
function New-TestZip([string[]]$Names,[bool]$Link=$false,[string]$EmptyLeaf='') {
    $memory=[IO.MemoryStream]::new();$zip=[IO.Compression.ZipArchive]::new($memory,[IO.Compression.ZipArchiveMode]::Create,$true)
    try {
        foreach ($name in $Names) {
            $entry=$zip.CreateEntry($name);if ($Link) { $entry.ExternalAttributes=[int](-1610612736) }
            $stream=$entry.Open();try { if ($name -cne $EmptyLeaf) { $data=[Text.Encoding]::ASCII.GetBytes('inert fixture');$stream.Write($data,0,$data.Length) } } finally { $stream.Dispose() }
        }
    } finally { $zip.Dispose() }
    $bytes=$memory.ToArray();$memory.Dispose();return ,$bytes
}
$members=@('manifest.json','Apply-VpsDelta.ps1','godusevpn.exe.delta.gz','godusevpn-cli.exe.delta.gz','godusevpn-svc.exe.delta.gz') | ForEach-Object {'vps-beta-qa3-transfer/'+$_}
Check ((Read-ExpandArchiveEntries (New-TestZip $members)).Count -eq 5)
Reject { Read-ExpandArchiveEntries (New-TestZip ($members+'extra')) }
$bad=@($members);$bad[4]='vps-beta-qa3-transfer/../outside';Reject { Read-ExpandArchiveEntries (New-TestZip $bad) }
$bad=@($members);$bad[1]=$bad[0];Reject { Read-ExpandArchiveEntries (New-TestZip $bad) }
Reject { Read-ExpandArchiveEntries (New-TestZip $members $true) }
Reject { Read-ExpandBundle (New-TestZip $members) }
$nonEXEs=@('ULTRA_PACKAGE.json')+@(0..346 | ForEach-Object {'docs/fixture-'+$_+'.txt'})
$lines=@($nonEXEs | ForEach-Object {('a'*64)+'  '+$_})
foreach ($name in @('godusevpn.exe','godusevpn-cli.exe','godusevpn-svc.exe')) { $lines+=$script:assembleHashes[$name]+'  '+$name }
$manifest=[Text.Encoding]::UTF8.GetBytes(($lines -join "`n")+"`n")
Check ((Read-AssembleManifest $manifest).Count -eq 351)
Reject { Read-AssembleManifest ([Text.Encoding]::UTF8.GetBytes(($lines+$lines[0] -join "`n")+"`n")) }
$members=@('SHA256SUMS')+$nonEXEs
Check ((Read-AssembleEntries (New-TestZip $members)).Count -eq 349)
$emptyFixture=Read-AssembleEntries (New-TestZip $members $false $members[2])
Check ($emptyFixture[$members[2]].Length -eq 0 -and (Get-AssembleHash $emptyFixture[$members[2]]) -ceq 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
foreach ($name in @('../outside','unapproved.exe','docs/CONOUT$.txt')) { $bad=@($members);$bad[1]=$name;Reject { Read-AssembleEntries (New-TestZip $bad) } }
$bad=@($members);$bad[1]=$bad[0];Reject { Read-AssembleEntries (New-TestZip $bad) }
Reject { Read-AssembleEntries (New-TestZip $members $true) }
Reject { Read-AssembleBundle (New-TestZip $members) }
if (-not $TransferZip -or -not $SupportZip) { throw 'explicit_real_zip_paths_required' }
$transferBytes=[IO.File]::ReadAllBytes($TransferZip);$transfer=Read-ExpandBundle $transferBytes
$supportBytes=[IO.File]::ReadAllBytes($SupportZip);$support=Read-AssembleBundle $supportBytes
Check ($transfer.payload.Count -eq 5 -and $transfer.manifest.files.Count -eq 3 -and $support.payload.Count -eq 349 -and $support.hashes.Count -eq 351)
Check (@($support.payload.Keys | Where-Object {$support.payload[$_].Length -eq 0}).Count -eq 2)
foreach ($name in @($support.payload.Keys | Where-Object {$support.payload[$_].Length -eq 0})) {
    Check ((Get-AssembleHash $support.payload[$name]) -ceq $support.hashes[$name] -and $support.hashes[$name] -ceq 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
}
foreach ($file in $transfer.manifest.files) {
    Check ($file.newLength -eq $script:expandLengths[$file.name] -and $file.newLength -eq $script:assembleLengths[$file.name] -and $file.newSha256 -ceq $support.hashes[$file.name])
}
$transferBytes[100]=$transferBytes[100] -bxor 1;Reject { Read-ExpandBundle $transferBytes }
$supportBytes[100]=$supportBytes[100] -bxor 1;Reject { Read-AssembleBundle $supportBytes }
[Console]::Out.WriteLine(('{"scope":"AST_metadata_in_memory_real_ZIP_only","checks":'+$checks+',"transferFiles":5,"supportFiles":349,"manifestPayload":351,"finalFiles":352,"productExecuted":false,"decoderExecuted":false,"mainExecuted":false,"packageAssembled":false}'))
