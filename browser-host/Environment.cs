using System;
using System.Collections.Generic;
using System.Globalization;
using System.Text.RegularExpressions;

namespace GodUseVPN.Ultra.BrowserHost
{
    // These are website-facing emulation values, never a native OS or engine.
    internal sealed class EnvironmentOptions
    {
        internal string Mode = "native", OS = "", PresetId = "", OSVersion = "", NavigatorPlatform = "", Platform = "", Architecture = "", Model = "";
        internal bool Mobile;
        internal double DPR = 1;
        internal int TouchPoints;
        internal bool Emulated { get { return Mode == "browser-emulation"; } }
        internal static EnvironmentOptions Parse(IDictionary<string, object> profile)
        {
            var result = new EnvironmentOptions(); if (!profile.ContainsKey("environment")) return result;
            var value = LaunchOptions.Object(LaunchOptions.Value(profile, "environment")); LaunchOptions.Keys(value, "mode", "os", "devicePresetId", "osVersion");
            result.Mode = LaunchOptions.Text(value, "mode");
            if (result.Mode == "native") { if (value.Count != 1) throw LaunchOptions.Reject("native_environment_override"); return result; }
            if (!result.Emulated) throw LaunchOptions.Reject("unsupported_environment_mode");
            result.OS = LaunchOptions.Text(value, "os"); result.PresetId = LaunchOptions.Text(value, "devicePresetId");
            string version;
            switch (result.PresetId)
            {
                case "windows-desktop": if (result.OS != "windows") throw LaunchOptions.Reject("environment_preset_mismatch"); version = "10.0.0"; result.NavigatorPlatform = "Win32"; result.Platform = "Windows"; result.Architecture = "x86"; break;
                case "macos-desktop": if (result.OS != "macos") throw LaunchOptions.Reject("environment_preset_mismatch"); version = "13.0.0"; result.NavigatorPlatform = "MacIntel"; result.Platform = "macOS"; result.Architecture = "x86"; break;
                case "pixel-7": if (result.OS != "android") throw LaunchOptions.Reject("environment_preset_mismatch"); version = "13.0.0"; result.NavigatorPlatform = "Linux armv8l"; result.Platform = "Android"; result.Architecture = "arm"; result.Model = "Pixel 7"; result.Mobile = true; result.DPR = 2.625; result.TouchPoints = 5; break;
                case "iphone-14": if (result.OS != "ios") throw LaunchOptions.Reject("environment_preset_mismatch"); version = "16.0.0"; result.NavigatorPlatform = "iPhone"; result.Platform = "iOS"; result.Architecture = "arm"; result.Model = "iPhone"; result.Mobile = true; result.DPR = 3; result.TouchPoints = 5; break;
                default: throw LaunchOptions.Reject("unsupported_device_preset");
            }
            result.OSVersion = NormalizeVersion(value.ContainsKey("osVersion") ? LaunchOptions.Text(value, "osVersion") : version);
            return result;
        }
        internal static string NormalizeVersion(string value)
        {
            if (value == null || !Regex.IsMatch(value, "^(?:0|[1-9][0-9]{0,2})(?:\\.(?:0|[1-9][0-9]{0,2})){0,2}$")) throw LaunchOptions.Reject("invalid_environment_os_version");
            string[] parts = value.Split('.'); if (Int32.Parse(parts[0], CultureInfo.InvariantCulture) == 0) throw LaunchOptions.Reject("invalid_environment_os_version");
            return parts[0] + "." + (parts.Length > 1 ? parts[1] : "0") + "." + (parts.Length > 2 ? parts[2] : "0");
        }
        internal string OSUserAgentToken()
        {
            string underscore = OSVersion.Replace('.', '_');
            switch (OS)
            {
                case "windows": return "Windows NT 10.0; Win64; x64";
                case "macos": return "Macintosh; Intel Mac OS X 10_15_7";
                case "android": return "Linux; Android 10; K";
                case "ios": return "iPhone; CPU iPhone OS " + underscore + " like Mac OS X";
            }
            throw LaunchOptions.Reject("unsupported_environment_os");
        }
    }

