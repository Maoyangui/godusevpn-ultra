using System;
using System.Collections.Generic;
using System.Threading;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.AccessControl;
using System.Security.Principal;

namespace GodUseVPN.Ultra.BrowserHost
{
    // This executable links no WebView2/WinForms assembly and never calls any
    // native methods in WindowsGuard. It only tests strings, state and TCP rows.
    internal static class PureTests
    {
        private static int count;
        private static void Check(bool value, string name) { count++; if (!value) throw new Exception(name); }
        private static void Refuses(string input, string name) { try { LaunchOptions.Parse(input); } catch { count++; return; } throw new Exception(name); }
        private static int Main()
        {
            try
            {
                const string valid = "{\"schemaVersion\":1,\"servicePid\":42,\"proxyPort\":2080,\"connectedStrictVerified\":true,\"profile\":{\"schemaVersion\":1,\"engineId\":\"webview2\",\"profileId\":\"sample\",\"identity\":{\"mode\":\"native\"},\"region\":{\"presetId\":\"en-us-pacific\",\"language\":\"en-US\",\"locale\":\"en-US\",\"timezone\":\"America/Los_Angeles\",\"acceptLanguages\":[\"en-US\",\"en\"]},\"viewport\":{\"width\":1280,\"height\":720},\"webrtc\":\"disable-non-proxied-udp\",\"webgl\":\"runtime-default\",\"webgpu\":\"runtime-default\",\"permissions\":{\"camera\":\"block\",\"microphone\":\"block\",\"geolocation\":\"block\",\"notifications\":\"block\",\"clipboardRead\":\"block\"},\"cookies\":\"runtime-default\",\"binding\":{\"mode\":\"loopback-proxy\",\"referenceId\":\"service-main\",\"proxyUrl\":\"socks5://127.0.0.1:2080\",\"allowDirectFallback\":false},\"exitCleanup\":\"retain-profile\"}}";
                var options = LaunchOptions.Parse(valid); Check(options.ProxyPort == 2080 && options.Width == 1280 && options.Timezone == "America/Los_Angeles", "valid_contract");
                foreach (var change in new[] {
                    new[] { "\"connectedStrictVerified\":true", "\"connectedStrictVerified\":false" },
                    new[] { "\"servicePid\":42", "\"servicePid\":0" },
                    new[] { "\"proxyPort\":2080", "\"proxyPort\":2081" },
                    new[] { "\"engineId\":\"webview2\"", "\"engineId\":\"chrome\"" },
                    new[] { "\"profileId\":\"sample\"", "\"profileId\":\"../sample\"" },
                    new[] { "\"profileId\":\"sample\"", "\"profileId\":\"con\"" },
                    new[] { "\"mode\":\"native\"", "\"mode\":\"native\",\"userAgent\":\"forged\"" },
                    new[] { "\"timezone\":\"America/Los_Angeles\"", "\"timezone\":\"UTC\"" },
                    new[] { "\"width\":1280", "\"width\":3841" },
                    new[] { "\"camera\":\"block\"", "\"camera\":\"ask\"" },
                    new[] { "\"webrtc\":\"disable-non-proxied-udp\"", "\"webrtc\":\"runtime-default\"" },
                    new[] { "\"webgl\":\"runtime-default\"", "\"webgl\":\"disabled\"" },
                    new[] { "\"webgpu\":\"runtime-default\"", "\"webgpu\":\"unknown\"" },
                    new[] { "\"cookies\":\"runtime-default\"", "\"cookies\":\"block-third-party\"" },
                    new[] { "\"exitCleanup\":\"retain-profile\"", "\"exitCleanup\":\"delete-owned-profile\"" },
                    new[] { "socks5://127.0.0.1:2080", "socks5://example.invalid:2080" },
                    new[] { "socks5://127.0.0.1:2080", "http://127.0.0.1:2080" },
                    new[] { "\"allowDirectFallback\":false", "\"allowDirectFallback\":true" },
                    new[] { "\"identity\":", "\"extraArguments\":[\"--no-sandbox\"],\"identity\":" },
                    new[] { "\"identity\":", "\"browserExecutableFolder\":\"C:\\\\untrusted\",\"identity\":" },
                    new[] { "\"schemaVersion\":1,\"servicePid\"", "\"schemaVersion\":1,\"schemaVersion\":1,\"servicePid\"" },
                    new[] { "\"width\":1280", "\"width\":1280,\"WIDTH\":1280" },
                    new[] { "\"width\":1280", "\"width\":1280,\"w\\u0069dth\":1280" }
                }) Refuses(valid.Replace(change[0], change[1]), "unsupported_contract");
                Refuses(valid + "{}", "trailing_json"); Refuses(new string(' ', 32769), "oversize_json");
                foreach (string value in new[] { "file:///C:/secret", "javascript:alert(1)", "data:text/html,x", "edge://settings", "https://name:secret@example.invalid/", "mailto:a@example.invalid" }) Check(!LaunchOptions.NetworkUri(value), "unsafe_navigation");
                Check(LaunchOptions.NetworkUri("https://example.invalid/path") && LaunchOptions.NetworkUri("http://example.invalid/"), "network_navigation");
                string arguments = LaunchOptions.BrowserArguments(34567);
                Check(arguments.Contains("socks5://127.0.0.1:34567") && arguments.Contains("--proxy-bypass-list=<-loopback>") && arguments.Contains("MAP * ~NOTFOUND") && arguments.Contains("--disable-quic") && !arguments.Contains("direct://") && !arguments.Contains("no-sandbox") && !arguments.Contains("ignore-certificate"), "fixed_browser_flags");
                var gate = new NavigationGate(); Check(!gate.Allowed, "default_closed"); gate.Configured(); Check(gate.Allowed, "configured_open"); gate.Close(); Check(!gate.Allowed, "revoked_closed");
                bool threw = false; try { gate.Configured(); } catch { threw = true; } Check(threw && !gate.Allowed, "cannot_reopen");
                long ticks = 0; var lease = new PipeLease(() => ticks, 3000); Check(lease.Live, "initial_lease"); ticks = 2500; Check(lease.Frame("{\"type\":\"lease\"}"), "lease_refresh"); ticks = 5499; Check(lease.Live, "lease_before_deadline"); ticks = 5500; Check(!lease.Live && !lease.Frame("{\"type\":\"lease\"}"), "expired_never_reopens");
                foreach (string frame in new[] { "{\"type\":\"close\"}", "{\"type\":\"lease\",\"port\":1}", "{\"type\":\"lease\",\"type\":\"lease\"}", "", "{}" }) { var test = new PipeLease(() => 0, 3000); Check(!test.Frame(frame) && !test.Frame("{\"type\":\"lease\"}"), "invalid_frame_terminal"); }
                Check(WindowsGuard.TcpRowMatches(2, 0x0100007f, 0x2008, 0, 0, 42, 2080, 0, 42, true), "listener_owner");
                Check(!WindowsGuard.TcpRowMatches(2, 0, 0x2008, 0, 0, 42, 2080, 0, 42, true), "wildcard_listener_rejected");
                Check(!WindowsGuard.TcpRowMatches(2, 0x0100007f, 0x2008, 0, 0, 43, 2080, 0, 42, true), "different_listener_owner");
                Check(WindowsGuard.TcpRowMatches(5, 0x0100007f, 0x2008, 0x0100007f, 0x3930, 42, 2080, 12345, 42, false), "connected_owner_tuple");
                Check(!WindowsGuard.TcpRowMatches(5, 0x0100007f, 0x2008, 0x0100007f, 0x3931, 42, 2080, 12345, 42, false), "wrong_connected_peer");
                var outputGate = new NavigationGate(); outputGate.Configured();
                FailureReceipt.CloseThenReport(outputGate.Close, new BrokenWriter(), "configuration_refused");
                Check(!outputGate.Allowed, "broken_stdout_still_revokes");
                var log = new StringWriter(); FailureReceipt.CloseThenReport(delegate { Check(log.ToString().Length == 0, "close_precedes_receipt"); }, log, "untrusted-path-secret");
                Check(log.ToString().Contains("start_refused") && !log.ToString().Contains("untrusted"), "receipt_code_is_fixed");
                AtomicDirectories();
                count += GraphicsScriptTests.Run(valid);
                count += EnvironmentRelayTests.Run(valid);
                count += HardwareTests.Run(valid);
                Console.WriteLine("PASS pure_browser_host_checks=" + count); return 0;
            }
            catch (Exception error) { Console.Error.WriteLine("FAIL " + error.Message); return 1; }
        }
        private static void AtomicDirectories()
        {
            Check(IntPtr.Size == 8 && Marshal.SizeOf(typeof(WindowsGuard.OBJECT_ATTRIBUTES)) == 48 && Marshal.OffsetOf(typeof(WindowsGuard.OBJECT_ATTRIBUTES), "RootDirectory").ToInt32() == 8 && Marshal.OffsetOf(typeof(WindowsGuard.OBJECT_ATTRIBUTES), "SecurityDescriptor").ToInt32() == 32, "nt_object_attributes_x64_abi");
            Check(Marshal.SizeOf(typeof(WindowsGuard.UNICODE_STRING)) == 16 && Marshal.OffsetOf(typeof(WindowsGuard.UNICODE_STRING), "Buffer").ToInt32() == 8 && Marshal.SizeOf(typeof(WindowsGuard.IO_STATUS_BLOCK)) == 16 && Marshal.OffsetOf(typeof(WindowsGuard.IO_STATUS_BLOCK), "Information").ToInt32() == 8, "nt_unicode_iosb_x64_abi");
            int calls = 0, closes = 0; uint expectedDisposition = 2; ulong information = 2; int status = 0;
            WindowsGuard.NtDirectoryCall native = delegate(out IntPtr handle, uint access, ref WindowsGuard.OBJECT_ATTRIBUTES attributes, out WindowsGuard.IO_STATUS_BLOCK io, IntPtr allocation, uint fileAttributes, uint share, uint disposition, uint flags, IntPtr ea, uint eaLength)
            {
                calls++;
                var name = (WindowsGuard.UNICODE_STRING)Marshal.PtrToStructure(attributes.ObjectName, typeof(WindowsGuard.UNICODE_STRING));
                Check(attributes.RootDirectory == new IntPtr(0x1234) && attributes.Length == 48 && attributes.Attributes == (0x40 | 0x1000) && attributes.SecurityQualityOfService == IntPtr.Zero, "relative_pinned_parent_request");
                Check(Marshal.PtrToStringUni(name.Buffer, name.Length / 2) == "fresh-leaf" && name.MaximumLength == name.Length + 2, "relative_component_unicode");
                Check(access == 0x120081 && share == 3 && (share & 4) == 0 && flags == 0x00200021 && disposition == expectedDisposition && allocation == IntPtr.Zero && ea == IntPtr.Zero && eaLength == 0, "atomic_directory_no_delete_share");
                Check(attributes.SecurityDescriptor != IntPtr.Zero && Marshal.ReadByte(attributes.SecurityDescriptor) == 1, "security_descriptor_at_creation");
                handle = new IntPtr(0x2345); io = new WindowsGuard.IO_STATUS_BLOCK { Status = new IntPtr(status), Information = new UIntPtr(information) }; return status;
            };
            Action<IntPtr> close = delegate(IntPtr handle) { Check(handle == new IntPtr(0x2345), "only_returned_handle_closed"); closes++; };
            var pinned = WindowsGuard.OpenRelativeDirectory(new IntPtr(0x1234), "fresh-leaf", new byte[] { 1, 2 }, 2, native, close);
            Check(pinned == new IntPtr(0x2345) && calls == 1 && closes == 0, "fresh_created_handle_retained_without_reopen");
            information = 1; bool refused = false;
            try { WindowsGuard.OpenRelativeDirectory(new IntPtr(0x1234), "fresh-leaf", new byte[] { 1 }, 2, native, close); } catch { refused = true; }
            Check(refused && calls == 2 && closes == 1, "existing_leaf_result_refused_no_retry");
            expectedDisposition = 3;
            Check(WindowsGuard.OpenRelativeDirectory(new IntPtr(0x1234), "fresh-leaf", new byte[] { 1 }, 3, native, close) == new IntPtr(0x2345) && calls == 3, "fixed_container_open_if_is_atomic");
            expectedDisposition = 2; information = 2; status = unchecked((int)0xc0000035); refused = false;
            try { WindowsGuard.OpenRelativeDirectory(new IntPtr(0x1234), "fresh-leaf", new byte[] { 1 }, 2, native, close); } catch { refused = true; }
            Check(refused && calls == 4 && closes == 2, "name_collision_refused_no_open_fallback");
            status = 0x103; refused = false;
            try { WindowsGuard.OpenRelativeDirectory(new IntPtr(0x1234), "fresh-leaf", new byte[] { 1 }, 2, native, close); } catch { refused = true; }
            Check(refused && calls == 5 && closes == 3, "pending_not_accepted_as_created");
            int before = calls;
            foreach (string name in new[] { "", ".", "..", "../old", "old\\profile", "name:stream", "name\0tail", "trailing ", "trailing.", new string('a', 256) })
            {
                refused = false; try { WindowsGuard.OpenRelativeDirectory(new IntPtr(0x1234), name, new byte[] { 1 }, 2, native, close); } catch { refused = true; }
                Check(refused && calls == before, "invalid_component_never_reaches_nt");
            }
            const string sid = "S-1-5-21-1-2-3-1001";
            string privateSddl = "O:" + sid + "G:SYD:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)";
            WindowsGuard.AssertDataDescriptor(new RawSecurityDescriptor(privateSddl), sid, true); Check(true, "private_created_descriptor");
            foreach (var descriptor in new[] {
                new RawSecurityDescriptor(privateSddl.Replace("O:" + sid, "O:BA")),
                new RawSecurityDescriptor(privateSddl.Replace("D:P", "D:")),
                new RawSecurityDescriptor(privateSddl + "(A;;GR;;;BU)"),
                new RawSecurityDescriptor(privateSddl + "(A;OIIO;GA;;;BU)"),
                new RawSecurityDescriptor(ControlFlags.None, new SecurityIdentifier(sid), null, null, null)
            })
            {
                refused = false; try { WindowsGuard.AssertDataDescriptor(descriptor, sid, true); } catch { refused = true; }
                Check(refused, "untrusted_handle_descriptor_refused");
            }
            WindowsGuard.AssertDataDescriptor(new RawSecurityDescriptor("O:BAG:BAD:(A;;FA;;;BA)(A;;0x4;;;BU)"), sid, false); Check(true, "ancestor_create_child_only_allowed");
            refused = false; try { WindowsGuard.AssertDataDescriptor(new RawSecurityDescriptor("O:BAG:BAD:(A;;FA;;;BA)(A;;0x40;;;BU)"), sid, false); } catch { refused = true; }
            Check(refused, "ancestor_delete_child_refused");
        }
        private sealed class BrokenWriter : StringWriter { public override void WriteLine(string text) { throw new IOException("closed test pipe"); } }
    }
}
