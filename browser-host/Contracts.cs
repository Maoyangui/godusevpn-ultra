using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Text;
using System.Text.RegularExpressions;
using System.Web.Script.Serialization;
using System.Diagnostics;
using System.Threading;

namespace GodUseVPN.Ultra.BrowserHost
{
    // This contract deliberately has no executable path, data path or extra flags.
    internal sealed class LaunchOptions
    {
        internal int ServicePid, ProxyPort, Width, Height, RelayPid;
        internal string RelayUsername = "", RelayPassword = "";
        internal bool ProfileProxy { get { return RelayPid != 0; } }
        internal string ProfileId, Language, Locale, Timezone, AcceptLanguage, UserAgent;
        internal string IdentityMode;
        internal GraphicsOptions Graphics;
        internal EnvironmentOptions Environment;
        internal HardwareOptions Hardware;
        internal bool WebGpuDisabled;
        internal IDictionary<string, object> ClientHints;
        internal static JavaScriptSerializer Json { get { return new JavaScriptSerializer { MaxJsonLength = 32768, RecursionLimit = 20 }; } }

        internal static LaunchOptions Parse(string line)
        {
            if (String.IsNullOrEmpty(line) || line.Length > 32768) throw Reject("launch_size");
            StrictJson.Check(line);
            var root = Object(Json.DeserializeObject(line));
            Keys(root, "schemaVersion", "servicePid", "proxyPort", "connectedStrictVerified", "profile", "relayPid", "relayUsername", "relayPassword");
            if (Number(root, "schemaVersion") != 1 || !Boolean(root, "connectedStrictVerified")) throw Reject("verified_strict_connection_required");
            var result = new LaunchOptions { ServicePid = Number(root, "servicePid"), ProxyPort = Number(root, "proxyPort") };
            if (result.ServicePid <= 0 || result.ProxyPort < 1 || result.ProxyPort > 65535) throw Reject("invalid_service_binding");
            var p = Object(Value(root, "profile"));
            Keys(p, "schemaVersion", "engineId", "profileId", "identity", "environment", "hardware", "region", "viewport", "webrtc", "webgl", "webgpu", "graphics", "permissions", "cookies", "binding", "exitCleanup", "randomizationSeed");
            if (Number(p, "schemaVersion") != 1 || Text(p, "engineId") != "webview2") throw Reject("webview2_only");
            result.ProfileId = Text(p, "profileId");
            if (!Regex.IsMatch(result.ProfileId, "^[a-z][a-z0-9-]{0,47}$") || result.ProfileId.EndsWith("-") || Regex.IsMatch(result.ProfileId, "^(con|prn|aux|nul|com[1-9]|lpt[1-9])$")) throw Reject("invalid_profile_id");
            var identity = Object(Value(p, "identity"));
            Keys(identity, "mode", "userAgent", "clientHints");
            result.IdentityMode = Text(identity, "mode");
            result.UserAgent = OptionalText(identity, "userAgent");
            if (result.IdentityMode == "native")
            {
                if (result.UserAgent.Length != 0 || identity.ContainsKey("clientHints")) throw Reject("native_identity_override");
            }
            else if (result.IdentityMode == "generated")
            {
                if (result.UserAgent.Length != 0 || identity.ContainsKey("clientHints")) throw Reject("generated_identity_must_be_host_derived");
            }
            else if (result.IdentityMode == "manual")
            {
                ValidateManualUserAgent(result.UserAgent);
                if (identity.ContainsKey("clientHints")) throw Reject("manual_client_hints_must_be_host_derived");
            }
            else if (result.IdentityMode == "observed-exact")
            {
                if (result.UserAgent.Length == 0 || result.UserAgent.Length > 1024 || Regex.IsMatch(result.UserAgent, "[^\\x20-\\x7e]")) throw Reject("invalid_observed_identity");
                result.ClientHints = Object(Value(identity, "clientHints"));
                Keys(result.ClientHints, "brands", "fullVersionList", "platform", "platformVersion", "architecture", "bitness", "mobile", "model");
            }
            else throw Reject("unsupported_identity");
            result.Environment = EnvironmentOptions.Parse(p);
            result.Hardware = HardwareOptions.Parse(p);
            if (result.Environment.Emulated && result.IdentityMode == "observed-exact") throw Reject("emulation_requires_runtime_native_source");
            var region = Object(Value(p, "region"));
            Keys(region, "presetId", "language", "locale", "timezone", "acceptLanguages");
            string[][] presets = {
                new[] { "en-us-pacific", "en-US", "America/Los_Angeles", "en-US,en" },
                new[] { "en-us-eastern", "en-US", "America/New_York", "en-US,en" },
                new[] { "en-gb", "en-GB", "Europe/London", "en-GB,en" },
                new[] { "de-de", "de-DE", "Europe/Berlin", "de-DE,de,en" },
                new[] { "ja-jp", "ja-JP", "Asia/Tokyo", "ja-JP,ja,en" },
                new[] { "zh-cn", "zh-CN", "Asia/Shanghai", "zh-CN,zh,en" }
            };
            string[] selected = null;
            foreach (var preset in presets) if (preset[0] == Text(region, "presetId")) selected = preset;
            var languages = Value(region, "acceptLanguages") as object[];
            if (selected == null || languages == null) throw Reject("unsupported_region");
            var languageText = new List<string>();
            foreach (var item in languages) { if (!(item is string)) throw Reject("invalid_languages"); languageText.Add((string)item); }
            if (Text(region, "language") != selected[1] || Text(region, "locale") != selected[1] || Text(region, "timezone") != selected[2] || String.Join(",", languageText) != selected[3]) throw Reject("inconsistent_region");
            result.Language = result.Locale = selected[1]; result.Timezone = selected[2]; result.AcceptLanguage = selected[3];
            var viewport = Object(Value(p, "viewport")); Keys(viewport, "width", "height");
            result.Width = Number(viewport, "width"); result.Height = Number(viewport, "height");
            if (result.Width < (result.Environment.Mobile ? 320 : 800) || result.Width > 3840 || result.Height < (result.Environment.Mobile ? 568 : 600) || result.Height > 2160) throw Reject("unsupported_viewport");
            string webgpu = Text(p, "webgpu");
            if (Text(p, "webrtc") != "disable-non-proxied-udp" || Text(p, "webgl") != "runtime-default" || (webgpu != "runtime-default" && webgpu != "disabled") || Text(p, "cookies") != "runtime-default" || Text(p, "exitCleanup") != "retain-profile") throw Reject("unsupported_runtime_setting");
            result.WebGpuDisabled = webgpu == "disabled";
            result.Graphics = GraphicsOptions.Parse(p, result.WebGpuDisabled);
            if (result.Environment.Emulated && result.Environment.OS != "windows" && result.Graphics.Metadata && result.Graphics.PresetId != "manual") throw Reject("non_windows_named_d3d_graphics_refused");
            var permissions = Object(Value(p, "permissions")); Keys(permissions, "camera", "microphone", "geolocation", "notifications", "clipboardRead");
            foreach (var key in new[] { "camera", "microphone", "geolocation", "notifications", "clipboardRead" }) if (Text(permissions, key) != "block") throw Reject("permissions_block_only");
            var binding = Object(Value(p, "binding")); Keys(binding, "mode", "referenceId", "proxyUrl", "allowDirectFallback");
            if (Text(binding, "mode") != "loopback-proxy" || Boolean(binding, "allowDirectFallback") || Text(binding, "proxyUrl") != "socks5://127.0.0.1:" + result.ProxyPort.ToString(CultureInfo.InvariantCulture) || !Regex.IsMatch(Text(binding, "referenceId"), "^[a-z][a-z0-9-]{0,47}$")) throw Reject("socks5_binding_required");
            bool relayFields = root.ContainsKey("relayPid") || root.ContainsKey("relayUsername") || root.ContainsKey("relayPassword");
            if (Text(binding, "referenceId") == "profile-relay")
            {
                result.RelayPid = Number(root, "relayPid"); result.RelayUsername = Text(root, "relayUsername"); result.RelayPassword = Text(root, "relayPassword");
                if (result.RelayPid <= 0 || result.RelayUsername != "ultra-browser" || !Regex.IsMatch(result.RelayPassword, "^[a-f0-9]{64}$")) throw Reject("authenticated_profile_relay_required");
            }
            else if (relayFields) throw Reject("unexpected_relay_credentials");
            if (p.ContainsKey("randomizationSeed") && !Regex.IsMatch(Text(p, "randomizationSeed"), "^[A-Za-z0-9_-]{1,128}$")) throw Reject("invalid_preview_seed");
            // A seed describes how the caller selected finite values; it never
            // authorizes an unimplemented hardware/fingerprint override.
            return result;
        }

