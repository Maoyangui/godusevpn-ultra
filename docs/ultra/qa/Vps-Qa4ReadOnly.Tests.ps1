# Only parses trusted QA source and evaluates an explicit pure-function list.
# No product, process runner, native filesystem call, settings or network access.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
$source=Join-Path $PSScriptRoot 'Vps-Qa4ReadOnly.ps1'
$tokens=$null;$parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($source,[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count -ne 0) { throw 'beta_script_parser_failed' }
$names=@('New-BetaNative','Get-BetaComponents','Assert-BetaPrivateSD','Get-BetaPlan','Get-BetaField','Get-BetaBool','Get-BetaCount','Assert-BetaUserScope','Measure-BetaPolicy','Measure-BetaDetection','Measure-BetaFirewall','Confirm-BetaVersion')
$functions=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst]},$true))
foreach ($name in $names) {
    $found=@($functions | Where-Object { $_.Name -ceq $name })
    if ($found.Count -ne 1) { throw 'pure_function_not_found' }
    Invoke-Expression $found[0].Extent.Text
}
function Assert-Test([bool]$Condition,[string]$Code) { if (-not $Condition) { throw $Code } }
function Assert-Reject([scriptblock]$Body) { $rejected=$false;try { &$Body | Out-Null } catch { $rejected=$true };Assert-Test $rejected 'expected_rejection' }

# The qa.4 wrapper is independently reviewed against the qa.3 shape. Keep
# this check focused on stale identity/hash rejection and ASCII-only source.
$sourceText=$ast.Extent.Text.Replace("`r`n","`n")
Assert-Test (-not [regex]::IsMatch($sourceText,'qa\.3|Qa3|QA3|0\.01-beta-qa\.3|3a17693|2ebd104|2293b29') -and $sourceText.Contains("0.01-beta-qa.4") -and $sourceText.Contains('53107fddbbbea4b11030d1aaede6beae6839b27ea7b4412d0e6fadbd9d4044be') -and $sourceText.Contains('b2ede7f86ebe73b4d8e5b3010507357b6f9eb27af4585f2e0f760189cad083f8') -and $sourceText.Contains('c321a62437521c6c8bfcb5b290c78396dd5fb9b7a2464c2c626d064794b8bafc') -and -not [regex]::IsMatch($sourceText,'[^\x00-\x7f]')) 'fixed_qa4_copy_only'
Assert-Test (@($functions | Group-Object Name | Where-Object {$_.Count -ne 1}).Count -eq 0) 'function_definitions_unique'

# Factory emits metadata in memory. Inspect marshaling flags without invoking
# any native method or touching a filesystem, token, registry or product.
$native=New-BetaNative
$import=@($native.GetMethod('CreateFileW').GetCustomAttributes([Runtime.InteropServices.DllImportAttribute],$false))[0]
Assert-Test ($import.SetLastError -and $import.ExactSpelling) 'native_error_metadata'
Assert-Test ($native.GetMethod('CreateFileW').ReturnType -eq [IntPtr]) 'native_handle_width'
Assert-Test (@($native.GetMethods() | Where-Object { $_.Name -eq 'GetFileInformationByHandleEx' }).Count -eq 1) 'native_info_signature'
$plan=@(Get-BetaPlan)
Assert-Test ($plan.Count -eq 5) 'fixed_plan_count'
Assert-Test ((@($plan | ForEach-Object {$_.arguments}) -join '|') -ceq 'version|privacy report|privacy detection|privacy firewall-list|version') 'fixed_plan_arguments'
Assert-Test ((@($plan | ForEach-Object {$_.role}) -join '|') -ceq 'cli|cli|cli|cli|svc') 'fixed_plan_roles'
Assert-Test ((Confirm-BetaVersion "GodUseVPN Ultra 0.01-beta-qa.4`r`n") -ceq '0.01-beta-qa.4') 'safe_version'
Assert-Reject { Confirm-BetaVersion 'GodUseVPN Ultra 0.01-beta-qa.2' }
foreach ($text in @('GodUseVPN Ultra other','GodUseVPN Ultra 0.01-beta-qa.4 extra',"GodUseVPN Ultra 0.01-beta-qa.4`nextra")) { Assert-Reject { Confirm-BetaVersion $text } }
Assert-Test (@(Get-BetaComponents 'C:\ProgramData\GodUseVPN-Ultra-QA\fixture').Count -eq 4) 'path_components'
foreach ($bad in @('\\invalid.example\share\file','\\?\C:\file','C:\folder\..\file','C:\folder\file:stream','C:\folder\NUL.txt','C:\folder\a.','C:\folder\bad*')) { Assert-Reject { Get-BetaComponents $bad } }
foreach ($point in @(0xb9,0xb2,0xb3)) { $bad='C:\folder\COM'+[char]$point+'.txt';Assert-Reject { Get-BetaComponents $bad } }
Assert-BetaPrivateSD 'O:SYG:SYD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)'
Assert-BetaPrivateSD 'O:BAG:BAD:(A;ID;FA;;;SY)(A;ID;FA;;;BA)'
Assert-Reject { Assert-BetaPrivateSD 'O:SYG:SYD:P(A;;FR;;;BU)' }
Assert-Reject { Assert-BetaPrivateSD 'O:BUG:SYD:P(A;;FA;;;SY)' }
Assert-Reject { Assert-BetaPrivateSD 'O:SYG:SYD:NO_ACCESS_CONTROL' }

