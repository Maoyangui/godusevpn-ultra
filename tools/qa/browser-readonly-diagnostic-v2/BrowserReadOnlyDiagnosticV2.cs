// VM-only QA11 diagnostic v2. No process, token, ACL or WFP mutation.
using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;
using System.Web.Script.Serialization;
using Microsoft.Win32.SafeHandles;

[assembly: DefaultDllImportSearchPaths(DllImportSearchPath.System32)]

internal static class BrowserReadOnlyDiagnostic
{
    internal const string GuiHash = "d93205d060ecd86d651d72a4ac63aedbb9572d83bc875c3edafa201bc5afae05";
    const uint Query = 0x1000, Sync = 0x100000, Dup = 0x40, TokenQuery = 8, TokenImpersonate = 4;
    const uint DontVerify = 0x4000;
    static readonly Guid ProgramFiles = new Guid("905e63b6-c1bf-494e-b29c-65b732d3d21a");
    static readonly Guid LocalAppData = new Guid("f1b32785-6fba-4fcf-9d55-7b8e7f157091");
    static readonly Guid Provider = new Guid("6e6f48e6-ae4c-4f26-bb9f-e90fda8d13b1");
    static readonly Guid[] Layers = {
        new Guid("c38d57d1-05a7-4c33-904f-7fbceee60e82"), new Guid("4a72393b-319f-44bc-84c3-ba54dcb3b6b4"),
        new Guid("e1cd9fe7-f4b5-4273-96c0-592e487b8650"), new Guid("a3b42c97-9f04-4672-b87e-cee9c483257f") };
    static readonly Dictionary<string, object> Result = new Dictionary<string, object>();
    static readonly List<IntPtr> Held = new List<IntPtr>();
    static uint Error { get { return unchecked((uint)Marshal.GetLastWin32Error()); } }
    static void Put(string key, object value) { Result[key] = value; }
    static void Require(bool value, uint stage) { if (!value) throw new Refused(stage); }
    sealed class Refused : Exception { internal readonly uint Stage; internal Refused(uint stage) { Stage = stage; } }

    public static int Main(string[] args)
    {
        Put("schemaVersion", 2); Put("readOnly", true); Put("identityVerified", false); Put("completed", false);
        Put("wfpKnownLayersOnly", true); Put("wfpCountComplete", false);
        int exit = 1;
        try { Run(args); exit = 0; Put("completed", true); }
        catch (Refused e) { Put("refusedStage", e.Stage); }
        catch (Exception e) { Put("unexpectedCode", unchecked((uint)e.HResult)); }
        finally { for (int i = Held.Count - 1; i >= 0; --i) CloseHandle(Held[i]); }
        WriteSanitizedReceipt();
        Console.WriteLine(new JavaScriptSerializer().Serialize(Result));
        // ShellExecute/RunAs opens a separate console. Keep the fixed, sanitized
        // report visible; no file/logger or secret command line is produced.
        if (!Console.IsInputRedirected && !Console.IsOutputRedirected) { Console.WriteLine("Press Enter to close this read-only report."); Console.ReadLine(); }
        return exit;
    }

