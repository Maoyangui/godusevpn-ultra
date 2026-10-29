# Fixed read-only diagnostic: no service, file, policy or token mutation.
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
function New-InstallNative {
    $an=New-Object Reflection.AssemblyName('UltraQa3InstallPreview')
    $ab=[AppDomain]::CurrentDomain.DefineDynamicAssembly($an,[Reflection.Emit.AssemblyBuilderAccess]::Run)
    $tb=$ab.DefineDynamicModule('Native').DefineType('UltraQa3InstallPreviewNative',[Reflection.TypeAttributes]'Public, Abstract, Sealed')
    $ctor=[Runtime.InteropServices.DllImportAttribute].GetConstructor([type[]]@([string]))
    $fields=[Reflection.FieldInfo[]]@([Runtime.InteropServices.DllImportAttribute].GetField('SetLastError'),[Runtime.InteropServices.DllImportAttribute].GetField('ExactSpelling'),[Runtime.InteropServices.DllImportAttribute].GetField('CharSet'),[Runtime.InteropServices.DllImportAttribute].GetField('CallingConvention'))
    foreach ($s in @(
        @('kernel32.dll','CreateFileW',[IntPtr],[type[]]@([string],[uint32],[uint32],[IntPtr],[uint32],[uint32],[IntPtr])),
        @('kernel32.dll','GetFileInformationByHandleEx',[bool],[type[]]@([IntPtr],[int],[IntPtr],[uint32])),
        @('kernel32.dll','CloseHandle',[bool],[type[]]@([IntPtr])),
        @('kernel32.dll','QueryDosDeviceW',[uint32],[type[]]@([string],[Text.StringBuilder],[uint32])),
        @('advapi32.dll','OpenSCManagerW',[IntPtr],[type[]]@([string],[string],[uint32])),
        @('advapi32.dll','OpenServiceW',[IntPtr],[type[]]@([IntPtr],[string],[uint32])),
        @('advapi32.dll','CloseServiceHandle',[bool],[type[]]@([IntPtr]))
    )) {
        $m=$tb.DefineMethod($s[1],[Reflection.MethodAttributes]'Public, Static, PinvokeImpl',$s[2],$s[3]);$m.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
        $m.SetCustomAttribute((New-Object Reflection.Emit.CustomAttributeBuilder($ctor,[object[]]@($s[0]),$fields,[object[]]@($true,$true,[Runtime.InteropServices.CharSet]::Unicode,[Runtime.InteropServices.CallingConvention]::Winapi))))
    }
    return $tb.CreateType()
}

$r=[ordered]@{schema='ultra-qa3-scm-readback-v1';status='rejected';effectiveSystemCaller=$false;cimQuerySucceeded=$false;originalServices=-1;ultraServices=-1;variants=@();productExecuted=$false;changesRequested=$false;error=''}
$t=$null
try {
 $id=[Security.Principal.WindowsIdentity]::GetCurrent();try { if (-not $id.User.IsWellKnown([Security.Principal.WellKnownSidType]::LocalSystemSid)) { throw 'system_required' };$r.effectiveSystemCaller=$true } finally {$id.Dispose()}
 $t=New-InstallNative
 foreach($mode in @('powershell_null_string','explicit_active_database','reflection_true_null')) {
  $h=[IntPtr]::Zero
  try {
   if($mode -ceq 'powershell_null_string') {$h=$t::OpenSCManagerW($null,$null,1)}
   elseif($mode -ceq 'explicit_active_database') {$h=$t::OpenSCManagerW($null,'ServicesActive',1)}
   else {$h=[IntPtr]$t.GetMethod('OpenSCManagerW').Invoke($null,[object[]]@($null,$null,[uint32]1))}
   $observed=[Runtime.InteropServices.Marshal]::GetLastWin32Error()
   $r.variants+=,[ordered]@{mode=$mode;managerOpened=($h -ne [IntPtr]::Zero);observedLastError=$observed}
  }finally{if($h -ne [IntPtr]::Zero){[void]$t::CloseServiceHandle($h)}}
 }
 $r.originalServices=@(Get-CimInstance Win32_Service -Filter "Name='godusevpn'" -Property Name -ErrorAction Stop).Count
 $r.ultraServices=@(Get-CimInstance Win32_Service -Filter "Name='godusevpn-ultra'" -Property Name -ErrorAction Stop).Count
 $r.cimQuerySucceeded=$true;$r.status='completed'
}catch{$r.error='readback_failed'}
[Console]::Out.WriteLine(($r|ConvertTo-Json -Compress -Depth 5))
