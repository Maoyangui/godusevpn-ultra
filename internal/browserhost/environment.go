package browserhost

import (
	"regexp"
	"strings"
)

// EnvironmentConfig requests browser-visible emulation, never a different OS,
// engine, physical device or arbitrary UA/runtime version supplied by the UI.
type EnvironmentConfig struct {
	Mode           string `json:"mode"`
	OS             string `json:"os,omitempty"`
	DevicePresetID string `json:"devicePresetId,omitempty"`
	OSVersion      string `json:"osVersion,omitempty"`
}
type EnvironmentPreset struct {
	Config              EnvironmentConfig `json:"config"`
	NavigatorPlatform   string            `json:"navigatorPlatform"`
	Platform            string            `json:"platform"`
	PlatformVersion     string            `json:"platformVersion"`
	Architecture        string            `json:"architecture"`
	Bitness             string            `json:"bitness"`
	Mobile              bool              `json:"mobile"`
	DeviceScaleFactor   float64           `json:"deviceScaleFactor"`
	MaxTouchPoints      int               `json:"maxTouchPoints"`
	RecommendedViewport ViewportConfig    `json:"recommendedViewport"`
	Model               string            `json:"model,omitempty"`
}

var environmentPresets = []EnvironmentPreset{
	{EnvironmentConfig{"browser-emulation", "windows", "windows-desktop", "10.0.0"}, "Win32", "Windows", "10.0.0", "x86", "64", false, 1, 0, ViewportConfig{1280, 720}, ""},
	{EnvironmentConfig{"browser-emulation", "macos", "macos-desktop", "13.0.0"}, "MacIntel", "macOS", "13.0.0", "x86", "64", false, 1, 0, ViewportConfig{1280, 720}, ""},
	{EnvironmentConfig{"browser-emulation", "android", "pixel-7", "13.0.0"}, "Linux armv8l", "Android", "13.0.0", "arm", "64", true, 2.625, 5, ViewportConfig{412, 915}, "Pixel 7"},
	{EnvironmentConfig{"browser-emulation", "ios", "iphone-14", "16.0.0"}, "iPhone", "iOS", "16.0.0", "arm", "64", true, 3, 5, ViewportConfig{390, 844}, "iPhone"},
}

func EnvironmentPresets() []EnvironmentPreset {
	return append([]EnvironmentPreset(nil), environmentPresets...)
}

var osVersionPattern = regexp.MustCompile(`^(?:0|[1-9][0-9]{0,2})(?:\.(?:0|[1-9][0-9]{0,2})){0,2}$`)

func normalizedOSVersion(version string) (string, bool) {
	if !osVersionPattern.MatchString(version) {
		return "", false
	}
	parts := strings.Split(version, ".")
	if parts[0] == "0" {
		return "", false
	}
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, "."), true
}

// EffectiveEnvironment resolves only a known paired preset; it does not apply
// recommended window sizes to a user's existing viewport or draft.
func EffectiveEnvironment(config EnvironmentConfig) (EnvironmentPreset, error) {
	if config.Mode != "browser-emulation" {
		return EnvironmentPreset{}, invalid("environment.mode", "emulation_required")
	}
	for _, preset := range environmentPresets {
		if config.OS == preset.Config.OS && config.DevicePresetID == preset.Config.DevicePresetID {
			if config.OSVersion != "" {
				version, ok := normalizedOSVersion(config.OSVersion)
				if !ok {
					return EnvironmentPreset{}, invalid("environment.osVersion", "numeric_version_1_to_3_segments_required")
				}
				preset.Config.OSVersion = version
				preset.PlatformVersion = version
			}
			return preset, nil
		}
	}
	return EnvironmentPreset{}, invalid("environment", "unknown_or_inconsistent_device_preset")
}
func validateEnvironment(config ProfileConfig) error {
	e := config.Environment
	if e == nil {
		return nil
	}
	if e.Mode == "native" {
		if e.OS != "" || e.DevicePresetID != "" || e.OSVersion != "" {
			return invalid("environment", "native_must_not_override")
		}
		return nil
	}
	if config.EngineID != "webview2" {
		return invalid("environment", "requires_verified_webview2_host")
	}
	if config.Identity.Mode == "observed-exact" {
		return invalid("environment", "identity_must_be_host_derived")
	}
	_, err := EffectiveEnvironment(*e)
	return err
}
func environmentMobile(config ProfileConfig) bool {
	if config.Environment == nil {
		return false
	}
	preset, err := EffectiveEnvironment(*config.Environment)
	return err == nil && preset.Mobile
}