        internal static Exception Reject(string code) { return new InvalidOperationException(code); }
        internal static IDictionary<string, object> Object(object value) { var result = value as IDictionary<string, object>; if (result == null) throw Reject("object_required"); return result; }
        internal static object Value(IDictionary<string, object> value, string key) { object result; if (!value.TryGetValue(key, out result)) throw Reject("missing_" + key); return result; }
        internal static string Text(IDictionary<string, object> value, string key) { var result = Value(value, key) as string; if (result == null) throw Reject("string_" + key); return result; }
        internal static string OptionalText(IDictionary<string, object> value, string key) { return value.ContainsKey(key) ? Text(value, key) : ""; }
        internal static void ValidateManualUserAgent(string value)
        { if (String.IsNullOrWhiteSpace(value) || value.Length > 1024 || Regex.IsMatch(value, "[^\\x20-\\x7e]")) throw Reject("manual_ua_printable_ascii_1_to_1024_required"); }
        internal static int Number(IDictionary<string, object> value, string key) { var n = Value(value, key); if (!(n is int)) throw Reject("integer_" + key); return (int)n; }
        internal static bool Boolean(IDictionary<string, object> value, string key) { var n = Value(value, key); if (!(n is bool)) throw Reject("boolean_" + key); return (bool)n; }
        internal static void Keys(IDictionary<string, object> value, params string[] keys) { var allowed = new HashSet<string>(keys, StringComparer.Ordinal); foreach (var key in value.Keys) if (!allowed.Contains(key)) throw Reject("unknown_field"); }

