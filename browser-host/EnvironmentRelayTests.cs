using System;
using System.Collections.Generic;
using System.IO;
using System.Text;
using System.Threading.Tasks;

namespace GodUseVPN.Ultra.BrowserHost
{
    // In-memory contracts and duplex streams only: no socket, Windows native API,
    // WebView, process inspection, browser launch or system configuration call.
    internal static class EnvironmentRelayTests
    {
        private static int count;
        private static void Check(bool value, string name) { count++; if (!value) throw new Exception(name); }
        private static void Refuse(Action action, string name) { try { action(); } catch { count++; return; } throw new Exception(name); }
        private static EnvironmentOptions Env(string os, string preset, string version = "")
        {
            var value = new Dictionary<string, object> { { "mode", "browser-emulation" }, { "os", os }, { "devicePresetId", preset } }; if (version.Length != 0) value["osVersion"] = version;
            return EnvironmentOptions.Parse(new Dictionary<string, object> { { "environment", value } });
        }
        internal static int Run(string valid)
        {
            string secret = new string('a', 64);
            string custom = valid.Replace("\"profile\":", "\"relayPid\":50,\"relayUsername\":\"ultra-browser\",\"relayPassword\":\"" + secret + "\",\"profile\":").Replace("\"referenceId\":\"service-main\"", "\"referenceId\":\"profile-relay\"");
            var parsed = LaunchOptions.Parse(custom); Check(parsed.ProfileProxy && parsed.RelayPid == 50 && parsed.RelayUsername == "ultra-browser" && parsed.RelayPassword == secret && parsed.ServicePid == 42, "authenticated_profile_proxy_contract");
            foreach (string bad in new[] { custom.Replace("\"relayPid\":50", "\"relayPid\":0"), custom.Replace("ultra-browser", "other-user"), custom.Replace(secret, new string('a', 63)), custom.Replace(secret, new string('A', 64)), custom.Replace("\"referenceId\":\"profile-relay\"", "\"referenceId\":\"service-main\""), valid.Replace("\"referenceId\":\"service-main\"", "\"referenceId\":\"profile-relay\"") }) Refuse(() => LaunchOptions.Parse(bad), "relay_contract_refused");
            Check(!LaunchOptions.Parse(valid).ProfileProxy, "legacy_scm_proxy_has_no_credentials");
            string[] operating = { "windows", "macos", "android", "ios" }, presets = { "windows-desktop", "macos-desktop", "pixel-7", "iphone-14" }, platforms = { "Win32", "MacIntel", "Linux armv8l", "iPhone" };
            string nativeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0";
            var hints = Hints();
            foreach (string manual in new[] { "QA-Custom/1.0", " User edited exact spaces ", new string('x', 1024) })
            {
                string body = LaunchOptions.Json.Serialize(new Dictionary<string, object> { { "mode", "manual" }, { "userAgent", manual } });
                var manualOptions = LaunchOptions.Parse(valid.Replace("{\"mode\":\"native\"}", body));
                Check(manualOptions.UserAgent == manual && manualOptions.IdentityMode == "manual", "manual_contract_preserves_text");
                var requested = RuntimeIdentity.Derive(new EnvironmentOptions(), nativeUA, hints, "", "Win32", "140.0.3485.81", "manual", manual);
                Check(requested.UserAgent == manual && LaunchOptions.Text(requested.Hints,"platformVersion") == "15.0.0", "manual_native_keeps_live_ch");
                RuntimeIdentity.VerifyUserAgentHeaders(new Dictionary<string,object>{{"User-Agent",manual}},manual); Check(true,"actual_http_ua_matches_exact_manual");
                Refuse(()=>RuntimeIdentity.VerifyUserAgentHeader(nativeUA,manual),"manual_http_native_reversion_refused");
            }
            foreach (string bad in new[] { "", " ", new string('x',1025), "UA\r\nHeader: value", "UA\tvalue", "UA\0", "UA\x7f", "UA中文" })
            {
                string body = LaunchOptions.Json.Serialize(new Dictionary<string, object> { { "mode", "manual" }, { "userAgent", bad } });
                Refuse(()=>LaunchOptions.Parse(valid.Replace("{\"mode\":\"native\"}",body)),"manual_ascii_bound_refused");
            }
            for(int ascii=0;ascii<=255;ascii++) { string value="A"+(char)ascii+"B"; if(ascii>=32&&ascii<=126){LaunchOptions.ValidateManualUserAgent(value);Check(true,"manual_printable_ascii");}else Refuse(()=>LaunchOptions.ValidateManualUserAgent(value),"manual_control_nonascii_refused"); }
            Check(LaunchOptions.Parse(valid.Replace("{\"mode\":\"native\"}","{\"mode\":\"generated\"}")).IdentityMode=="generated","generated_no_environment_contract");
            Refuse(()=>LaunchOptions.Parse(valid.Replace("{\"mode\":\"native\"}","{\"mode\":\"generated\",\"userAgent\":\"frontend\"}")),"generated_frontend_ua_refused");
            Refuse(()=>LaunchOptions.Parse(valid.Replace("{\"mode\":\"native\"}","{\"mode\":\"manual\",\"userAgent\":\"QA/1\",\"clientHints\":{}}")),"manual_frontend_ch_refused");
            var generatedNative=RuntimeIdentity.Derive(new EnvironmentOptions(),nativeUA,hints,"","Win32","140.0.3485.81","generated");
            Check(generatedNative.UserAgent.Contains("Chrome/140.0.0.0")&&!generatedNative.UserAgent.Contains("Chrome/140.0.7339.90")&&!generatedNative.UserAgent.Contains("Edg/")&&LaunchOptions.Text(generatedNative.Hints,"platformVersion")=="15.0.0","generated_reduced_major_keeps_real_chromium_and_native_ch");
            Refuse(()=>RuntimeIdentity.VerifyUserAgentHeaders(new Dictionary<string,object>(),"QA/1"),"missing_http_ua_refused");
            Refuse(()=>RuntimeIdentity.VerifyUserAgentHeaders(new Dictionary<string,object>{{"User-Agent","QA/1"},{"user-agent","QA/1"}},"QA/1"),"duplicate_http_ua_refused");
            for (int n = 0; n < operating.Length; n++)
            {
                var environment = Env(operating[n], presets[n]);
                string json = valid.Replace("\"identity\":", "\"environment\":{\"mode\":\"browser-emulation\",\"os\":\"" + operating[n] + "\",\"devicePresetId\":\"" + presets[n] + "\"},\"identity\":");
                Check(LaunchOptions.Parse(json).Environment.PresetId == presets[n], "four_os_contract");
                if (operating[n] != "windows")
                {
                    string named = json.Replace("\"webgpu\":\"runtime-default\"", "\"webgpu\":\"disabled\"").Replace("\"webrtc\":", "\"graphics\":{\"mode\":\"webgl-metadata\",\"presetId\":\"intel-uhd-620-d3d11\",\"vendor\":\"Google Inc. (Intel)\",\"renderer\":\"ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)\"},\"webrtc\":");
                    Refuse(() => LaunchOptions.Parse(named), "non_windows_named_d3d_gpu_refused");
                    Check(LaunchOptions.Parse(named.Replace("\"presetId\":\"intel-uhd-620-d3d11\"", "\"presetId\":\"manual\"")).Graphics.PresetId == "manual", "non_windows_manual_gpu_scope_explicit");
                }
                if (environment.Mobile)
                {
                    Check(LaunchOptions.Parse(json.Replace("\"width\":1280", "\"width\":320").Replace("\"height\":720", "\"height\":568")).Environment.Mobile, "mobile_actual_small_viewport");
                    Refuse(() => LaunchOptions.Parse(json.Replace("\"width\":1280", "\"width\":319")), "mobile_viewport_too_small");
                }
                var identity = RuntimeIdentity.Derive(environment, nativeUA, hints, "", "Win32", "140.0.3485.81", "generated");
                var nativeEnvironment = RuntimeIdentity.Derive(environment, nativeUA, hints, "", "Win32", "140.0.3485.81", "native");
                Check(nativeEnvironment.UserAgent==nativeUA&&nativeEnvironment.NavigatorPlatform==environment.NavigatorPlatform&&LaunchOptions.Text(nativeEnvironment.Hints,"platformVersion")==environment.OSVersion,"native_raw_ua_preserved_with_each_emulated_environment");
                var manualEnvironment=RuntimeIdentity.Derive(environment,nativeUA,hints,"","Win32","140.0.3485.81","manual","My edited UA/7");
                Check(manualEnvironment.UserAgent=="My edited UA/7"&&LaunchOptions.Text(manualEnvironment.Hints,"platform")==environment.Platform&&LaunchOptions.Text(manualEnvironment.Hints,"model")==environment.Model,"manual_environment_ua_does_not_override_ch");
                Check(LaunchOptions.Parse(json.Replace("{\"mode\":\"native\"}","{\"mode\":\"generated\"}")).Environment.OS==operating[n],"generated_four_os_contract");
                Check(identity.ChromiumVersion == "140.0.7339.90" && identity.UserAgent.Contains("Chrome/140.0.0.0") && !identity.UserAgent.Contains("Chrome/140.0.7339.90") && !identity.UserAgent.Contains("Chrome/140.0.3485.81") && !identity.UserAgent.Contains("Version/") && identity.NavigatorPlatform == platforms[n], "reduced_ua_real_chromium_full_and_selected_platform");
                Check(LaunchOptions.Text(identity.Hints, "platform") == environment.Platform && LaunchOptions.Text(identity.Hints, "platformVersion") == environment.OSVersion && LaunchOptions.Boolean(identity.Hints, "mobile") == environment.Mobile && LaunchOptions.Text(identity.Hints, "model") == environment.Model && LaunchOptions.Text(identity.Hints, "architecture") == environment.Architecture, "coherent_client_hints");
                Check(((object[])identity.Hints["brands"]).Length == 2 && ((object[])identity.Hints["fullVersionList"]).Length == 2 && identity.UserAgent.Contains(" Mobile") == environment.Mobile, "native_chromium_grease_preserved_edge_removed");
                Check(environment.TouchPoints == (environment.Mobile ? 5 : 0) && environment.DPR == (n == 2 ? 2.625 : n == 3 ? 3 : 1), "preset_metrics_touch");
            }
            Check(EnvironmentOptions.NormalizeVersion("17") == "17.0.0" && EnvironmentOptions.NormalizeVersion("17.2") == "17.2.0" && EnvironmentOptions.NormalizeVersion("999.0.999") == "999.0.999", "os_version_normalization");
            double fit = RuntimeIdentity.FitMobileScale(412, 915, 1280, 680);
            Check(fit > 0 && fit < 1 && Math.Ceiling(915 * fit) + 148 + 64 <= 680, "mobile_real_window_fits_work_area");
            Refuse(() => RuntimeIdentity.FitMobileScale(390, 844, 300, 400), "display_too_small_refused");
            var metric = LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(LaunchOptions.Json.Serialize(RuntimeIdentity.Metrics(Env("android", "pixel-7"), 412, 915, fit))));
            Check(LaunchOptions.Number(metric,"width") == 412 && LaunchOptions.Number(metric,"screenHeight") == 915 && Convert.ToDouble(metric["deviceScaleFactor"]) == 2.625 && Convert.ToDouble(metric["scale"]) == fit && LaunchOptions.Boolean(metric,"mobile"), "actual_cdp_metrics_payload");
            foreach (string bad in new[] { "", "0", "01", "17.02", "17.2.0.1", "1000", "17.-1", "17.2 ", "17;--no-sandbox", "17\n2" }) Refuse(() => EnvironmentOptions.NormalizeVersion(bad), "os_version_refused");
            Refuse(() => Env("ios", "pixel-7"), "preset_os_mismatch");
            Refuse(() => Env("linux", "linux-desktop"), "unsupported_environment");
            Refuse(() => EnvironmentOptions.Parse(new Dictionary<string, object> { { "environment", new Dictionary<string, object> { { "mode", "native" }, { "os", "windows" } } } }), "native_extra_fields_refused");
            var native = RuntimeIdentity.Derive(new EnvironmentOptions(), nativeUA, hints, "", "Win32", "140.0.3485.81"); Check(native.UserAgent == nativeUA && ((object[])native.Hints["brands"]).Length == 3 && LaunchOptions.Text(native.Hints, "model") == "", "native_identity_retained_model_required");
            Refuse(() => RuntimeIdentity.Derive(Env("ios", "iphone-14"), nativeUA.Replace("Chrome/140", "Chrome/141"), hints, "", "Win32", "140.0.3485.81"), "native_chromium_runtime_major_mismatch");
            var missing = Hints(); ((IDictionary<string, object>)((object[])missing["fullVersionList"])[1])["brand"] = "Not Chromium";
            Refuse(() => RuntimeIdentity.Derive(Env("ios", "iphone-14"), nativeUA, missing, "", "Win32", "140.0.3485.81"), "missing_actual_chromium_version_refused");
            Check(WindowsGuard.FrontendRowMatches(5, 0x0100007f, 0x3930, 0x0100007f, 0x2008, 12345, 2080), "frontend_client_reverse_tuple");
            Check(!WindowsGuard.FrontendRowMatches(5, 0x0100007f, 0x2008, 0x0100007f, 0x3930, 12345, 2080) && !WindowsGuard.FrontendRowMatches(5, 0, 0x3930, 0x0100007f, 0x2008, 12345, 2080), "frontend_server_wildcard_not_client");
            SharedUserAgentGolden(); Protocol(secret); return count;
        }
        private static void SharedUserAgentGolden()
        {
            using(var stream=typeof(EnvironmentRelayTests).Assembly.GetManifestResourceStream("ChromiumUaGolden.json"))
            {
                if(stream==null)throw new Exception("shared_ua_golden_resource_missing");
                using(var reader=new StreamReader(stream,Encoding.UTF8))
                {
                    var root=LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(reader.ReadToEnd()));
                    var cases=LaunchOptions.Value(root,"cases") as object[];
                    Check(LaunchOptions.Number(root,"formatVersion")==1&&cases!=null&&cases.Length==9,"shared_ua_golden_format");
                    foreach(object item in cases)
                    {
                        var value=LaunchOptions.Object(item);string os=LaunchOptions.Text(value,"os"),version=LaunchOptions.Text(value,"version");
                        var environment=os==""?new EnvironmentOptions():Env(os,LaunchOptions.Text(value,"devicePresetId"),LaunchOptions.Text(value,"osVersion"));
                        string token=os==""?"Windows NT 10.0; Win64; x64":environment.OSUserAgentToken();
                        string actual=RuntimeIdentity.GeneratedUserAgent(token,version,environment.Mobile);
                        Check(actual==LaunchOptions.Text(value,"userAgent")&&!actual.Contains(version)&&!actual.Contains("CriOS/")&&!actual.Contains("FxiOS/")&&!actual.Contains("Version/"),"same_go_csharp_reduced_ua_golden");
                    }
                }
            }
        }
        private static IDictionary<string, object> Hints()
        {
            return new Dictionary<string, object> {
                {"brands", new object[] {new Dictionary<string,object>{{"brand","Not_A Brand"},{"version","99"}},new Dictionary<string,object>{{"brand","Chromium"},{"version","140"}},new Dictionary<string,object>{{"brand","Microsoft Edge"},{"version","140"}}}},
                {"fullVersionList",new object[]{new Dictionary<string,object>{{"brand","Not_A Brand"},{"version","99.0.0.0"}},new Dictionary<string,object>{{"brand","Chromium"},{"version","140.0.7339.90"}},new Dictionary<string,object>{{"brand","Microsoft Edge"},{"version","140.0.3485.81"}}}},
                {"platform","Windows"},{"platformVersion","15.0.0"},{"architecture","x86"},{"bitness","64"},{"mobile",false}
            };
        }
        private static void Protocol(string secret)
        {
            var browser = new Duplex(new byte[] {5, 2, 0, 2}); RelayProtocol.FrontendGreeting(browser).GetAwaiter().GetResult(); Check(browser.Written.Length == 0, "no_frontend_accept_before_upstream_auth");
            var relay = new Duplex(new byte[] {5, 2, 1, 0}); RelayProtocol.UpstreamGreeting(relay, "ultra-browser", secret).GetAwaiter().GetResult(); byte[] sent = relay.Written;
            Check(sent.Length == 3 + 3 + 13 + 64 && sent[0] == 5 && sent[2] == 2 && sent[3] == 1 && sent[4] == 13 && sent[18] == 64 && Encoding.ASCII.GetString(sent, 5, 13) == "ultra-browser" && Encoding.ASCII.GetString(sent, 19, 64) == secret, "real_rfc1929_auth_frame");
            RelayProtocol.FrontendAccepted(browser).GetAwaiter().GetResult(); Check(browser.Written.Length == 2 && browser.Written[0] == 5 && browser.Written[1] == 0, "frontend_noauth_only_after_auth");
            var legacy = new Duplex(new byte[] {5, 0}); RelayProtocol.UpstreamGreeting(legacy, "", "").GetAwaiter().GetResult(); Check(legacy.Written.Length == 3 && legacy.Written[2] == 0, "legacy_service_noauth_negotiation");
            foreach (byte[] bad in new[] {new byte[]{5,0},new byte[]{5,2,1,1},new byte[]{5,2,2,0},new byte[]{5,255},new byte[]{5},new byte[]{}}) Refuse(() => RelayProtocol.UpstreamGreeting(new Duplex(bad), "ultra-browser", secret).GetAwaiter().GetResult(), "auth_no_downgrade_or_partial_accept");
            foreach (byte[] bad in new[] {new byte[]{4,1,0},new byte[]{5,0},new byte[]{5,17},new byte[]{5,1,2},new byte[]{5,1},new byte[]{}}) Refuse(() => RelayProtocol.FrontendGreeting(new Duplex(bad)).GetAwaiter().GetResult(), "frontend_handshake_refused");
        }
        private sealed class Duplex : Stream
        {
            private readonly MemoryStream read, write = new MemoryStream(); internal Duplex(byte[] data) { read = new MemoryStream(data); }
            internal byte[] Written { get { return write.ToArray(); } }
            public override bool CanRead { get { return true; } } public override bool CanSeek { get { return false; } } public override bool CanWrite { get { return true; } }
            public override long Length { get { return read.Length; } } public override long Position { get { return read.Position; } set { throw new NotSupportedException(); } }
            public override int Read(byte[] buffer, int offset, int count) { return read.Read(buffer, offset, count); } public override void Write(byte[] buffer, int offset, int count) { write.Write(buffer, offset, count); }
            public override void Flush() { } public override long Seek(long offset, SeekOrigin origin) { throw new NotSupportedException(); } public override void SetLength(long value) { throw new NotSupportedException(); }
        }
    }
}
