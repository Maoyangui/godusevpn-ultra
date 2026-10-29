# Cold -File entry regression. Clone only the real read-only entry/preflight;
# replace package/SCM/ACL/platform helpers with strict TEMP stubs, and physically
# remove every statement after ShouldProcess. Never run the real installer.
[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
$tempBase=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$fixture=Join-Path $tempBase ('ultra-entry-qa-'+[Guid]::NewGuid().ToString('N'))
$checks=0
function Check([bool]$Okay,[string]$Name){if(-not $Okay){throw "Entry fixture failed: $Name"};$script:checks++}
function Invoke-Cold([string]$Shell,[string]$Script,[string]$Extra,[string]$Expected){
    $info=New-Object Diagnostics.ProcessStartInfo
    $info.FileName=$Shell
    $info.Arguments='-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$Script+'" -WhatIf '+$Extra
    $info.UseShellExecute=$false;$info.CreateNoWindow=$true
    $info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
    $info.WorkingDirectory=$fixture
    $info.EnvironmentVariables.Remove('PSModulePath')
    $info.EnvironmentVariables['ULTRA_ENTRY_FIXTURE']=$fixture
    $info.EnvironmentVariables['ULTRA_ENTRY_EXPECTED_SOURCE']=$Expected
    $process=New-Object Diagnostics.Process;$process.StartInfo=$info
    try{
        Check ($process.Start()) 'cold_shell_started'
        $output=$process.StandardOutput.ReadToEndAsync();$errorText=$process.StandardError.ReadToEndAsync()
        if(-not $process.WaitForExit(20000)){$process.Kill();throw 'Cold entry fixture timeout.'}
        return [pscustomobject]@{exit=$process.ExitCode;output=$output.GetAwaiter().GetResult();error=$errorText.GetAwaiter().GetResult()}
    }finally{$process.Dispose()}
}
try{
    [IO.Directory]::CreateDirectory($fixture)|Out-Null
    $override=Join-Path $fixture 'explicit-source';[IO.Directory]::CreateDirectory($override)|Out-Null
    $sourcePath=Join-Path $PSScriptRoot 'Install-Ultra.ps1'
    $source=[IO.File]::ReadAllText($sourcePath)
    $tokens=$null;$errors=$null
    $ast=[Management.Automation.Language.Parser]::ParseInput($source,[ref]$tokens,[ref]$errors)
    Check ($errors.Count -eq 0) 'installer_parse'
    $stops=@($ast.EndBlock.Statements|Where-Object{$_.Extent.Text.StartsWith('if (-not $PSCmdlet.ShouldProcess(')})
    Check ($stops.Count -eq 1) 'single_main_shouldprocess_boundary'
    $end=$stops[0].Extent.EndOffset
    $stubbed=$source.Substring(0,$end)+"`r`nthrow 'MUTATION_GUARD: ShouldProcess unexpectedly allowed a write.'`r`n"
    $stubs=@{
        'Assert-Platform'=@'
function Assert-Platform { }
'@
        'Test-Administrator'=@'
function Test-Administrator { return $false }
'@
        'Test-Package'=@'
function Test-Package([string]$Root) {
    if ([string]::IsNullOrWhiteSpace($Root)) { throw 'EMPTY_SOURCE_REACHED_PACKAGE' }
    $full=[IO.Path]::GetFullPath($Root)
    if (-not [string]::Equals($full,$env:ULTRA_ENTRY_EXPECTED_SOURCE,[StringComparison]::OrdinalIgnoreCase)) { throw 'UNEXPECTED_SOURCE_PATH' }
    [Console]::Out.WriteLine('ENTRY_SOURCE_ACCEPTED')
    return [pscustomobject]@{Root=$full;Hashes=@{fixture='hash'};Descriptor=[pscustomobject]@{version='fixture'}}
}
'@
        'Get-FixedTarget'=@'
function Get-FixedTarget { return [IO.Path]::Combine($env:ULTRA_ENTRY_FIXTURE,'never-created-installation') }
'@
        'Assert-NoReparse'=@'
function Assert-NoReparse([string]$Path) {
    if (-not [string]::Equals([IO.Path]::GetFullPath($Path),$env:ULTRA_ENTRY_FIXTURE,[StringComparison]::OrdinalIgnoreCase)) { throw 'NON_TEMP_SYSTEM_PATH' }
}
'@
        'Get-UltraService'=@'
function Get-UltraService { [Console]::Out.WriteLine('STUB_SCM_READ_ONLY'); return $null }
'@
        'Set-ProgramACL'="function Set-ProgramACL { throw 'FORBIDDEN_ACL' }"
        'Set-ProgramTreeACL'="function Set-ProgramTreeACL { throw 'FORBIDDEN_ACL' }"
        'Assert-ProtectedCodeDirectory'="function Assert-ProtectedCodeDirectory { throw 'FORBIDDEN_SYSTEM_PATH' }"
    }
    # Replace backwards, so original offsets stay valid. No actual helper with
    # CIM, native identity, ACL write, or privileged code execution remains live.
    $definitions=@($ast.FindAll({param($n)$n -is [Management.Automation.Language.FunctionDefinitionAst] -and $stubs.ContainsKey($n.Name)},$true)|Sort-Object {$_.Extent.StartOffset} -Descending)
    Check ($definitions.Count -eq $stubs.Count) 'all_system_stubs_found'
    foreach($definition in $definitions){$start=$definition.Extent.StartOffset;$length=$definition.Extent.EndOffset-$start;$stubbed=$stubbed.Remove($start,$length).Insert($start,$stubs[$definition.Name])}
    Check (-not $stubbed.Contains("& `$svcExe 'install'")) 'product_install_statement_removed'
    $entry=Join-Path $fixture 'Install Entry.ps1'
    [IO.File]::WriteAllText($entry,$stubbed,(New-Object Text.UTF8Encoding($false)))
    $pwsh=(Get-Command pwsh -ErrorAction Stop).Source
    $shells=@([IO.Path]::Combine($env:WINDIR,'System32/WindowsPowerShell/v1.0/powershell.exe'),$pwsh)
    foreach($shell in $shells){
        Check ([IO.File]::Exists($shell)) 'shell_exists'
        $default=Invoke-Cold $shell $entry '' $fixture
        Check ($default.exit -eq 0 -and $default.output.Contains('ENTRY_SOURCE_ACCEPTED') -and $default.output.Contains('STUB_SCM_READ_ONLY') -and -not $default.error) 'default_source_cold_file_whatif'
        $explicit=Invoke-Cold $shell $entry ('-Source "'+$override+'"') $override
        Check ($explicit.exit -eq 0 -and $explicit.output.Contains('ENTRY_SOURCE_ACCEPTED') -and -not $explicit.error) 'explicit_source_cold_file_whatif'
        foreach($bad in @('-Source ""','-Source "  "')){
            $refused=Invoke-Cold $shell $entry $bad ''
            Check ($refused.exit -ne 0 -and $refused.error.Contains('Source must be a nonempty local package directory') -and -not $refused.output.Contains('ENTRY_SOURCE_ACCEPTED')) 'explicit_empty_source_refused_before_preflight'
        }
        Check (-not [IO.Directory]::Exists((Join-Path $fixture 'never-created-installation'))) 'no_directory_installed'
    }
    # The uninstaller uses a literal default; PSScriptRoot is read in its body.
    $tokens=$null;$errors=$null
    $uninstall=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Uninstall-Ultra.ps1'),[ref]$tokens,[ref]$errors)
    Check ($errors.Count -eq 0) 'uninstaller_parse'
    foreach($parameter in $uninstall.ParamBlock.Parameters){Check ($null -eq $parameter.DefaultValue -or -not $parameter.DefaultValue.Extent.Text.Contains('$PSScriptRoot')) 'uninstaller_no_early_scriptroot_default'}
    [pscustomobject]@{status='passed';checks=$checks;psVersion=$PSVersionTable.PSVersion.ToString();scope='cold WindowsPowerShell5.1 and PS7 -File -WhatIf, exact real entry/preflight with TEMP stubs; mutating tail removed';realInstallerExecuted=$false;serviceChanged=$false}|ConvertTo-Json -Compress
}finally{
    $resolved=[IO.Path]::GetFullPath($fixture)
    if(-not $resolved.StartsWith($tempBase+'\',[StringComparison]::OrdinalIgnoreCase)-or [IO.Path]::GetFileName($resolved)-notmatch '^ultra-entry-qa-[a-f0-9]{32}$'){throw 'Unsafe entry fixture cleanup target.'}
    if([IO.Directory]::Exists($resolved)){[IO.Directory]::Delete($resolved,$true)}
}
