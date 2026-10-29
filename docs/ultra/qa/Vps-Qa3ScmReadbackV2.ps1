# Fixed read-only SCM compatibility diagnostic. Windows PowerShell 5.1 x64, SYSTEM.
# The OpenSCManagerW database argument is IntPtr so PowerShell cannot coerce NULL
# into an empty string. No service, file, policy, token, registry or network mutation.
[CmdletBinding()]
param()
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'

function New-ScmNative {
    $an=New-Object Reflection.AssemblyName('UltraQa3ScmReadbackV2')
    $ab=[AppDomain]::CurrentDomain.DefineDynamicAssembly($an,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $tb=$ab.DefineDynamicModule('Native').DefineType('UltraQa3ScmReadbackV2Native',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $ctor=[Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields=[Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'),[Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'),[Runtime.InteropServices.DllImportAttribute].GetField('CharSet'),[Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    foreach ($s in @(
        @('advapi32.dll','OpenSCManagerW',[IntPtr],[type[]]@([IntPtr],[IntPtr],[uint32])),
        @('advapi32.dll','OpenServiceW',[IntPtr],[type[]]@([IntPtr],[string],[uint32])),
        @('advapi32.dll','CloseServiceHandle',[bool],[type[]]@([IntPtr]))
    )) {
        $m=$tb.DefineMethod($s[1],[Reflection.MethodAttributes]'Public, Static, PinvokeImpl',$s[2],$s[3])
        $m.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $m.SetCustomAttribute((New-Object Reflection.Emit.CustomAttributeBuilder($ctor,[object[]]@($s[0]),$fields,[object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi))))
    }
    return $tb.CreateType()
}

$r=[ordered]@{schema='ultra-qa3-scm-readback-v2';status='rejected';effectiveSystemCaller=$false;scmOpened=$false;serviceChecks=@();cimQuerySucceeded=$false;originalServices=-1;ultraServices=-1;productExecuted=$false;changesRequested=$false;error=''}
$native=$null;$manager=[IntPtr]::Zero
try {
    $id=[Security.Principal.WindowsIdentity]::GetCurrent()
    try { if (-not $id.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalSystemSid)) { throw 'system_required' };$r.effectiveSystemCaller=$true } finally { $id.Dispose() }
    $native=New-ScmNative
    # IntPtr.Zero maps to a true NULL for the lpDatabase argument.
    $manager=$native::OpenSCManagerW([IntPtr]::Zero,[IntPtr]::Zero,1)
    if ($manager -eq [IntPtr]::Zero) { throw 'SCM_unknown' }
    $r.scmOpened=$true
    foreach($name in @('godusevpn','godusevpn-ultra')) {
        $service=[IntPtr]::Zero
        try {
            $service=$native::OpenServiceW($manager,$name,4)
            $last=[Runtime.InteropServices.Marshal]::GetLastWin32Error()
            if ($service -ne [IntPtr]::Zero) { throw 'service_exists' }
            if ($last -ne 1060) { throw 'SCM_unknown' }
            $r.serviceChecks+=,[ordered]@{name=$name;present=$false;lastError=$last}
        } finally { if ($service -ne [IntPtr]::Zero) { [void]$native::CloseServiceHandle($service) } }
    }
    $r.originalServices=@(Get-CimInstance Win32_Service -Filter "Name='godusevpn'" -Property Name -ErrorAction Stop).Count
    $r.ultraServices=@(Get-CimInstance Win32_Service -Filter "Name='godusevpn-ultra'" -Property Name -ErrorAction Stop).Count
    $r.cimQuerySucceeded=$true;$r.status='completed'
} catch { $r.error=$_.Exception.Message; if ($r.error -notin @('SCM_unknown','service_exists','system_required')) { $r.error='readback_failed' } }
finally { if ($manager -ne [IntPtr]::Zero) { [void]$native::CloseServiceHandle($manager) } }
$json=$r|ConvertTo-Json -Compress -Depth 5
if ($json.Length -gt 2500) { $json='{"schema":"ultra-qa3-scm-readback-v2","status":"rejected","error":"summary_limit","productExecuted":false}'; }
[Console]::Out.WriteLine($json)
