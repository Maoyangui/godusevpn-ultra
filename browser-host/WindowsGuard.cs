using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Net;
using System.Net.Sockets;
using System.Runtime.InteropServices;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Text;
using Microsoft.Win32;

namespace GodUseVPN.Ultra.BrowserHost
{
    internal sealed class WindowsGuard : IDisposable
    {
        internal readonly string InstallRoot;
        private IntPtr parent, service, browser;
        private readonly int servicePid, port, proxyPid;
        private int browserPid;
        private string browserRuntime;
        internal WindowsGuard(LaunchOptions options)
        {
            AssertOrdinaryToken();
            InstallRoot = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "GodUseVPN Ultra");
            string self = Process.GetCurrentProcess().MainModule.FileName;
            if (!SamePath(self, Path.Combine(InstallRoot, "browser-host", "GodUseVPN.Ultra.BrowserHost.exe"))) throw LaunchOptions.Reject("fixed_installed_host_required");
            AssertCode(InstallRoot, true, false);
            if (!Directory.GetAccessControl(InstallRoot).AreAccessRulesProtected) throw LaunchOptions.Reject("protected_install_root_required");
            AssertCode(Path.GetDirectoryName(self), true, false);
            foreach (string name in new[] { "GodUseVPN.Ultra.BrowserHost.exe", "GodUseVPN.Ultra.BrowserHost.exe.config", "Microsoft.Web.WebView2.Core.dll", "Microsoft.Web.WebView2.WinForms.dll", "WebView2Loader.dll" }) AssertCode(Path.Combine(Path.GetDirectoryName(self), name), false, false);
            int parentPid = ParentPid(); uint pipeServer;
            if (!GetNamedPipeServerProcessId(GetStdHandle(-10), out pipeServer) || pipeServer != parentPid) throw LaunchOptions.Reject("authenticated_parent_pipe_required");
            parent = OpenProcess(0x100000 | 0x1000, false, parentPid);
            if (parent == IntPtr.Zero) throw LaunchOptions.Reject("parent_unavailable");
            try
            {
                if (!SamePath(Image(parent), Path.Combine(InstallRoot, "godusevpn.exe"))) throw LaunchOptions.Reject("trusted_gui_parent_required");
                AssertCode(Path.Combine(InstallRoot, "godusevpn.exe"), false, false);
                AssertSameOrdinaryUser(parent);
                servicePid = options.ServicePid; port = options.ProxyPort;
                if (options.ProfileProxy && options.RelayPid != parentPid) throw LaunchOptions.Reject("profile_relay_parent_mismatch");
                proxyPid = options.ProfileProxy ? parentPid : servicePid;
                service = OpenProcess(0x100000 | 0x1000, false, servicePid);
                if (service == IntPtr.Zero || !SamePath(Image(service), Path.Combine(InstallRoot, "godusevpn-svc.exe"))) throw LaunchOptions.Reject("service_image_mismatch");
                AssertCode(Path.Combine(InstallRoot, "godusevpn-svc.exe"), false, false);
                CheckAlive();
            }
            catch { Dispose(); throw; }
        }