    internal sealed class RuntimeIdentity
    {
        internal string UserAgent, NavigatorPlatform, ChromiumVersion;
        internal IDictionary<string, object> Hints;
        internal static RuntimeIdentity Derive(EnvironmentOptions options, string nativeUA, IDictionary<string, object> nativeHints, string nativeModel, string nativePlatform, string runtimeVersion, string identityMode = "native", string manualUA = "")
        {
            if (identityMode != "native" && identityMode != "generated" && identityMode != "manual" && identityMode != "observed-exact") throw LaunchOptions.Reject("unsupported_identity");
            if (identityMode == "manual") LaunchOptions.ValidateManualUserAgent(manualUA);
            else if (identityMode != "observed-exact" && manualUA.Length != 0) throw LaunchOptions.Reject("unexpected_identity_ua");
            Version edge; if (!Version.TryParse(runtimeVersion, out edge) || edge.Build < 0 || edge.Revision < 0) throw LaunchOptions.Reject("runtime_version_unknown");
            var uaMatch = Regex.Match(nativeUA, "(?:^| )Chrome/([0-9]+)\\.[0-9]+\\.[0-9]+\\.[0-9]+(?: |$)");
            if (!uaMatch.Success || uaMatch.Groups[1].Value != edge.Major.ToString(CultureInfo.InvariantCulture) || !nativeUA.Contains("AppleWebKit/537.36") || LaunchOptions.Text(nativeHints, "platform") != "Windows" || LaunchOptions.Boolean(nativeHints, "mobile")) throw LaunchOptions.Reject("native_runtime_identity_mismatch");
            var brands = LaunchOptions.Value(nativeHints, "brands") as object[]; var full = LaunchOptions.Value(nativeHints, "fullVersionList") as object[];
            if (brands == null || full == null || brands.Length < 1 || brands.Length > 8 || full.Length != brands.Length) throw LaunchOptions.Reject("native_runtime_brands_unknown");
            string chromium = null; var keepFull = new List<object>(); var keepBrands = new List<object>(); var seen = new HashSet<string>(StringComparer.Ordinal);
            foreach (object item in full)
            {
                var value = LaunchOptions.Object(item); LaunchOptions.Keys(value, "brand", "version"); string brand = LaunchOptions.Text(value, "brand"), version = LaunchOptions.Text(value, "version");
                if (!seen.Add(brand)) throw LaunchOptions.Reject("duplicate_runtime_brand");
                Version parsed; if (!Version.TryParse(version, out parsed) || parsed.Revision < 0) throw LaunchOptions.Reject("invalid_runtime_full_version");
                if (brand == "Chromium") { if (parsed.Major != edge.Major) throw LaunchOptions.Reject("chromium_major_mismatch"); chromium = version; }
                if (brand != "Microsoft Edge") keepFull.Add(value);
            }
            if (chromium == null) throw LaunchOptions.Reject("native_chromium_version_unknown");
            seen.Clear(); foreach (object item in brands)
            {
                var value = LaunchOptions.Object(item); LaunchOptions.Keys(value, "brand", "version"); string brand = LaunchOptions.Text(value, "brand"), version = LaunchOptions.Text(value, "version");
                if (!seen.Add(brand)) throw LaunchOptions.Reject("duplicate_runtime_brand"); bool matched = false;
                foreach (object f in full) { var fv = LaunchOptions.Object(f); if (LaunchOptions.Text(fv, "brand") == brand && LaunchOptions.Text(fv, "version").Split('.')[0] == version) matched = true; }
                if (!matched) throw LaunchOptions.Reject("runtime_brand_version_mismatch"); if (brand != "Microsoft Edge") keepBrands.Add(value);
            }
            var result = new RuntimeIdentity { UserAgent = nativeUA, NavigatorPlatform = nativePlatform, ChromiumVersion = chromium, Hints = new Dictionary<string, object>(nativeHints) };
            result.Hints["model"] = nativeModel;
            if (options.Emulated)
            {
                if (identityMode == "generated") result.UserAgent = GeneratedUserAgent(options.OSUserAgentToken(), chromium, options.Mobile);
                result.NavigatorPlatform = options.NavigatorPlatform;
                result.Hints = new Dictionary<string, object> { { "brands", keepBrands.ToArray() }, { "fullVersionList", keepFull.ToArray() }, { "platform", options.Platform }, { "platformVersion", options.OSVersion }, { "architecture", options.Architecture }, { "bitness", "64" }, { "mobile", options.Mobile }, { "model", options.Model } };
            }
            else if (identityMode == "generated")
            {
                var token = Regex.Match(nativeUA, "^Mozilla/5\\.0 \\((Windows NT [0-9]+\\.[0-9]+[^)]*)\\) AppleWebKit/537\\.36");
                if (!token.Success) throw LaunchOptions.Reject("native_windows_ua_token_required");
                result.UserAgent = GeneratedUserAgent("Windows NT 10.0; Win64; x64", chromium, false);
                result.Hints["brands"] = keepBrands.ToArray(); result.Hints["fullVersionList"] = keepFull.ToArray();
            }
            if (identityMode == "manual") result.UserAgent = manualUA;
            return result;
        }
        internal static string GeneratedUserAgent(string osToken, string chromium, bool mobile)
        { Version version; if (!Version.TryParse(chromium, out version) || version.Revision < 0 || version.Major < 1) throw LaunchOptions.Reject("native_chromium_version_unknown"); return "Mozilla/5.0 (" + osToken + ") AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + version.Major.ToString(CultureInfo.InvariantCulture) + ".0.0.0" + (mobile ? " Mobile" : "") + " Safari/537.36"; }
        internal static void VerifyUserAgentHeader(string actual, string expected)
        { if (String.IsNullOrEmpty(expected) || actual != expected) throw LaunchOptions.Reject("ua_http_readback_failed"); }
        internal static void VerifyUserAgentHeaders(IDictionary<string, object> headers, string expected)
        {
            int matches = 0;
            foreach (var pair in headers) if (String.Equals(pair.Key, "User-Agent", StringComparison.OrdinalIgnoreCase)) { matches++; VerifyUserAgentHeader(pair.Value as string, expected); }
            if (matches != 1) throw LaunchOptions.Reject("ua_http_readback_failed");
        }
        internal static object Metrics(EnvironmentOptions options, int width, int height, double scale)
        { return new { width = width, height = height, deviceScaleFactor = options.DPR, mobile = options.Mobile, screenWidth = width, screenHeight = height, positionX = 0, positionY = 0, scale = scale, screenOrientation = new { type = width <= height ? "portraitPrimary" : "landscapePrimary", angle = 0 } }; }
        internal static double FitMobileScale(int width, int height, int availableWidth, int availableHeight)
        {
            if (availableWidth < 352 || availableHeight < 452) throw LaunchOptions.Reject("display_too_small");
            return Math.Min(1.0, Math.Min((availableHeight - 148 - 64.0) / height, (availableWidth - 32.0) / width));
        }
    }
}