        internal static bool NetworkUri(string value)
        {
            Uri uri;
            return value != null && value.Length <= 8192 && Uri.TryCreate(value, UriKind.Absolute, out uri) &&
                (uri.Scheme == "https" || uri.Scheme == "http") && uri.UserInfo.Length == 0 && uri.Host.Length != 0;
        }

        internal static string BrowserArguments(int relayPort, bool disableWebGpu = false)
        {
            if (relayPort < 1 || relayPort > 65535) throw Reject("invalid_relay_port");
            return "--proxy-server=socks5://127.0.0.1:" + relayPort.ToString(CultureInfo.InvariantCulture) +
                " --proxy-bypass-list=<-loopback> --host-resolver-rules=\"MAP * ~NOTFOUND\" --disable-quic --force-webrtc-ip-handling-policy=disable_non_proxied_udp" +
                (disableWebGpu ? " --disable-features=WebGPUService" : "");
        }
    }

    // JavaScriptSerializer alone silently accepts duplicate keys. Validate the
    // token structure first, decoding each JSON key before checking uniqueness.
    internal sealed class StrictJson
    {
        private readonly string text; private int position;
        private StrictJson(string value) { text = value; }
        internal static void Check(string value) { var parser = new StrictJson(value); parser.Scan(0); parser.Space(); if (parser.position != value.Length) throw LaunchOptions.Reject("trailing_json"); }
        private void Space() { while (position < text.Length && " \r\n\t".IndexOf(text[position]) >= 0) position++; }
        private string Quoted()
        {
            Space(); if (position >= text.Length || text[position] != '"') throw LaunchOptions.Reject("invalid_json");
            int start = position++; bool escape = false;
            while (position < text.Length) { char c = text[position++]; if (escape) { escape = false; continue; } if (c == '\\') { escape = true; continue; } if (c == '"') return LaunchOptions.Json.Deserialize<string>(text.Substring(start, position - start)); }
            throw LaunchOptions.Reject("invalid_json");
        }
        private void Scan(int depth)
        {
            if (depth > 16) throw LaunchOptions.Reject("json_depth");
            Space(); if (position >= text.Length) throw LaunchOptions.Reject("invalid_json");
            char c = text[position];
            if (c == '"') { Quoted(); return; }
            if (c == '{' || c == '[')
            {
                position++; var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase); Space(); char close = c == '{' ? '}' : ']';
                if (position < text.Length && text[position] == close) { position++; return; }
                while (true)
                {
                    if (c == '{') { if (!seen.Add(Quoted())) throw LaunchOptions.Reject("duplicate_json_field"); Space(); if (position >= text.Length || text[position++] != ':') throw LaunchOptions.Reject("invalid_json"); }
                    Scan(depth + 1); Space(); if (position >= text.Length) throw LaunchOptions.Reject("invalid_json");
                    char delimiter = text[position++]; if (delimiter == close) return; if (delimiter != ',') throw LaunchOptions.Reject("invalid_json");
                }
            }
            int token = position;
            while (position < text.Length && ",]} \r\n\t".IndexOf(text[position]) < 0) position++;
            if (token == position) throw LaunchOptions.Reject("invalid_json");
        }
    }

    internal sealed class NavigationGate
    {
        private int state;
        internal bool Allowed { get { return Volatile.Read(ref state) == 1; } }
        internal void Configured() { if (Interlocked.CompareExchange(ref state, 1, 0) != 0) throw LaunchOptions.Reject("closed_session"); }
        internal void Close() { Interlocked.Exchange(ref state, 2); }
    }

    internal sealed class PipeLease
    {
        private readonly Func<long> clock;
        private readonly long timeout;
        private readonly object sync = new object();
        private long latest;
        private bool closed;
        internal PipeLease() : this(Stopwatch.GetTimestamp, Stopwatch.Frequency * 3) { }
        internal PipeLease(Func<long> ticks, long deadline) { clock = ticks; timeout = deadline; latest = clock(); }
        internal bool Live { get { lock (sync) { if (closed || clock() - latest >= timeout) { closed = true; return false; } return true; } } }
        internal bool Frame(string text)
        {
            lock (sync)
            {
                if (!Live || text == null || text.Length > 128) { closed = true; return false; }
                try
                {
                    StrictJson.Check(text); var frame = LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(text));
                    LaunchOptions.Keys(frame, "type");
                    if (LaunchOptions.Text(frame, "type") != "lease") { closed = true; return false; }
                    latest = clock(); return true;
                }
                catch { closed = true; return false; }
            }
        }
        internal void Close() { lock (sync) { closed = true; } }
    }

    internal static class FailureReceipt
    {
        internal static void CloseThenReport(Action close, TextWriter writer, string code)
        {
            try { close(); }
            finally
            {
                if (code != "configuration_refused") code = "start_refused";
                try { writer.WriteLine("{\"type\":\"error\",\"code\":\"" + code + "\"}"); writer.Flush(); } catch { }
            }
        }
    }
}