    internal static bool ValidArgs(string[] args, out uint parent, out uint helper)
    {
        parent = helper = 0;
        return args != null && args.Length == 3 && args[2] == GuiHash &&
            UInt32.TryParse(args[0], NumberStyles.None, CultureInfo.InvariantCulture, out parent) &&
            UInt32.TryParse(args[1], NumberStyles.None, CultureInfo.InvariantCulture, out helper) &&
            parent > 0 && helper > 0 && parent != helper && parent.ToString(CultureInfo.InvariantCulture) == args[0] && helper.ToString(CultureInfo.InvariantCulture) == args[1];
    }
    static void Run(string[] args)
    {
        uint parentId, helperId;
        Require(Environment.Is64BitProcess && ValidArgs(args, out parentId, out helperId), 1);
        // Definite assignment after the short-circuit architecture guard.
        Require(ValidArgs(args, out parentId, out helperId), 1);
        string basePath; uint hr = KnownFolder(ProgramFiles, IntPtr.Zero, out basePath);
        Put("programFilesCode", hr); Require(hr == 0 && CanonicalLocal(basePath), 2);
        string expected = Path.Combine(basePath, "GodUseVPN Ultra", "godusevpn.exe");
        Require(CanonicalLocal(expected), 2); PinInstalledFile(expected);
        // Preserve both real token privilege states even if opening the GUI's
        // baseline process handle is the first cross-account access failure.
        IntPtr selfToken = Token(GetCurrentProcess(), TokenQuery, "toolTokenQueryCode"); Require(selfToken != IntPtr.Zero, 8);
        Privilege(selfToken, "tool");
        IntPtr helper = Open(helperId, Query | Sync, "helperQueryOpenCode"); Require(helper != IntPtr.Zero, 3);
        Put("helperImageMatches", ImageMatches(helper, expected)); Require((bool)Result["helperImageMatches"] && Alive(helper), 5);
        IntPtr helperToken = Token(helper, TokenQuery, "helperTokenQueryCode"); Require(helperToken != IntPtr.Zero, 8);
        string selfSid = Sid(selfToken), helperSid = Sid(helperToken);
        bool sameAccount = selfSid == helperSid && !SystemSid(selfSid);
        Put("sameHelperAccount", sameAccount);
        bool selfHigh = Elevated(selfToken) && Integrity(selfToken) == 0x3000;
        bool helperHigh = Elevated(helperToken) && Integrity(helperToken) == 0x3000;
        Put("toolElevatedHigh", selfHigh); Put("helperElevatedHigh", helperHigh);
        Put("sameHelperAuthenticationId", Authentication(selfToken) == Authentication(helperToken));
        Privilege(helperToken, "helper");
        Require(sameAccount && selfHigh && helperHigh, 9);
        IntPtr parent = Open(parentId, Query | Sync, "parentQueryOpenCode");
        IntPtr parentDup = Open(parentId, Query | Sync | Dup, "parentQueryDupOpenCode");
        Require(parent != IntPtr.Zero, 3);
        ulong parentCreated = Created(parent, "parentTimesCode"), helperCreated = Created(helper, "helperTimesCode");
        Require(parentCreated > 0 && helperCreated > parentCreated && Alive(parent) && Alive(helper), 4);
        Put("guiImageMatches", ImageMatches(parent, expected));
        Require((bool)Result["guiImageMatches"] && (bool)Result["helperImageMatches"], 5);
        uint parentSession = 0, helperSession = 0, ownSession = 0;
        bool sessions = ProcessIdToSessionId(parentId, out parentSession) && ProcessIdToSessionId(helperId, out helperSession) && ProcessIdToSessionId(GetCurrentProcessId(), out ownSession);
        Put("sessionQueryCode", sessions ? 0u : Error);
        Require(sessions && parentSession != 0 && parentSession == helperSession && parentSession == ownSession, 6);
        Put("sessionsMatch", true);
        ProcessBasic basic; uint length;
        int nt = NtQueryInformationProcess(helper, 0, out basic, 48, out length);
        Put("helperParentQueryNtStatus", unchecked((uint)nt));
        bool related = nt == 0 && length == 48 && basic.UniqueProcessId.ToUInt64() == helperId && basic.InheritedFromUniqueProcessId.ToUInt64() == parentId;
        Put("helperParentMatchesGui", related); Require(related, 7);

        // Run each requested access probe independently. A denied token query is
        // itself useful evidence; do not enable SeDebug or substitute a token.
        if (parentDup != IntPtr.Zero) {
            bool duplicateMatches = Created(parentDup, "parentDupTimesCode") == parentCreated && Alive(parentDup);
            Put("parentDupIdentityMatches", duplicateMatches); Require(duplicateMatches, 4);
        }
        IntPtr tokenQuery = Token(parent, TokenQuery, "parentTokenQueryCode");
        IntPtr tokenImpersonate = Token(parent, TokenQuery | TokenImpersonate, "parentTokenQueryImpersonateCode");
        IntPtr dupQuery = parentDup == IntPtr.Zero ? IntPtr.Zero : Token(parentDup, TokenQuery, "dupParentTokenQueryCode");
        IntPtr dupImpersonate = parentDup == IntPtr.Zero ? IntPtr.Zero : Token(parentDup, TokenQuery | TokenImpersonate, "dupParentTokenQueryImpersonateCode");
        Require(tokenQuery != IntPtr.Zero, 10);
        string guiSid = Sid(tokenQuery);
        bool ordinary = !SystemSid(guiSid) && !Elevated(tokenQuery) && Integrity(tokenQuery) == 0x2000;
        Put("guiAndHelperDifferentAccounts", guiSid != helperSid);
        Put("guiOrdinaryMedium", ordinary); Require(ordinary, 11);
        Put("identityVerified", true);
        FolderProbe(tokenQuery, "queryToken"); FolderProbe(tokenImpersonate, "impersonateToken");
        FolderProbe(dupQuery, "dupQueryToken"); FolderProbe(dupImpersonate, "dupImpersonateToken");
        CountFilters();
        bool stable = Created(parent, "parentFinalTimesCode") == parentCreated && Created(helper, "helperFinalTimesCode") == helperCreated && Alive(parent) && Alive(helper);
        Put("processesStableAndAlive", stable); Require(stable, 12);
    }