type EnvironmentEnforcementRequirements struct {
	BeforeNavigation              bool              `json:"beforeNavigation"`
	ActualChromiumVersionRequired bool              `json:"actualChromiumVersionRequired"`
	Contexts                      []string          `json:"contexts"`
	WorkersBlockedBeforeRun       bool              `json:"workersBlockedBeforeRun"`
	ServiceWorkerBlocked          bool              `json:"serviceWorkerBlocked"`
	CloseOnReadbackMismatch       bool              `json:"closeOnReadbackMismatch"`
	ChangesOperatingSystem        bool              `json:"changesOperatingSystem"`
	ChangesBrowserEngine          bool              `json:"changesBrowserEngine"`
	Preset                        EnvironmentPreset `json:"preset"`
}

func EnvironmentRequirements(config ProfileConfig) *EnvironmentEnforcementRequirements {
	if config.Environment == nil || config.Environment.Mode == "native" {
		return nil
	}
	preset, err := EffectiveEnvironment(*config.Environment)
	if err != nil {
		return nil
	}
	return &EnvironmentEnforcementRequirements{BeforeNavigation: true, ActualChromiumVersionRequired: true, Contexts: []string{"main-frame", "same-origin-frame", "cross-origin-frame"}, WorkersBlockedBeforeRun: true, ServiceWorkerBlocked: true, CloseOnReadbackMismatch: true, Preset: preset}
}

type EnvironmentIdentity struct {
	UserAgent         string      `json:"userAgent"`
	NavigatorPlatform string      `json:"navigatorPlatform"`
	ClientHints       ClientHints `json:"clientHints"`
	MobileMetrics     bool        `json:"mobileMetrics"`
	DeviceScaleFactor float64     `json:"deviceScaleFactor"`
	MaxTouchPoints    int         `json:"maxTouchPoints"`
}

var nativeChromeVersion = regexp.MustCompile(`(?:^| )Chrome/([1-9][0-9]{0,3})\.[0-9]+\.[0-9]+\.[0-9]+(?: |$)`)

func chromeMajor(version string) string { return strings.SplitN(version, ".", 2)[0] }