$policy='{"platform":"windows","observerElevated":true,"backupHealthy":true,"backupScope":"current_process_user","managedScope":"current_process_user_journal","machineJournalChecked":false,"machineJournalStatus":"not_inspected","controls":[{"managed":false,"canApply":true,"status":"available","detail":"MUST_NOT_ESCAPE"},{"managed":true,"canApply":false,"status":"unknown","detail":"MUST_NOT_ESCAPE"}],"warnings":["MUST_NOT_ESCAPE"]}' | ConvertFrom-Json
$detection='{"platform":"windows","executionMode":"direct_cli","admin":true,"helperAuthorized":false,"backupScope":"current_process_user","managedScope":"current_process_user_journal","machineJournalChecked":false,"machineJournalStatus":"not_inspected","findings":[{"status":"observed","summary":"MUST_NOT_ESCAPE"},{"status":"unknown"},{"status":"future_MUST_NOT_ESCAPE"}],"observer":"MUST_NOT_ESCAPE"}' | ConvertFrom-Json
$firewall='{"supported":true,"elevated":true,"unknownFilters":2,"rules":[{"state":"filters_installed","rule":{"program":"MUST_NOT_ESCAPE"}},{"state":"partial"},{"state":"future_MUST_NOT_ESCAPE"}],"warnings":["MUST_NOT_ESCAPE"]}' | ConvertFrom-Json
$summary=[ordered]@{policy=(Measure-BetaPolicy $policy);detection=(Measure-BetaDetection $detection);firewall=(Measure-BetaFirewall $firewall)}
Assert-Test ($summary.policy.controls -eq 2 -and $summary.policy.managed -eq 1 -and $summary.policy.canApply -eq 1 -and $summary.policy.unknownOrOther -eq 1) 'policy_counts'
Assert-Test ($summary.detection.findings -eq 3 -and $summary.detection.statuses.observed -eq 1 -and $summary.detection.statuses.unknown -eq 1 -and $summary.detection.statuses.other -eq 1) 'detection_unknown_preserved'
Assert-Test ($summary.firewall.rules -eq 3 -and $summary.firewall.unknownFilters -eq 2 -and $summary.firewall.filtersInstalled -eq 1 -and $summary.firewall.partial -eq 1 -and $summary.firewall.other -eq 1) 'firewall_counts'
$text=$summary | ConvertTo-Json -Compress -Depth 8
Assert-Test ($text.Length -lt 3000 -and -not $text.Contains('MUST_NOT_ESCAPE')) 'safe_summary_only'
$empty='{"supported":true,"elevated":true,"unknownFilters":0,"rules":[]}' | ConvertFrom-Json
Assert-Test ((Measure-BetaFirewall $empty).rules -eq 0) 'empty_array_preserved'
Assert-Reject { Measure-BetaFirewall ('{"supported":true,"elevated":true,"unknownFilters":0,"rules":null}' | ConvertFrom-Json) }
Assert-Reject { Get-BetaCount -1 }
Assert-Reject { Get-BetaBool ([pscustomobject]@{value='true'}) 'value' }
$detection.helperAuthorized=$true
Assert-Reject { Measure-BetaDetection $detection }
$policy.machineJournalChecked=$true
Assert-Reject { Measure-BetaPolicy $policy }

# A pure fake runner consumes the fixed plan only. It does not load or invoke
# Invoke-BetaProcess, the main body, a product, or any supplied executable path.
$fakeCalls=0
foreach ($step in $plan) { $fakeCalls++;Assert-Test ($step.role -cne 'gui' -and $step.arguments -notmatch 'apply|restore|install|connect|start') 'no_mutation_in_plan' }
Assert-Test ($fakeCalls -eq 5) 'fake_plan_count'
[Console]::Out.WriteLine('{"scope":"AST_and_synthetic_only","parser":true,"nativeMetadata":true,"fixedCommands":5,"aggregates":true,"rawFieldsExcluded":true,"productExecuted":false}')