    internal static bool CanonicalLocal(string value)
    {
        if (String.IsNullOrEmpty(value) || value.Length < 4 || value.Length > 30000 || !Char.IsLetter(value[0]) || value[1] != ':' || value[2] != '\\' || Path.GetFullPath(value) != value || value.IndexOf('/') >= 0) return false;
        foreach (string part in value.Substring(3).Split('\\')) {
            if (part.Length == 0 || part == "." || part == ".." || part.EndsWith(".", StringComparison.Ordinal) || part.EndsWith(" ", StringComparison.Ordinal) || part.IndexOfAny(new char[] { ':', '*', '?', '"', '<', '>', '|' }) >= 0) return false;
            foreach (char c in part) if (c < 32) return false;
            string stem = part.Split('.')[0].ToUpperInvariant();
            if (stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CONIN$" || stem == "CONOUT$" || (stem.Length == 4 && (stem.StartsWith("COM", StringComparison.Ordinal) || stem.StartsWith("LPT", StringComparison.Ordinal)) && "123456789¹²³".IndexOf(stem[3]) >= 0)) return false;
        }
        return true;
    }
    static void PinInstalledFile(string path)
    {
        string drive = path.Substring(0, 2); StringBuilder mapping = new StringBuilder(32768);
        Require(QueryDosDevice(drive, mapping, mapping.Capacity) != 0 && System.Text.RegularExpressions.Regex.IsMatch(mapping.ToString(), @"^\\Device\\HarddiskVolume[0-9]+$") && GetDriveType(drive + "\\") == 3, 20);
        string current = drive + "\\";
        string[] pieces = path.Substring(3).Split('\\');
        for (int i = -1; i < pieces.Length; ++i) {
            if (i >= 0) current = Path.Combine(current, pieces[i]);
            bool directory = i < pieces.Length - 1;
            IntPtr h = CreateFile(current, directory ? 0x81u : 0x80000000u, 1, IntPtr.Zero, 3, 0x00200000u | (directory ? 0x02000000u : 0), IntPtr.Zero);
            Put("installedOpenCode", h == new IntPtr(-1) ? Error : 0u); Require(h != new IntPtr(-1), 21); Held.Add(h);
            ByHandle info; Require(GetFileInformationByHandle(h, out info) && (info.Attributes & 0x400) == 0 && ((info.Attributes & 0x10) != 0) == directory && (directory || info.Links == 1), 22);
            if (!directory) {
                StringBuilder final = new StringBuilder(32768); uint n = GetFinalPathNameByHandle(h, final, final.Capacity, 0);
                Require(n > 0 && n < final.Capacity && String.Equals(final.ToString(), @"\\?\" + path, StringComparison.OrdinalIgnoreCase), 23);
                using (SafeFileHandle safe = new SafeFileHandle(h, false))
                using (FileStream stream = new FileStream(safe, FileAccess.Read))
                using (SHA256 sha = SHA256.Create()) {
                    string actual = BitConverter.ToString(sha.ComputeHash(stream)).Replace("-", "").ToLowerInvariant();
                    Put("installedHashMatches", actual == GuiHash); Require(actual == GuiHash, 24);
                }
            }
        }
    }
    static IntPtr Open(uint pid, uint access, string field) { IntPtr h = OpenProcess(access, false, pid); Put(field, h == IntPtr.Zero ? Error : 0u); if (h != IntPtr.Zero) Held.Add(h); return h; }
    static IntPtr Token(IntPtr process, uint access, string field) { IntPtr h; bool ok = OpenProcessToken(process, access, out h); Put(field, ok ? 0u : Error); if (ok) Held.Add(h); return ok ? h : IntPtr.Zero; }
    static ulong Created(IntPtr process, string field) { FileTime creation, exit, kernel, user; bool ok = GetProcessTimes(process, out creation, out exit, out kernel, out user); Put(field, ok ? 0u : Error); return ok ? ((ulong)creation.High << 32 | creation.Low) : 0; }
    static bool Alive(IntPtr process) { return WaitForSingleObject(process, 0) == 258; }
    static bool ImageMatches(IntPtr process, string expected) { StringBuilder path = new StringBuilder(32768); int count = path.Capacity; return QueryFullProcessImageName(process, 0, path, ref count) && count > 0 && count < path.Capacity && String.Equals(path.ToString(), expected, StringComparison.OrdinalIgnoreCase); }
    static IntPtr Info(IntPtr token, int kind)
    {
        uint size = FixedInfoSize(kind), required = 0;
        Put("tokenInfoClass", kind); Put("tokenInfoUsesFixedSize", size != 0);
        Put("tokenInfoProbeAttempted", false); Put("tokenInfoProbeCode", 0u);
        if (size == 0) {
            // Variable classes use a bounded real buffer too, avoiding a
            // class-specific NULL/zero-buffer status contract altogether.
            Require(MinimumInfoSize(kind) != 0, 30); size = 4096;
        }
        for (int attempt = 0; attempt < 2; ++attempt) {
            IntPtr data = Marshal.AllocHGlobal(checked((int)size));
            bool ok = GetTokenInformation(token, kind, data, size, out required);
            uint code = ok ? 0u : Error;
            Put("tokenInfoCode", code); Put("tokenInfoRequired", required); Put("tokenInfoBufferSize", size);
            if (ok && ValidInfoReturn(kind, size, required)) return data;
            Marshal.FreeHGlobal(data);
            // A variable token can grow between sizing and data calls. One
            // bounded retry is allowed, never an acceptance of failed data.
            if (!ok && attempt == 0 && FixedInfoSize(kind) == 0 && required > size && ValidInfoProbe(kind, code, required)) { size = required; continue; }
            throw new Refused(30);
        }
        throw new Refused(30);
    }
    internal static uint FixedInfoSize(int kind) { return kind == 20 ? 4u : kind == 10 ? 56u : 0u; }
    internal static uint MinimumInfoSize(int kind) { return kind == 1 || kind == 25 ? 16u : kind == 3 ? 4u : FixedInfoSize(kind); }
    internal static bool ValidInfoProbe(int kind, uint code, uint required) { uint min = MinimumInfoSize(kind); return FixedInfoSize(kind) == 0 && min != 0 && (code == 24 || code == 122) && required >= min && required <= 1 << 20; }
    internal static bool ValidInfoReturn(int kind, uint allocated, uint returned) { uint fixedSize = FixedInfoSize(kind), min = MinimumInfoSize(kind); return fixedSize != 0 ? allocated == fixedSize && returned == fixedSize : min != 0 && returned >= min && returned <= allocated && allocated <= 1 << 20; }

    static void WriteSanitizedReceipt()
    {
        const string stage = @"C:\GodUseVPN-Ultra-Qa11-BrowserReadOnlyDiagV2";
        Put("receiptWritten", false); Put("receiptWriteCode", 0u);
        List<IntPtr> pins = new List<IntPtr>();
        try {
            Require(String.Equals(AppDomain.CurrentDomain.BaseDirectory.TrimEnd('\\'), stage, StringComparison.OrdinalIgnoreCase), 50);
            StringBuilder mapping = new StringBuilder(32768);
            Require(QueryDosDevice("C:", mapping, mapping.Capacity) != 0 && System.Text.RegularExpressions.Regex.IsMatch(mapping.ToString(), @"^\\Device\\HarddiskVolume[0-9]+$") && GetDriveType(@"C:\") == 3, 50);
            foreach (string component in new string[] { @"C:\", stage }) {
                IntPtr h = CreateFile(component, 0x20081u, 1, IntPtr.Zero, 3, 0x02200000u, IntPtr.Zero);
                if (h == new IntPtr(-1)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
                pins.Add(h); ByHandle info;
                Require(GetFileInformationByHandle(h, out info) && (info.Attributes & 0x410) == 0x10, 50);
                StringBuilder final = new StringBuilder(32768); uint n = GetFinalPathNameByHandle(h, final, final.Capacity, 0);
                Require(n > 0 && n < final.Capacity && String.Equals(final.ToString(), @"\\?\" + component, StringComparison.OrdinalIgnoreCase), 50);
            }
            System.Security.AccessControl.DirectorySecurity acl = Directory.GetAccessControl(stage);
            string owner = acl.GetOwner(typeof(SecurityIdentifier)).Value;
            Require((owner == "S-1-5-18" || owner == "S-1-5-32-544") && acl.AreAccessRulesProtected, 50);
            System.Security.AccessControl.AuthorizationRuleCollection rules = acl.GetAccessRules(true, true, typeof(SecurityIdentifier));
            Require(rules.Count > 0, 50);
            const System.Security.AccessControl.FileSystemRights writeMask = System.Security.AccessControl.FileSystemRights.Write | System.Security.AccessControl.FileSystemRights.Delete | System.Security.AccessControl.FileSystemRights.DeleteSubdirectoriesAndFiles | System.Security.AccessControl.FileSystemRights.ChangePermissions | System.Security.AccessControl.FileSystemRights.TakeOwnership;
            foreach (System.Security.AccessControl.FileSystemAccessRule rule in rules) {
                string sid = rule.IdentityReference.Value;
                Require(rule.AccessControlType != System.Security.AccessControl.AccessControlType.Allow || sid == "S-1-5-18" || sid == "S-1-5-32-544" || (rule.FileSystemRights & writeMask) == 0, 50);
            }
            // Fixed leaf, CREATE_NEW only, after fixed local protected directory
            // pins. Existing files/links and failed partial reports are retained.
            Put("receiptWritten", true);
            byte[] bytes = new UTF8Encoding(false).GetBytes(new JavaScriptSerializer().Serialize(Result));
            using (FileStream file = new FileStream(Path.Combine(stage, "diagnostic-v2-report.json"), FileMode.CreateNew, FileAccess.Write, FileShare.Read)) { file.Write(bytes, 0, bytes.Length); file.Flush(true); }
        } catch (Exception e) { Put("receiptWritten", false); Put("receiptWriteCode", unchecked((uint)e.HResult)); Refused refusal = e as Refused; if (refusal != null) Put("receiptRefusedStage", refusal.Stage); }
        finally { for (int i = pins.Count - 1; i >= 0; --i) CloseHandle(pins[i]); }
    }
    static string Sid(IntPtr token) { IntPtr data = Info(token, 1); try { return new SecurityIdentifier(Marshal.ReadIntPtr(data)).Value; } finally { Marshal.FreeHGlobal(data); } }
    static bool SystemSid(string sid) { return sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20"; }
    static bool Elevated(IntPtr token) { IntPtr data = Info(token, 20); try { return Marshal.ReadInt32(data) != 0; } finally { Marshal.FreeHGlobal(data); } }
    static uint Integrity(IntPtr token) { IntPtr data = Info(token, 25); try { SecurityIdentifier sid = new SecurityIdentifier(Marshal.ReadIntPtr(data)); byte[] raw = new byte[sid.BinaryLength]; sid.GetBinaryForm(raw, 0); return BitConverter.ToUInt32(raw, raw.Length - 4); } finally { Marshal.FreeHGlobal(data); } }
    static ulong Authentication(IntPtr token) { IntPtr data = Info(token, 10); try { return unchecked((ulong)Marshal.ReadInt64(data, 8)); } finally { Marshal.FreeHGlobal(data); } }
    static void Privilege(IntPtr token, string prefix)
    {
        Luid debug; bool found = LookupPrivilegeValue(null, "SeDebugPrivilege", out debug); Put(prefix + "DebugLookupCode", found ? 0u : Error); Require(found, 31);
        IntPtr data = Info(token, 3);
        try {
            uint count = unchecked((uint)Marshal.ReadInt32(data)); Require(count <= 256, 31); bool present = false, enabled = false;
            for (uint i = 0; i < count; ++i) { int offset = checked(4 + (int)i * 12); if (unchecked((uint)Marshal.ReadInt32(data, offset)) == debug.Low && Marshal.ReadInt32(data, offset + 4) == debug.High) { present = true; enabled = (Marshal.ReadInt32(data, offset + 8) & 2) != 0; } }
            Put(prefix + "DebugPresent", present); Put(prefix + "DebugEnabled", enabled);
        } finally { Marshal.FreeHGlobal(data); }
    }
    static uint KnownFolder(Guid folder, IntPtr token, out string value) { IntPtr p; uint hr = SHGetKnownFolderPath(ref folder, DontVerify, token, out p); try { value = hr == 0 && p != IntPtr.Zero ? Marshal.PtrToStringUni(p) : null; return hr; } finally { if (p != IntPtr.Zero) Marshal.FreeCoTaskMem(p); } }
    static void FolderProbe(IntPtr token, string prefix) { Put(prefix + "FolderAttempted", token != IntPtr.Zero); if (token == IntPtr.Zero) return; string path; uint hr = KnownFolder(LocalAppData, token, out path); Put(prefix + "FolderHresult", hr); Put(prefix + "FolderWin32Facility", (hr & 0xFFFF0000u) == 0x80070000u); if (hr == 0 || (hr & 0xFFFF0000u) == 0x80070000u) Put(prefix + "FolderWin32Code", hr & 0xFFFFu); Put(prefix + "FolderCanonicalLocal", hr == 0 && CanonicalLocal(path)); }

    static void CountFilters()
    {
        Require(Marshal.SizeOf(typeof(FilterTemplate)) == 72 && Marshal.OffsetOf(typeof(FilterTemplate), "ActionMask").ToInt32() == 56, 40);
        IntPtr engine; uint code = FwpmEngineOpen(null, 10, IntPtr.Zero, IntPtr.Zero, out engine); Put("wfpOpenCode", code); if (code != 0) return;
        IntPtr key = Marshal.AllocHGlobal(16); Marshal.StructureToPtr(Provider, key, false); ulong total = 0;
        try {
            foreach (Guid layer in Layers) {
                // Zero conditions match all per the API; use OVERLAPPING (1)
                // consistently with the existing appfirewall read template.
                FilterTemplate template = new FilterTemplate { ProviderKey = key, LayerKey = layer, EnumType = 1, Flags = 0x18, ActionMask = UInt32.MaxValue };
                IntPtr enumeration; code = FwpmFilterCreateEnumHandle(engine, ref template, out enumeration); Put("wfpCreateEnumCode", code); if (code != 0) return;
                try {
                    for (int page = 0; page < 256; ++page) {
                        IntPtr entries; uint count; code = FwpmFilterEnum(engine, enumeration, 128, out entries, out count); Put("wfpEnumCode", code);
                        try { if (code != 0 || count > 128 || count != 0 && entries == IntPtr.Zero) return; total += count; }
                        finally { if (entries != IntPtr.Zero) FwpmFreeMemory(ref entries); }
                        if (count < 128) { if (layer == Layers[Layers.Length - 1]) { Put("wfpCountComplete", true); Put("wfpFilterCount", total); } break; }
                        if (page == 255) return;
                    }
                } finally { Put("wfpDestroyEnumCode", FwpmFilterDestroyEnumHandle(engine, enumeration)); }
            }
        } finally { Marshal.FreeHGlobal(key); Put("wfpCloseCode", FwpmEngineClose(engine)); }
    }

    [StructLayout(LayoutKind.Sequential)] struct FileTime { internal uint Low, High; }
    [StructLayout(LayoutKind.Sequential)] struct Luid { internal uint Low; internal int High; }
    [StructLayout(LayoutKind.Sequential)] struct ByHandle { internal uint Attributes; internal FileTime Creation, Access, Write; internal uint Volume, SizeHigh, SizeLow, Links, IndexHigh, IndexLow; }
    [StructLayout(LayoutKind.Explicit, Size = 48)] struct ProcessBasic { [FieldOffset(32)] internal UIntPtr UniqueProcessId; [FieldOffset(40)] internal UIntPtr InheritedFromUniqueProcessId; }
    [StructLayout(LayoutKind.Sequential)] struct FilterTemplate { internal IntPtr ProviderKey; internal Guid LayerKey; internal uint EnumType, Flags; internal IntPtr ContextTemplate; internal uint Conditions; internal IntPtr Condition; internal uint ActionMask; internal IntPtr CalloutKey; }
    [DllImport("kernel32.dll", SetLastError=true)] static extern IntPtr OpenProcess(uint access, bool inherit, uint pid);
    [DllImport("kernel32.dll")] static extern IntPtr GetCurrentProcess();
    [DllImport("kernel32.dll")] static extern uint GetCurrentProcessId();
    [DllImport("kernel32.dll", SetLastError=true)] static extern bool CloseHandle(IntPtr handle);
    [DllImport("kernel32.dll", SetLastError=true)] static extern bool GetProcessTimes(IntPtr process, out FileTime creation, out FileTime exit, out FileTime kernel, out FileTime user);
    [DllImport("kernel32.dll")] static extern uint WaitForSingleObject(IntPtr handle, uint timeout);
    [DllImport("kernel32.dll", SetLastError=true)] static extern bool ProcessIdToSessionId(uint pid, out uint session);
    [DllImport("kernel32.dll", EntryPoint="QueryFullProcessImageNameW", CharSet=CharSet.Unicode, SetLastError=true)] static extern bool QueryFullProcessImageName(IntPtr process, uint flags, StringBuilder path, ref int size);
    [DllImport("ntdll.dll")] static extern int NtQueryInformationProcess(IntPtr process, int kind, out ProcessBasic value, uint size, out uint returned);
    [DllImport("advapi32.dll", SetLastError=true)] static extern bool OpenProcessToken(IntPtr process, uint access, out IntPtr token);
    [DllImport("advapi32.dll", SetLastError=true)] static extern bool GetTokenInformation(IntPtr token, int kind, IntPtr data, uint size, out uint required);
    [DllImport("advapi32.dll", EntryPoint="LookupPrivilegeValueW", CharSet=CharSet.Unicode, SetLastError=true)] static extern bool LookupPrivilegeValue(string system, string name, out Luid luid);
    [DllImport("shell32.dll", CharSet=CharSet.Unicode)] static extern uint SHGetKnownFolderPath(ref Guid folder, uint flags, IntPtr token, out IntPtr path);
    [DllImport("kernel32.dll", EntryPoint="QueryDosDeviceW", CharSet=CharSet.Unicode, SetLastError=true)] static extern uint QueryDosDevice(string name, StringBuilder target, int size);
    [DllImport("kernel32.dll", EntryPoint="GetDriveTypeW", CharSet=CharSet.Unicode)] static extern uint GetDriveType(string root);
    [DllImport("kernel32.dll", EntryPoint="CreateFileW", CharSet=CharSet.Unicode, SetLastError=true)] static extern IntPtr CreateFile(string path, uint access, uint share, IntPtr security, uint disposition, uint flags, IntPtr template);
    [DllImport("kernel32.dll", SetLastError=true)] static extern bool GetFileInformationByHandle(IntPtr handle, out ByHandle information);
    [DllImport("kernel32.dll", EntryPoint="GetFinalPathNameByHandleW", CharSet=CharSet.Unicode, SetLastError=true)] static extern uint GetFinalPathNameByHandle(IntPtr handle, StringBuilder path, int size, uint flags);
    [DllImport("fwpuclnt.dll", EntryPoint="FwpmEngineOpen0", CharSet=CharSet.Unicode)] static extern uint FwpmEngineOpen(string server, uint authentication, IntPtr identity, IntPtr session, out IntPtr handle);
    [DllImport("fwpuclnt.dll", EntryPoint="FwpmEngineClose0")] static extern uint FwpmEngineClose(IntPtr handle);
    [DllImport("fwpuclnt.dll", EntryPoint="FwpmFilterCreateEnumHandle0")] static extern uint FwpmFilterCreateEnumHandle(IntPtr engine, ref FilterTemplate template, out IntPtr handle);
    [DllImport("fwpuclnt.dll", EntryPoint="FwpmFilterEnum0")] static extern uint FwpmFilterEnum(IntPtr engine, IntPtr enumeration, uint requested, out IntPtr entries, out uint returned);
    [DllImport("fwpuclnt.dll", EntryPoint="FwpmFilterDestroyEnumHandle0")] static extern uint FwpmFilterDestroyEnumHandle(IntPtr engine, IntPtr enumeration);
    [DllImport("fwpuclnt.dll", EntryPoint="FwpmFreeMemory0")] static extern void FwpmFreeMemory(ref IntPtr entries);
}