// ExpectedEnvironmentIdentity requires an independently observed live native
// identity. Edge's full product version is not Chromium's full version. The
// latter comes from the native Chromium CH brand, with all majors cross-checked.
func ExpectedEnvironmentIdentity(config ProfileConfig, runtime RuntimeContext) (EnvironmentIdentity, error) {
	if err := ValidateProfile(config, runtime); err != nil {
		return EnvironmentIdentity{}, err
	}
	if config.Environment == nil {
		return EnvironmentIdentity{}, invalid("environmentReadback", "no_override_requested")
	}
	preset, err := EffectiveEnvironment(*config.Environment)
	if err != nil {
		return EnvironmentIdentity{}, err
	}
	o := runtime.Identity
	if o == nil || o.RuntimeVersion != runtime.Engine.Version || !versionPattern.MatchString(runtime.Engine.Version) {
		return EnvironmentIdentity{}, invalid("environmentReadback", "live_native_identity_required")
	}
	if !safeHeader(o.UserAgent, 2048) || o.ClientHints.Platform != "Windows" || o.ClientHints.Mobile ||
		len(o.ClientHints.Brands) < 1 || len(o.ClientHints.Brands) > 8 || len(o.ClientHints.FullVersionList) < 1 || len(o.ClientHints.FullVersionList) > 8 {
		return EnvironmentIdentity{}, invalid("environmentReadback", "native_windows_identity_required")
	}
	matches := nativeChromeVersion.FindAllStringSubmatch(o.UserAgent, -1)
	version := ""
	count := 0
	for _, brand := range o.ClientHints.FullVersionList {
		if brand.Brand == "Chromium" {
			version = brand.Version
			count++
		}
	}
	if count != 1 || !versionPattern.MatchString(version) || len(matches) != 1 || matches[0][1] != chromeMajor(version) || chromeMajor(version) != chromeMajor(runtime.Engine.Version) {
		return EnvironmentIdentity{}, invalid("environmentReadback", "native_chromium_version_mismatch")
	}
	hints := o.ClientHints
	hints.Brands = nil
	hints.FullVersionList = nil
	for _, pair := range []struct {
		input []BrandVersion
		full  bool
	}{{o.ClientHints.Brands, false}, {o.ClientHints.FullVersionList, true}} {
		seen := map[string]bool{}
		chromium := false
		for _, brand := range pair.input {
			if !safeHeader(brand.Brand, 128) || !safeHeader(brand.Version, 32) || seen[brand.Brand] {
				return EnvironmentIdentity{}, invalid("environmentReadback", "invalid_native_brand_list")
			}
			seen[brand.Brand] = true
			if brand.Brand == "Microsoft Edge" {
				continue
			}
			if brand.Brand == "Chromium" {
				chromium = true
				if pair.full && brand.Version != version || !pair.full && brand.Version != chromeMajor(version) {
					return EnvironmentIdentity{}, invalid("environmentReadback", "native_brand_version_mismatch")
				}
			} else if !strings.Contains(brand.Brand, "Not") {
				return EnvironmentIdentity{}, invalid("environmentReadback", "unknown_native_brand")
			}
			if pair.full {
				hints.FullVersionList = append(hints.FullVersionList, brand)
			} else {
				hints.Brands = append(hints.Brands, brand)
			}
		}
		if !chromium {
			return EnvironmentIdentity{}, invalid("environmentReadback", "chromium_brand_missing")
		}
	}
	hints.Platform = preset.Platform
	hints.PlatformVersion = preset.PlatformVersion
	hints.Architecture = preset.Architecture
	hints.Bitness = preset.Bitness
	hints.Mobile = preset.Mobile
	hints.Model = preset.Model
	ua := o.UserAgent
	if config.Identity.Mode == "manual" {
		ua = config.Identity.UserAgent
	}
	if config.Identity.Mode == "generated" {
		ua, err = GeneratedChromiumUserAgent(&preset, version, o.UserAgent)
		if err != nil {
			return EnvironmentIdentity{}, err
		}
	}
	return EnvironmentIdentity{ua, preset.NavigatorPlatform, hints, preset.Mobile, preset.DeviceScaleFactor, preset.MaxTouchPoints}, nil
}

type EnvironmentContextReadback struct {
	Context  string              `json:"context"`
	Identity EnvironmentIdentity `json:"identity"`
	Viewport ViewportConfig      `json:"viewport"`
}
type EnvironmentReadback struct {
	ProfileID                        string                       `json:"profileId"`
	EngineID                         string                       `json:"engineId"`
	RuntimeVersion                   string                       `json:"runtimeVersion"`
	BeforeNavigation                 bool                         `json:"beforeNavigation"`
	WorkersBlockedBeforeRun          bool                         `json:"workersBlockedBeforeRun"`
	ServiceWorkerRegistrationBlocked bool                         `json:"serviceWorkerRegistrationBlocked"`
	Contexts                         []EnvironmentContextReadback `json:"contexts"`
}

func ValidateEnvironmentReadback(config ProfileConfig, runtime RuntimeContext, readback EnvironmentReadback) error {
	expected, err := ExpectedEnvironmentIdentity(config, runtime)
	if err != nil {
		return err
	}
	requirements := EnvironmentRequirements(config)
	if readback.ProfileID != config.ProfileID || readback.EngineID != config.EngineID || readback.RuntimeVersion != runtime.Engine.Version || !readback.BeforeNavigation || !readback.WorkersBlockedBeforeRun || !readback.ServiceWorkerRegistrationBlocked {
		return invalid("environmentReadback", "identity_or_pre_navigation_coverage_missing")
	}
	if len(readback.Contexts) != len(requirements.Contexts) {
		return invalid("environmentReadback.contexts", "incomplete_context_coverage")
	}
	seen := map[string]bool{}
	for _, context := range readback.Contexts {
		valid := false
		for _, name := range requirements.Contexts {
			if name == context.Context {
				valid = true
			}
		}
		if !valid || seen[context.Context] || !sameJSON(context.Identity, expected) || context.Viewport != config.Viewport {
			return invalid("environmentReadback.contexts", "duplicate_or_inconsistent_readback")
		}
		seen[context.Context] = true
	}
	return nil
}
