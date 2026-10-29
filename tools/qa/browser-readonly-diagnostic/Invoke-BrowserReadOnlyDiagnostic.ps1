param(
    [Parameter(Mandatory=$true)][ValidateRange(1,2147483647)][uint32]$ParentPid,
    [Parameter(Mandatory=$true)][ValidateRange(1,2147483647)][uint32]$HelperPid
)
$ErrorActionPreference = 'Stop'
$stage = 'C:\GodUseVPN-Ultra-Qa11-BrowserReadOnlyDiag'
$exe = Join-Path $stage 'BrowserReadOnlyDiagnostic.exe'
$expectedToolSha = 'f19d14f9a2ac6059e9123f2949c2c36a4e770297268f586138570ed4401a4e6e'
$expectedGuiSha = 'd93205d060ecd86d651d72a4ac63aedbb9572d83bc875c3edafa201bc5afae05'
if ($ParentPid -eq $HelperPid) { throw 'Distinct GUI/helper IDs are required.' }
if ([IO.Path]::GetFullPath($PSCommandPath) -ine (Join-Path $stage 'Invoke-BrowserReadOnlyDiagnostic.ps1')) { throw 'The launcher must be independently staged at the fixed approved location.' }

# This script never creates the stage or changes security. A SYSTEM operator
# must independently approve/stage/pin these exact two files before this runs.
$trustedOwners = @('S-1-5-18','S-1-5-32-544')
$writeMask = [Security.AccessControl.FileSystemRights]::Write -bor
    [Security.AccessControl.FileSystemRights]::Delete -bor
    [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor
    [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
    [Security.AccessControl.FileSystemRights]::TakeOwnership
foreach ($path in @($stage,$exe,$PSCommandPath)) {
    $item = Get-Item -LiteralPath $path -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Reparse objects are refused.' }
    $acl = Get-Acl -LiteralPath $path
    $owner = $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value
    if ($trustedOwners -notcontains $owner) { throw 'The approved stage must have a system/administrator owner.' }
    if ($path -eq $stage -and -not $acl.AreAccessRulesProtected) { throw 'The approved stage DACL must be protected.' }
    $rules = @($acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]))
    if ($rules.Count -eq 0) { throw 'An empty or unconfirmed DACL is refused.' }
    foreach ($rule in $rules) {
        if ($rule.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and
            $trustedOwners -notcontains $rule.IdentityReference.Value -and
            ($rule.FileSystemRights -band $writeMask) -ne 0) { throw 'Ordinary-user writable stage objects are refused.' }
    }
}
$leaves = @(Get-ChildItem -LiteralPath $stage -Force)
if ($leaves.Count -ne 2 -or @($leaves | Where-Object { $_.PSIsContainer -or $_.Name -notin @('BrowserReadOnlyDiagnostic.exe','Invoke-BrowserReadOnlyDiagnostic.ps1') }).Count -ne 0) { throw 'Only the approved tool and launcher may be present; config/DLL files are refused.' }
$file = Get-Item -LiteralPath $exe
if ($file.Length -ne 18944 -or (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expectedToolSha) { throw 'The fixed diagnostic bytes do not match.' }

# Explicit, interactive UAC only. No user path, flags, shell text, credential,
# environment or output filename is passed to the diagnostic.
$process = Start-Process -FilePath $exe -Verb RunAs -WorkingDirectory 'C:\Windows\System32' -ArgumentList @(
    $ParentPid.ToString([Globalization.CultureInfo]::InvariantCulture),
    $HelperPid.ToString([Globalization.CultureInfo]::InvariantCulture),
    $expectedGuiSha
) -Wait -PassThru
[pscustomobject]@{ toolExitCode = $process.ExitCode }