        internal static void AssertOrdinaryToken()
        {
            using (var identity = WindowsIdentity.GetCurrent())
            {
                string sid = identity.User.Value;
                if (sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20" || Elevated(identity.Token)) throw LaunchOptions.Reject("ordinary_user_required");
            }
        }
        private static void AssertSameOrdinaryUser(IntPtr process)
        {
            IntPtr token;
            if (!OpenProcessToken(process, 8, out token)) throw LaunchOptions.Reject("parent_token_unavailable");
            try { using (var identity = new WindowsIdentity(token)) using (var current = WindowsIdentity.GetCurrent()) if (identity.User != current.User || Elevated(token)) throw LaunchOptions.Reject("parent_user_mismatch"); }
            finally { CloseHandle(token); }
        }
        private static bool Elevated(IntPtr token)
        {
            int value, size;
            if (!GetTokenInformation(token, 20, out value, 4, out size)) throw LaunchOptions.Reject("token_elevation_unknown");
            return value != 0;
        }
        internal void CheckAlive()
        {
            if (parent == IntPtr.Zero || service == IntPtr.Zero || WaitForSingleObject(parent, 0) != 258 || WaitForSingleObject(service, 0) != 258) throw LaunchOptions.Reject("binding_process_ended");
            CheckScm();
            if (!TcpOwner(port, 0, proxyPid, true)) throw LaunchOptions.Reject("proxy_listener_owner_mismatch");
        }
        internal void CheckConnectedSocket(TcpClient upstream)
        {
            CheckAlive();
            var local = upstream.Client.LocalEndPoint as IPEndPoint;
            if (local == null || !local.Address.Equals(IPAddress.Loopback) || !TcpOwner(port, local.Port, proxyPid, false)) throw LaunchOptions.Reject("proxy_connection_owner_mismatch");
            // Both process handles stay open for the whole session. A recycled
            // numeric PID cannot turn a terminated trusted process into a match.
            if (WaitForSingleObject(service, 0) != 258) throw LaunchOptions.Reject("service_ended");
        }
        private void CheckScm()
        {
            IntPtr scm = OpenSCManager(null, null, 1); if (scm == IntPtr.Zero) throw LaunchOptions.Reject("scm_unavailable");
            try
            {
                IntPtr handle = OpenService(scm, "godusevpn-ultra", 1 | 4); if (handle == IntPtr.Zero) throw LaunchOptions.Reject("fixed_service_unavailable");
                try
                {
                    SERVICE_STATUS_PROCESS status; int needed;
                    if (!QueryServiceStatusEx(handle, 0, out status, Marshal.SizeOf(typeof(SERVICE_STATUS_PROCESS)), out needed) || status.State != 4 || status.Pid != servicePid || status.Type != 0x10) throw LaunchOptions.Reject("current_service_instance_mismatch");
                    IntPtr buffer = Marshal.AllocHGlobal(65536);
                    try
                    {
                        if (!QueryServiceConfig(handle, buffer, 65536, out needed)) throw LaunchOptions.Reject("service_config_unavailable");
                        var config = (QUERY_SERVICE_CONFIG)Marshal.PtrToStructure(buffer, typeof(QUERY_SERVICE_CONFIG));
                        var args = CommandLine(Marshal.PtrToStringUni(config.BinaryPath));
                        if (config.Type != 0x10 || !String.Equals(Marshal.PtrToStringUni(config.Account), "LocalSystem", StringComparison.OrdinalIgnoreCase) || args.Length != 2 || args[1] != "service" || !SamePath(args[0], Path.Combine(InstallRoot, "godusevpn-svc.exe"))) throw LaunchOptions.Reject("fixed_service_config_mismatch");
                    }
                    finally { Marshal.FreeHGlobal(buffer); }
                }
                finally { CloseServiceHandle(handle); }
            }
            finally { CloseServiceHandle(scm); }
        }
        private static string[] CommandLine(string text)
        {
            int count; IntPtr args = CommandLineToArgvW(text, out count); if (args == IntPtr.Zero) throw LaunchOptions.Reject("invalid_service_command");
            try { var result = new string[count]; for (int n = 0; n < count; n++) result[n] = Marshal.PtrToStringUni(Marshal.ReadIntPtr(args, n * IntPtr.Size)); return result; }
            finally { LocalFree(args); }
        }
        internal static bool TcpRowMatches(uint state, uint localAddress, uint localPort, uint remoteAddress, uint remotePort, uint pid, int expectedPort, int peerPort, int expectedPid, bool listener)
        {
            // MIB_TCPROW_OWNER_PID ports are stored in network byte order.
            int lp = (int)(((localPort & 255) << 8) | ((localPort >> 8) & 255));
            int rp = (int)(((remotePort & 255) << 8) | ((remotePort >> 8) & 255));
            return pid == expectedPid && localAddress == 0x0100007f && lp == expectedPort &&
                (listener ? state == 2 : state == 5 && remoteAddress == 0x0100007f && rp == peerPort);
        }
        private static bool TcpOwner(int localPort, int peerPort, int pid, bool listener)
        {
            int length = 0; uint result = GetExtendedTcpTable(IntPtr.Zero, ref length, false, 2, 5, 0);
            if (result != 122 || length < 4 || length > 16 * 1024 * 1024) throw LaunchOptions.Reject("tcp_owner_table_unavailable");
            IntPtr buffer = Marshal.AllocHGlobal(length);
            try
            {
                result = GetExtendedTcpTable(buffer, ref length, false, 2, 5, 0);
                if (result != 0) throw LaunchOptions.Reject("tcp_owner_table_changed");
                int rows = Marshal.ReadInt32(buffer); if (rows < 0 || rows > (length - 4) / 24) throw LaunchOptions.Reject("tcp_owner_table_invalid");
                bool found = false;
                for (int n = 0; n < rows; n++)
                {
                    IntPtr row = IntPtr.Add(buffer, 4 + n * 24);
                    if (TcpRowMatches((uint)Marshal.ReadInt32(row, 0), (uint)Marshal.ReadInt32(row, 4), (uint)Marshal.ReadInt32(row, 8), (uint)Marshal.ReadInt32(row, 12), (uint)Marshal.ReadInt32(row, 16), (uint)Marshal.ReadInt32(row, 20), localPort, peerPort, pid, listener)) found = true;
                }
                return found;
            }
            finally { Marshal.FreeHGlobal(buffer); }
        }

        internal static string RuntimeDirectory()
        {
            // Only machine-installed stable WebView2 in the standard Program
            // Files roots. Never fall back to user registration or another channel.
            string path = null;
            using (var machine = RegistryKey.OpenBaseKey(RegistryHive.LocalMachine, RegistryView.Registry32))
            using (var key = machine.OpenSubKey(@"Software\Microsoft\EdgeUpdate\ClientState\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"))
            { if (key != null) path = key.GetValue("EBWebView", null, RegistryValueOptions.DoNotExpandEnvironmentNames) as string; }
            if (String.IsNullOrEmpty(path)) throw LaunchOptions.Reject("machine_stable_webview2_required");
            string full = Path.GetFullPath(path).TrimEnd(Path.DirectorySeparatorChar);
            string version = Path.GetFileName(full); Version parsed;
            if (!Version.TryParse(version, out parsed) || parsed.Major < 110 || parsed.Build < 0 || parsed.Revision < 0) throw LaunchOptions.Reject("unsupported_runtime_version");
            bool accepted = false;
            foreach (string root in new[] { Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), Environment.GetFolderPath(Environment.SpecialFolder.ProgramFilesX86) })
                if (!String.IsNullOrEmpty(root) && SamePath(full, Path.Combine(root, "Microsoft", "EdgeWebView", "Application", version))) accepted = true;
            if (!accepted) throw LaunchOptions.Reject("unknown_runtime_path");
            AssertCode(full, true, true); AssertCode(Path.Combine(full, "msedgewebview2.exe"), false, true);
            return full;
        }
        internal static void CheckBrowserPid(uint pid, string runtimeDirectory)
        {
            if (pid == 0 || pid > Int32.MaxValue) throw LaunchOptions.Reject("browser_process_unknown");
            IntPtr handle = OpenProcess(0x100000 | 0x1000, false, (int)pid);
            if (handle == IntPtr.Zero) throw LaunchOptions.Reject("browser_process_unavailable");
            try { if (!SamePath(Image(handle), Path.Combine(runtimeDirectory, "msedgewebview2.exe")) || WaitForSingleObject(handle, 0) != 258) throw LaunchOptions.Reject("actual_browser_image_mismatch"); }
            finally { CloseHandle(handle); }
        }
        internal void AttachBrowser(uint pid, string runtimeDirectory)
        {
            if (browser != IntPtr.Zero || pid == 0 || pid > Int32.MaxValue) throw LaunchOptions.Reject("browser_registration_invalid");
            IntPtr handle = OpenProcess(0x100000 | 0x1000, false, (int)pid);
            try
            {
                if (handle == IntPtr.Zero || !SamePath(Image(handle), Path.Combine(runtimeDirectory, "msedgewebview2.exe")) || WaitForSingleObject(handle, 0) != 258) throw LaunchOptions.Reject("actual_browser_image_mismatch");
                AssertSameOrdinaryUser(handle); browserRuntime = runtimeDirectory; browserPid = (int)pid; browser = handle; handle = IntPtr.Zero;
            }
            finally { if (handle != IntPtr.Zero) CloseHandle(handle); }
        }
        internal static bool FrontendRowMatches(uint state, uint localAddress, uint localPort, uint remoteAddress, uint remotePort, int clientPort, int listenerPort)
        { return TcpRowMatches(state, localAddress, localPort, remoteAddress, remotePort, 1, clientPort, listenerPort, 1, false); }
        private static int FrontendPid(TcpClient client)
        {
            var remote = client.Client.RemoteEndPoint as IPEndPoint; var local = client.Client.LocalEndPoint as IPEndPoint;
            if (remote == null || local == null || !remote.Address.Equals(IPAddress.Loopback) || !local.Address.Equals(IPAddress.Loopback)) throw LaunchOptions.Reject("frontend_non_loopback");
            int size = 0; if (GetExtendedTcpTable(IntPtr.Zero, ref size, false, 2, 5, 0) != 122 || size < 4 || size > 16 * 1024 * 1024) throw LaunchOptions.Reject("frontend_owner_unavailable");
            IntPtr buffer = Marshal.AllocHGlobal(size);
            try
            {
                if (GetExtendedTcpTable(buffer, ref size, false, 2, 5, 0) != 0) throw LaunchOptions.Reject("frontend_owner_changed");
                int rows = Marshal.ReadInt32(buffer), pid = 0; if (rows < 0 || rows > (size - 4) / 24) throw LaunchOptions.Reject("frontend_owner_invalid");
                for (int n = 0; n < rows; n++)
                {
                    IntPtr row = IntPtr.Add(buffer, 4 + n * 24);
                    if (!FrontendRowMatches((uint)Marshal.ReadInt32(row, 0), (uint)Marshal.ReadInt32(row, 4), (uint)Marshal.ReadInt32(row, 8), (uint)Marshal.ReadInt32(row, 12), (uint)Marshal.ReadInt32(row, 16), remote.Port, local.Port)) continue;
                    int found = Marshal.ReadInt32(row, 20); if (found <= 0 || pid != 0 && pid != found) throw LaunchOptions.Reject("frontend_owner_ambiguous"); pid = found;
                }
                if (pid == 0) throw LaunchOptions.Reject("frontend_owner_unknown"); return pid;
            }
            finally { Marshal.FreeHGlobal(buffer); }
        }
        internal IDisposable CheckFrontendSocket(TcpClient client, ISet<int> environmentPids)
        {
            CheckAlive(); if (browser == IntPtr.Zero || WaitForSingleObject(browser, 0) != 258) throw LaunchOptions.Reject("browser_ended");
            int candidate = FrontendPid(client); if (!environmentPids.Contains(candidate)) throw LaunchOptions.Reject("frontend_not_this_webview");
            var parents = new Dictionary<int, int>(); IntPtr snapshot = CreateToolhelp32Snapshot(2, 0); if (snapshot == new IntPtr(-1)) throw LaunchOptions.Reject("frontend_snapshot_failed");
            try { var entry = new PROCESSENTRY32 { Size = (uint)Marshal.SizeOf(typeof(PROCESSENTRY32)) }; for (bool more = Process32First(snapshot, ref entry); more; more = Process32Next(snapshot, ref entry)) if (entry.Pid <= Int32.MaxValue && entry.ParentPid <= Int32.MaxValue) parents[(int)entry.Pid] = (int)entry.ParentPid; }
            finally { CloseHandle(snapshot); }
            var pins = new ProcessPins();
            try
            {
                int pid = candidate; long childCreation = Int64.MaxValue; var seen = new HashSet<int>();
                while (true)
                {
                    if (!seen.Add(pid) || seen.Count > 16) throw LaunchOptions.Reject("frontend_ancestry_invalid");
                    IntPtr handle = OpenProcess(0x100000 | 0x1000, false, pid); if (handle == IntPtr.Zero) throw LaunchOptions.Reject("frontend_process_unavailable"); pins.Handles.Add(handle);
                    long creation, exit, kernel, user;
                    if (!SamePath(Image(handle), Path.Combine(browserRuntime, "msedgewebview2.exe")) || WaitForSingleObject(handle, 0) != 258 || !GetProcessTimes(handle, out creation, out exit, out kernel, out user) || creation > childCreation) throw LaunchOptions.Reject("frontend_process_mismatch");
                    childCreation = creation;
                    if (pid == browserPid) break;
                    if (!parents.TryGetValue(pid, out pid) || pid <= 0) throw LaunchOptions.Reject("frontend_ancestry_unknown");
                }
                // Re-read the tuple after all handles are pinned. A row/PID
                // snapshot alone never authorizes a recycled or replaced process.
                if (FrontendPid(client) != candidate || WaitForSingleObject(browser, 0) != 258) throw LaunchOptions.Reject("frontend_connection_changed");
                return pins;
            }
            catch { pins.Dispose(); throw; }
        }
        private sealed class ProcessPins : IDisposable
        { internal readonly List<IntPtr> Handles = new List<IntPtr>(); public void Dispose() { foreach (var h in Handles) CloseHandle(h); Handles.Clear(); } }

