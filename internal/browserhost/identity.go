package browserhost

import (
	"regexp"
	"strings"
)

type IdentityPreview struct {
	Mode                      string   `json:"mode"`
	Status                    string   `json:"status"`
	UserAgent                 string   `json:"userAgent,omitempty"`
	UserAgentTemplate         string   `json:"userAgentTemplate,omitempty"`
	EngineID                  string   `json:"engineId"`
	RuntimeProductVersion     string   `json:"runtimeProductVersion,omitempty"`
	ChromiumVersion           string   `json:"chromiumVersion,omitempty"`
	VersionSource             string   `json:"versionSource"`
	ExactChromiumVersionKnown bool     `json:"exactChromiumVersionKnown"`
	RequiresRuntimeReadback   bool     `json:"requiresRuntimeReadback"`
	Applied                   bool     `json:"applied"`
	NavigatorPlatform         string   `json:"navigatorPlatform"`
	ClientHintsSource         string   `json:"clientHintsSource"`
	Warnings                  []string `json:"warnings"`
}

func validManualUserAgent(value string) bool {
	if len(value) == 0 || len(value) > 1024 || strings.TrimSpace(value) == "" {
		return false
	}
	for _, b := range []byte(value) {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

var nativeWindowsToken = regexp.MustCompile(`^Mozilla/5\.0 \((Windows NT [0-9]+\.[0-9]+[^)]*)\) AppleWebKit/537\.36`)

// GeneratedChromiumUserAgent follows the modern reduced Chromium UA format.
// A full trusted engine version supplies only its major; its full version and
// the selected detailed OS/model belong in independently validated Client Hints.
// The one fixed placeholder is for unknown-runtime previews, never live trust.
// An iOS choice remains an explicitly synthetic Chromium identity on Windows.
func GeneratedChromiumUserAgent(preset *EnvironmentPreset, version, nativeUA string) (string, error) {
	major := "{ChromiumMajor}"
	if version != major {
		if !versionPattern.MatchString(version) {
			return "", invalid("identity", "chromium_version_required")
		}
		major = chromeMajor(version)
	}
	token := "Windows NT 10.0; Win64; x64"
	mobile := false
	if preset != nil {
		effective, err := EffectiveEnvironment(preset.Config)
		if err != nil {
			return "", err
		}
		mobile = effective.Mobile
		switch effective.Config.OS {
		case "windows":
			token = "Windows NT 10.0; Win64; x64"
		case "macos":
			token = "Macintosh; Intel Mac OS X 10_15_7"
		case "android":
			token = "Linux; Android 10; K"
		case "ios":
			token = "iPhone; CPU iPhone OS " + strings.ReplaceAll(effective.PlatformVersion, ".", "_") + " like Mac OS X"
		default:
			return "", invalid("identity", "unsupported_environment")
		}
	} else if nativeUA != "" {
		match := nativeWindowsToken.FindStringSubmatch(nativeUA)
		if len(match) != 2 {
			return "", invalid("identity", "native_windows_ua_token_required")
		}
	}
	ua := "Mozilla/5.0 (" + token + ") AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + major + ".0.0.0"
	if mobile {
		ua += " Mobile"
	}
	return ua + " Safari/537.36", nil
}

func observedChromiumVersion(runtime RuntimeContext) (string, error) {
	o := runtime.Identity
	if o == nil {
		return "", invalid("identity", "independent_runtime_identity_required")
	}
	candidate := ProfileIdentity{Mode: "observed-exact", UserAgent: o.UserAgent, ClientHints: &o.ClientHints}
	if err := validateIdentity(candidate, runtime); err != nil {
		return "", err
	}
	version := ""
	count := 0
	for _, b := range o.ClientHints.FullVersionList {
		if b.Brand == "Chromium" {
			version = b.Version
			count++
		}
	}
	ua := nativeChromeVersion.FindAllStringSubmatch(o.UserAgent, -1)
	if count != 1 || !versionPattern.MatchString(version) || chromeMajor(version) != chromeMajor(runtime.Engine.Version) || len(ua) != 1 || ua[0][1] != chromeMajor(version) {
		return "", invalid("identity", "native_chromium_version_mismatch")
	}
	return version, nil
}

// PreviewIdentity uses only trusted caller-supplied inventory/observation. A
// registry Edge product version supplies a major, never Chromium's full build.
// No runtime is started and every returned UA remains a pre-navigation plan.
func PreviewIdentity(config ProfileConfig, runtime RuntimeContext) (IdentityPreview, error) {
	if err := ValidateProfile(config, runtime); err != nil {
		return IdentityPreview{}, err
	}
	p := IdentityPreview{Mode: config.Identity.Mode, Status: "planned", EngineID: config.EngineID, VersionSource: "unavailable", RequiresRuntimeReadback: true, NavigatorPlatform: "Win32", ClientHintsSource: "live-native-runtime", Warnings: []string{"Plan only: startup must verify HTTP/navigator UA and live Client Hints before navigation."}}
	var preset *EnvironmentPreset
	if config.Environment != nil && config.Environment.Mode == "browser-emulation" {
		e, err := EffectiveEnvironment(*config.Environment)
		if err != nil {
			return p, err
		}
		preset = &e
		p.NavigatorPlatform = e.NavigatorPlatform
		p.ClientHintsSource = "live-native-runtime-with-selected-environment"
	}
	version := "{ChromiumMajor}"
	if versionPattern.MatchString(runtime.Engine.Version) {
		p.RuntimeProductVersion = runtime.Engine.Version
		p.VersionSource = "installed-runtime-product-major"
		version = chromeMajor(runtime.Engine.Version) + ".0.0.0"
		p.Warnings = append(p.Warnings, "Reduced UA preview uses the installed product major; Chromium full version is not observed until startup.")
	}
	nativeUA := ""
	if runtime.Identity != nil {
		v, err := observedChromiumVersion(runtime)
		if err != nil {
			// Legacy exact snapshots could contain only the Edge product brand.
			// Preserve their known UA without inventing a Chromium full version.
			count := 0
			for _, brand := range runtime.Identity.ClientHints.FullVersionList {
				if brand.Brand == "Chromium" {
					count++
				}
			}
			legacy := preset == nil && (config.Identity.Mode == "native" || config.Identity.Mode == "observed-exact") && count == 0
			candidate := ProfileIdentity{Mode: "observed-exact", UserAgent: runtime.Identity.UserAgent, ClientHints: &runtime.Identity.ClientHints}
			if !legacy || validateIdentity(candidate, runtime) != nil {
				return p, err
			}
			p.Warnings = append(p.Warnings, "Legacy native snapshot preserves its observed UA but does not establish Chromium's full version.")
		} else {
			version = v
			p.ChromiumVersion = v
			p.VersionSource = "live-native-client-hints"
			p.ExactChromiumVersionKnown = true
		}
		nativeUA = runtime.Identity.UserAgent
	}
	switch config.Identity.Mode {
	case "manual":
		p.UserAgent = config.Identity.UserAgent
		p.Warnings = append(p.Warnings, "Manual text is preserved exactly; it may disagree with engine/platform tokens. Client Hints are never inferred from this text.")
		if p.UserAgent != strings.TrimSpace(p.UserAgent) {
			p.Warnings = append(p.Warnings, "Manual UA has leading/trailing spaces; if HTTP normalizes them, startup must reject rather than silently rewrite the text.")
		}
		if matches := nativeChromeVersion.FindAllStringSubmatch(p.UserAgent, -1); len(matches) == 1 && p.RuntimeProductVersion != "" && matches[0][1] != chromeMajor(p.RuntimeProductVersion) {
			p.Warnings = append(p.Warnings, "Manual UA Chrome major differs from the installed runtime; live Client Hints will keep the actual engine version.")
		}
		manualOS := ""
		for _, pair := range []struct{ token, os string }{{"Windows NT ", "windows"}, {"Macintosh;", "macos"}, {"Android ", "android"}, {"iPhone;", "ios"}} {
			if strings.Contains(p.UserAgent, pair.token) {
				manualOS = pair.os
			}
		}
		expectedOS := "windows"
		if preset != nil {
			expectedOS = preset.Config.OS
		}
		if manualOS != "" && manualOS != expectedOS {
			p.Warnings = append(p.Warnings, "Manual UA OS token differs from the selected environment; Client Hints/platform/mobile metrics keep the selected environment.")
		}
	case "observed-exact":
		p.UserAgent = config.Identity.UserAgent
	case "native":
		if nativeUA != "" {
			p.UserAgent = nativeUA
		} else {
			p.UserAgentTemplate = "{NativeUserAgent}"
		}
		if preset != nil {
			p.Warnings = append(p.Warnings, "Native UA text is retained even with a selected environment. Platform, Client Hints, viewport and touch emulation are independent and may differ from that UA.")
		}
	case "generated":
		ua, err := GeneratedChromiumUserAgent(preset, version, nativeUA)
		if err != nil {
			return p, err
		}
		if p.VersionSource == "unavailable" {
			p.UserAgentTemplate = ua
		} else {
			p.UserAgent = ua
		}
		p.Warnings = append(p.Warnings, "Generated Chromium UA uses reduced fixed platform tokens and major.0.0.0; detailed requested platform/model and actual full engine versions remain in Client Hints.")
	}
	if preset != nil && preset.Config.OS == "ios" {
		p.Warnings = append(p.Warnings, "The iOS environment UA is a synthetic Chromium iPhone profile on Windows, not a retail iOS CriOS browser, native Safari, or WebKit 605.")
	}
	return p, nil
}