        internal static void AssertNoOverrides()
        {
            foreach (System.Collections.DictionaryEntry entry in Environment.GetEnvironmentVariables())
                if (((string)entry.Key).StartsWith("WEBVIEW2_", StringComparison.OrdinalIgnoreCase) || ((string)entry.Key).StartsWith("CHROME_", StringComparison.OrdinalIgnoreCase)) throw LaunchOptions.Reject("browser_environment_override");
            foreach (var hive in new[] { RegistryHive.LocalMachine, RegistryHive.CurrentUser })
                foreach (var view in new[] { RegistryView.Registry32, RegistryView.Registry64 })
                    using (var root = RegistryKey.OpenBaseKey(hive, view))
                    using (var policy = root.OpenSubKey(@"Software\Policies\Microsoft\Edge\WebView2"))
                        if (policy != null && (policy.SubKeyCount != 0 || policy.ValueCount != 0)) throw LaunchOptions.Reject("webview2_policy_override");
        }
        internal sealed class DataLease : IDisposable
        {
            private readonly List<IntPtr> handles = new List<IntPtr>();
            internal string Path { get; set; }
            internal IntPtr Current { get { if (handles.Count == 0) throw LaunchOptions.Reject("data_parent_missing"); return handles[handles.Count - 1]; } }
            internal void PinVolumeRoot(string path, string sid)
            {
                IntPtr handle = CreateFile(path, 0x20081, 1 | 2, IntPtr.Zero, 3, 0x02200000, IntPtr.Zero);
                if (handle == new IntPtr(-1)) throw LaunchOptions.Reject("data_directory_pin_failed");
                Accept(handle, sid, false);
            }
            internal void Child(string name, string sid, bool privateDirectory, bool fresh)
            {
                byte[] descriptor = privateDirectory ? UserDirectorySecurity().GetSecurityDescriptorBinaryForm() : null;
                IntPtr handle = OpenRelativeDirectory(Current, name, descriptor, privateDirectory ? (fresh ? 2u : 3u) : 1u, NtCreateFile, delegate(IntPtr value) { CloseHandle(value); });
                Accept(handle, sid, privateDirectory);
            }
            private void Accept(IntPtr handle, string sid, bool privateDirectory)
            {
                try
                {
                    FILE_INFORMATION information;
                    if (!GetFileInformationByHandle(handle, out information) || (information.Attributes & 0x10) == 0 || (information.Attributes & 0x400) != 0) throw LaunchOptions.Reject("data_directory_reparse_or_type");
                    AssertDataDescriptor(ReadDescriptor(handle), sid, privateDirectory);
                    handles.Add(handle);
                }
                catch { CloseHandle(handle); throw; }
            }
            public void Dispose() { for (int n = handles.Count - 1; n >= 0; n--) CloseHandle(handles[n]); handles.Clear(); }
        }
        internal static DataLease NewDataDirectory(string id)
        {
            var lease = new DataLease();
            try
            {
                string root = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
                NoReparse(root);
                string sid = WindowsIdentity.GetCurrent().User.Value;
                string current = Path.GetPathRoot(root); lease.PinVolumeRoot(current, sid);
                foreach (string part in root.Substring(current.Length).Split(Path.DirectorySeparatorChar))
                { if (part.Length == 0) continue; lease.Child(part, sid, false, false); }
                foreach (string part in new[] { "GodUseVPN Ultra", "BrowserSessions" })
                {
                    root = Path.Combine(root, part);
                    // FILE_OPEN_IF atomically returns a pinned existing or newly
                    // created container; the exact handle's owner/ACL is checked.
                    lease.Child(part, sid, true, false);
                }
                string leaf = id + "-" + Guid.NewGuid().ToString("N");
                string target = Path.Combine(root, leaf);
                // FILE_CREATE under the already pinned parent creates the leaf
                // and returns its no-DELETE-sharing handle in the same call.
                // No subsequent pathname open can substitute an existing profile.
                lease.Child(leaf, sid, true, true);
                lease.Path = target; return lease;
            }
            catch { lease.Dispose(); throw; }
        }
        internal delegate int NtDirectoryCall(out IntPtr handle, uint access, ref OBJECT_ATTRIBUTES attributes, out IO_STATUS_BLOCK io, IntPtr allocation, uint fileAttributes, uint share, uint disposition, uint options, IntPtr ea, uint eaLength);
        internal static IntPtr OpenRelativeDirectory(IntPtr parent, string name, byte[] descriptor, uint disposition, NtDirectoryCall create, Action<IntPtr> close)
        {
            if (parent == IntPtr.Zero || parent == new IntPtr(-1) || String.IsNullOrEmpty(name) || name.Length > 255 || name == "." || name == ".." || name.IndexOfAny(new[] { '\\', '/', ':', '\0', '<', '>', '"', '|', '?', '*' }) >= 0 || name.EndsWith(".") || name.EndsWith(" ") || (disposition != 1 && disposition != 2 && disposition != 3) || disposition != 1 && (descriptor == null || descriptor.Length == 0)) throw LaunchOptions.Reject("unsafe_relative_directory_request");
            IntPtr text = Marshal.StringToHGlobalUni(name), unicode = IntPtr.Zero;
            GCHandle pinned = default(GCHandle);
            try
            {
                var value = new UNICODE_STRING { Length = checked((ushort)(name.Length * 2)), MaximumLength = checked((ushort)((name.Length + 1) * 2)), Buffer = text };
                unicode = Marshal.AllocHGlobal(Marshal.SizeOf(typeof(UNICODE_STRING))); Marshal.StructureToPtr(value, unicode, false);
                if (descriptor != null) pinned = GCHandle.Alloc(descriptor, GCHandleType.Pinned);
                var attributes = new OBJECT_ATTRIBUTES { Length = Marshal.SizeOf(typeof(OBJECT_ATTRIBUTES)), RootDirectory = parent, ObjectName = unicode, Attributes = 0x40 | 0x1000, SecurityDescriptor = descriptor == null ? IntPtr.Zero : pinned.AddrOfPinnedObject() };
                IO_STATUS_BLOCK io; IntPtr handle;
                int status = create(out handle, 0x120081, ref attributes, out io, IntPtr.Zero, 0x80, 1 | 2, disposition, 0x00200021, IntPtr.Zero, 0);
                ulong information = io.Information.ToUInt64();
                if (status != 0 || unchecked((int)io.Status.ToInt64()) != 0 || handle == IntPtr.Zero || handle == new IntPtr(-1) || disposition == 2 && information != 2 || disposition == 1 && information != 1 || disposition == 3 && information != 1 && information != 2)
                {
                    if (handle != IntPtr.Zero && handle != new IntPtr(-1)) close(handle);
                    throw LaunchOptions.Reject("atomic_directory_open_or_create_failed");
                }
                return handle;
            }
            finally { if (pinned.IsAllocated) pinned.Free(); if (unicode != IntPtr.Zero) Marshal.FreeHGlobal(unicode); Marshal.FreeHGlobal(text); }
        }
        private static DirectorySecurity UserDirectorySecurity()
        {
            string sid = WindowsIdentity.GetCurrent().User.Value;
            var acl = new DirectorySecurity(); acl.SetSecurityDescriptorSddlForm("O:" + sid + "G:" + sid + "D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"); return acl;
        }
        private static RawSecurityDescriptor ReadDescriptor(IntPtr handle)
        {
            IntPtr owner, group, dacl, sacl, descriptor;
            uint status = GetSecurityInfo(handle, 1, 1 | 4, out owner, out group, out dacl, out sacl, out descriptor);
            if (status != 0 || descriptor == IntPtr.Zero) throw LaunchOptions.Reject("data_directory_security_unavailable");
            try
            {
                uint length = GetSecurityDescriptorLength(descriptor);
                if (length < 20 || length > 65536) throw LaunchOptions.Reject("data_directory_security_invalid");
                byte[] bytes = new byte[length]; Marshal.Copy(descriptor, bytes, 0, (int)length); return new RawSecurityDescriptor(bytes, 0);
            }
            finally { LocalFree(descriptor); }
        }
        internal static void AssertDataDescriptor(RawSecurityDescriptor descriptor, string sid, bool privateDirectory)
        {
            if (descriptor.Owner == null || (privateDirectory ? descriptor.Owner.Value != sid : descriptor.Owner.Value != sid && !TrustedSid(descriptor.Owner.Value, true)) || descriptor.DiscretionaryAcl == null || (descriptor.ControlFlags & ControlFlags.DiscretionaryAclPresent) == 0 || privateDirectory && (descriptor.ControlFlags & ControlFlags.DiscretionaryAclProtected) == 0) throw LaunchOptions.Reject("data_directory_owner_or_dacl");
            foreach (GenericAce generic in descriptor.DiscretionaryAcl)
            {
                if (!privateDirectory && (generic.AceFlags & AceFlags.InheritOnly) != 0) continue;
                var ace = generic as CommonAce;
                if (ace == null) throw LaunchOptions.Reject("data_directory_unsupported_ace");
                if (ace.AceQualifier == AceQualifier.AccessDenied) continue;
                if (ace.AceQualifier != AceQualifier.AccessAllowed) throw LaunchOptions.Reject("data_directory_unsupported_ace");
                bool trusted = ace.SecurityIdentifier.Value == sid || TrustedSid(ace.SecurityIdentifier.Value, !privateDirectory);
                if (!trusted && (privateDirectory || ((uint)ace.AccessMask & 0x500d0040) != 0)) throw LaunchOptions.Reject("data_directory_acl");
            }
        }
        private static bool TrustedSid(string sid, bool os) { return sid == "S-1-5-18" || sid == "S-1-5-32-544" || os && sid == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"; }
        private static void AssertCode(string path, bool directory, bool os)
        {
            NoReparse(path); FileSystemSecurity acl = directory ? (FileSystemSecurity)Directory.GetAccessControl(path) : File.GetAccessControl(path);
            if (!TrustedSid(acl.GetOwner(typeof(SecurityIdentifier)).Value, os)) throw LaunchOptions.Reject("untrusted_code_owner");
            foreach (FileSystemAccessRule rule in acl.GetAccessRules(true, true, typeof(SecurityIdentifier)))
                if (rule.AccessControlType == AccessControlType.Allow && ((long)rule.FileSystemRights & 0x500d0156) != 0 && !TrustedSid(rule.IdentityReference.Value, os)) throw LaunchOptions.Reject("writable_code");
            for (string part = Path.GetDirectoryName(path); part != null; part = Path.GetDirectoryName(part))
            {
                var parentAcl = Directory.GetAccessControl(part);
                if (!TrustedSid(parentAcl.GetOwner(typeof(SecurityIdentifier)).Value, true)) throw LaunchOptions.Reject("untrusted_code_ancestor");
                foreach (FileSystemAccessRule rule in parentAcl.GetAccessRules(true, true, typeof(SecurityIdentifier)))
                    if ((rule.PropagationFlags & PropagationFlags.InheritOnly) == 0 && rule.AccessControlType == AccessControlType.Allow && ((long)rule.FileSystemRights & 0x500d0040) != 0 && !TrustedSid(rule.IdentityReference.Value, true)) throw LaunchOptions.Reject("replaceable_code_ancestor");
            }
        }
        private static void NoReparse(string path)
        {
            string full = Path.GetFullPath(path);
            if (full.Length < 3 || full[1] != ':' || full.StartsWith(@"\\") || full.IndexOf(':', 2) >= 0 || new DriveInfo(Path.GetPathRoot(full)).DriveType != DriveType.Fixed) throw LaunchOptions.Reject("local_fixed_path_required");
            for (string part = full; part != null; part = Path.GetDirectoryName(part)) if ((File.GetAttributes(part) & FileAttributes.ReparsePoint) != 0) throw LaunchOptions.Reject("reparse_path");
        }
        private static bool SamePath(string a, string b) { return String.Equals(Path.GetFullPath(a).TrimEnd('\\'), Path.GetFullPath(b).TrimEnd('\\'), StringComparison.OrdinalIgnoreCase); }
        private static string Image(IntPtr handle) { var text = new StringBuilder(32768); int size = text.Capacity; if (!QueryFullProcessImageName(handle, 0, text, ref size)) throw LaunchOptions.Reject("process_image_unavailable"); return text.ToString(); }
        private static int ParentPid()
        {
            IntPtr snapshot = CreateToolhelp32Snapshot(2, 0); if (snapshot == new IntPtr(-1)) throw LaunchOptions.Reject("parent_snapshot_failed");
            try { var entry = new PROCESSENTRY32 { Size = (uint)Marshal.SizeOf(typeof(PROCESSENTRY32)) }; bool more = Process32First(snapshot, ref entry); while (more) { if (entry.Pid == Process.GetCurrentProcess().Id) return (int)entry.ParentPid; more = Process32Next(snapshot, ref entry); } }
            finally { CloseHandle(snapshot); }
            throw LaunchOptions.Reject("parent_unknown");
        }
        public void Dispose() { if (browser != IntPtr.Zero) { CloseHandle(browser); browser = IntPtr.Zero; } if (service != IntPtr.Zero) { CloseHandle(service); service = IntPtr.Zero; } if (parent != IntPtr.Zero) { CloseHandle(parent); parent = IntPtr.Zero; } }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)] private struct PROCESSENTRY32 { public uint Size, Usage, Pid; public UIntPtr Heap; public uint Module, Threads, ParentPid; public int Priority; public uint Flags; [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 260)] public string Exe; }
        [StructLayout(LayoutKind.Sequential)] private struct SERVICE_STATUS_PROCESS { public uint Type, State, Controls, Win32Exit, SpecificExit, Checkpoint, WaitHint; public int Pid; public uint Flags; }
        [StructLayout(LayoutKind.Sequential)] private struct QUERY_SERVICE_CONFIG { public uint Type, Start, Error; public IntPtr BinaryPath, Group; public uint Tag; public IntPtr Dependencies, Account, Display; }
        [StructLayout(LayoutKind.Sequential)] private struct FILE_INFORMATION { public uint Attributes, CreationLow, CreationHigh, AccessLow, AccessHigh, WriteLow, WriteHigh, Volume, SizeHigh, SizeLow, Links, IndexHigh, IndexLow; }
        [StructLayout(LayoutKind.Sequential)] internal struct UNICODE_STRING { public ushort Length, MaximumLength; public IntPtr Buffer; }
        [StructLayout(LayoutKind.Sequential)] internal struct OBJECT_ATTRIBUTES { public int Length; public IntPtr RootDirectory, ObjectName; public uint Attributes; public IntPtr SecurityDescriptor, SecurityQualityOfService; }
        [StructLayout(LayoutKind.Sequential)] internal struct IO_STATUS_BLOCK { public IntPtr Status; public UIntPtr Information; }
        [DllImport("kernel32.dll", SetLastError = true)] private static extern IntPtr OpenProcess(uint access, bool inherit, int pid);
        [DllImport("kernel32.dll", SetLastError = true)] private static extern IntPtr GetStdHandle(int kind);
        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern IntPtr CreateFile(string path, uint access, uint share, IntPtr security, uint creation, uint flags, IntPtr template);
        [DllImport("ntdll.dll", ExactSpelling = true)] private static extern int NtCreateFile(out IntPtr handle, uint access, ref OBJECT_ATTRIBUTES attributes, out IO_STATUS_BLOCK io, IntPtr allocation, uint fileAttributes, uint share, uint disposition, uint options, IntPtr ea, uint eaLength);
        [DllImport("kernel32.dll", SetLastError = true)] private static extern bool GetFileInformationByHandle(IntPtr file, out FILE_INFORMATION information);
        [DllImport("advapi32.dll", SetLastError = true)] private static extern uint GetSecurityInfo(IntPtr handle, int objectType, uint information, out IntPtr owner, out IntPtr group, out IntPtr dacl, out IntPtr sacl, out IntPtr descriptor);
        [DllImport("advapi32.dll", SetLastError = true)] private static extern uint GetSecurityDescriptorLength(IntPtr descriptor);
        [DllImport("kernel32.dll", SetLastError = true)] private static extern bool GetNamedPipeServerProcessId(IntPtr pipe, out uint pid);
        [DllImport("kernel32.dll")] private static extern bool CloseHandle(IntPtr handle);
        [DllImport("kernel32.dll")] private static extern uint WaitForSingleObject(IntPtr handle, uint milliseconds);
        [DllImport("kernel32.dll", SetLastError = true)] private static extern bool GetProcessTimes(IntPtr handle, out long creation, out long exit, out long kernel, out long user);
        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern bool QueryFullProcessImageName(IntPtr handle, uint flags, StringBuilder name, ref int size);
        [DllImport("advapi32.dll", SetLastError = true)] private static extern bool OpenProcessToken(IntPtr process, uint access, out IntPtr token);
        [DllImport("advapi32.dll", SetLastError = true)] private static extern bool GetTokenInformation(IntPtr token, int kind, out int value, int size, out int needed);
        [DllImport("kernel32.dll", SetLastError = true)] private static extern IntPtr CreateToolhelp32Snapshot(uint flags, uint pid);
        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, EntryPoint = "Process32FirstW")] private static extern bool Process32First(IntPtr snapshot, ref PROCESSENTRY32 entry);
        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, EntryPoint = "Process32NextW")] private static extern bool Process32Next(IntPtr snapshot, ref PROCESSENTRY32 entry);
        [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern IntPtr OpenSCManager(string machine, string database, uint access);
        [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern IntPtr OpenService(IntPtr scm, string name, uint access);
        [DllImport("advapi32.dll")] private static extern bool CloseServiceHandle(IntPtr handle);
        [DllImport("advapi32.dll", SetLastError = true)] private static extern bool QueryServiceStatusEx(IntPtr service, int level, out SERVICE_STATUS_PROCESS status, int size, out int needed);
        [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern bool QueryServiceConfig(IntPtr service, IntPtr config, int size, out int needed);
        [DllImport("shell32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern IntPtr CommandLineToArgvW(string command, out int count);
        [DllImport("kernel32.dll")] private static extern IntPtr LocalFree(IntPtr handle);
        [DllImport("iphlpapi.dll", SetLastError = true)] private static extern uint GetExtendedTcpTable(IntPtr table, ref int size, bool order, int family, int tableClass, uint reserved);
    }
}
