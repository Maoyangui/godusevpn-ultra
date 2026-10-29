package browserenginehost

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

// ChromeIdentity describes actual CDP overrides derived from this session's
// native observation. It is not a frontend-supplied UA-CH or browser version.
type ChromeIdentity struct {
	UserAgent         string
	NavigatorPlatform string
	ClientHints       browserhost.ClientHints
	Override          bool
	Mobile            bool
	DeviceScaleFactor float64
	MaxTouchPoints    int
}

func identityMode(c Config) string {
	if c.Identity.Mode == "" {
		return "native"
	}
	return c.Identity.Mode
}
func printableUA(value string, max int) bool {
	if len(value) == 0 || len(value) > max || strings.TrimSpace(value) == "" {
		return false
	}
	for _, b := range []byte(value) {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}
func selectedEnvironment(c Config) (*browserhost.EnvironmentPreset, error) {
	if c.Environment == nil {
		return nil, nil
	}
	if c.Environment.Mode == "native" {
		if c.Environment.OS != "" || c.Environment.DevicePresetID != "" || c.Environment.OSVersion != "" {
			return nil, ErrRefused
		}
		return nil, nil
	}
	preset, err := browserhost.EffectiveEnvironment(*c.Environment)
	if err != nil {
		return nil, ErrRefused
	}
	return &preset, nil
}
func validateIdentity(c Config) error {
	if c.Identity.ClientHints != nil {
		return ErrRefused
	}
	switch identityMode(c) {
	case "native", "generated":
		if c.Identity.UserAgent != "" {
			return ErrRefused
		}
	case "manual":
		if !printableUA(c.Identity.UserAgent, 1024) {
			return ErrRefused
		}
	default:
		return ErrRefused
	}
	_, err := selectedEnvironment(c)
	return err
}

var observedChromeUA = regexp.MustCompile(`(?:^| )Chrome/([0-9]+)\.[0-9]+\.[0-9]+\.[0-9]+(?: |$)`)
var numericPlatformVersion = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`)
var observedFirefoxUA = regexp.MustCompile(`(?:^| )Firefox/([0-9]+)\.[0-9]+(?:\.[0-9]+)?(?: |$)`)

func nativeChromeHints(raw json.RawMessage, c Config) (browserhost.ClientHints, error) {
	var hints browserhost.ClientHints
	if len(raw) == 0 || len(raw) > 16384 || uniqueProtocolJSON(raw) != nil {
		return hints, ErrRefused
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return hints, ErrRefused
	}
	allowed := map[string]bool{"brands": true, "fullVersionList": true, "platform": true, "platformVersion": true, "architecture": true, "bitness": true, "mobile": true, "model": true}
	if len(fields) != len(allowed) {
		return hints, ErrRefused
	}
	for name := range fields {
		if !allowed[name] {
			return hints, ErrRefused
		}
	}
	if json.Unmarshal(raw, &hints) != nil || hints.Platform != "Windows" || hints.Mobile || hints.Model != "" || hints.Bitness != "64" || hints.Architecture != "x86" && hints.Architecture != "arm" || !numericPlatformVersion.MatchString(hints.PlatformVersion) {
		return hints, ErrRefused
	}
	major := strings.SplitN(c.Version, ".", 2)[0]
	for _, list := range []struct {
		values []browserhost.BrandVersion
		full   bool
	}{{hints.Brands, false}, {hints.FullVersionList, true}} {
		if len(list.values) < 1 || len(list.values) > 8 {
			return hints, ErrRefused
		}
		seen := map[string]bool{}
		chromium := false
		for _, brand := range list.values {
			if !printableUA(brand.Brand, 128) || !printableUA(brand.Version, 32) || seen[brand.Brand] {
				return hints, ErrRefused
			}
			seen[brand.Brand] = true
			switch brand.Brand {
			case "Chromium", "Google Chrome":
				if list.full && brand.Version != c.Version || !list.full && brand.Version != major {
					return hints, ErrRefused
				}
				if brand.Brand == "Chromium" {
					chromium = true
				}
			default:
				if !strings.Contains(brand.Brand, "Not") {
					return hints, ErrRefused
				}
			}
		}
		if !chromium {
			return hints, ErrRefused
		}
	}
	return hints, nil
}
func ChromeIdentityPlan(c Config, nativeUA string, nativeCH json.RawMessage) (ChromeIdentity, error) {
	var plan ChromeIdentity
	if c.EngineID != browserengine.Chrome || !chromeVersion.MatchString(c.Version) || validateIdentity(c) != nil || !printableUA(nativeUA, 2048) {
		return plan, ErrRefused
	}
	versions := observedChromeUA.FindAllStringSubmatch(nativeUA, -1)
	if len(versions) != 1 || versions[0][1] != strings.SplitN(c.Version, ".", 2)[0] || strings.Contains(nativeUA, "Edg/") {
		return plan, ErrRefused
	}
	hints, err := nativeChromeHints(nativeCH, c)
	if err != nil {
		return plan, err
	}
	preset, err := selectedEnvironment(c)
	if err != nil {
		return plan, err
	}
	plan = ChromeIdentity{UserAgent: nativeUA, NavigatorPlatform: "Win32", ClientHints: hints, MaxTouchPoints: -1}
	if preset != nil {
		plan.NavigatorPlatform = preset.NavigatorPlatform
		plan.ClientHints.Platform = preset.Platform
		plan.ClientHints.PlatformVersion = preset.PlatformVersion
		plan.ClientHints.Architecture = preset.Architecture
		plan.ClientHints.Bitness = preset.Bitness
		plan.ClientHints.Mobile = preset.Mobile
		plan.ClientHints.Model = preset.Model
		plan.Mobile = preset.Mobile
		plan.DeviceScaleFactor = preset.DeviceScaleFactor
		plan.MaxTouchPoints = preset.MaxTouchPoints
	}
	mode := identityMode(c)
	plan.Override = mode != "native" || preset != nil
	if mode == "manual" {
		plan.UserAgent = c.Identity.UserAgent
	} else if mode == "generated" {
		plan.UserAgent, err = browserhost.GeneratedChromiumUserAgent(preset, c.Version, nativeUA)
		if err != nil {
			return ChromeIdentity{}, ErrRefused
		}
	}
	return plan, nil
}

func firefoxOSCPU(preset *browserhost.EnvironmentPreset) string {
	parts := strings.Split(preset.PlatformVersion, ".")
	switch preset.Config.OS {
	case "windows":
		// The environment's Windows version is a UA-CH API contract value,
		// not the Windows NT marketing version. Modern Gecko freezes NT10.0.
		return "Windows NT 10.0; Win64; x64"
	case "macos":
		return "Intel Mac OS X 10.15"
	case "android":
		return "Linux armv8l"
	case "ios":
		return "iPhone OS " + strings.Join(parts, "_") + " like Mac OS X"
	}
	return ""
}

func generatedFirefoxUA(version string, preset *browserhost.EnvironmentPreset) string {
	major := strings.SplitN(version, ".", 2)[0]
	token := "Windows NT 10.0; Win64; x64"
	gecko := "20100101"
	if preset != nil {
		switch preset.Config.OS {
		case "windows":
			token = "Windows NT 10.0; Win64; x64"
		case "macos":
			token = "Macintosh; Intel Mac OS X 10.15"
		case "android":
			token = "Android " + firefoxAndroidVersion(version, preset.PlatformVersion)
		case "ios":
			// This is an explicit iOS-style environment with the real Gecko
			// product token, never a retail FxiOS/WebKit or Safari identity.
			token = "iPhone; CPU iPhone OS " + strings.ReplaceAll(preset.PlatformVersion, ".", "_") + " like Mac OS X"
		}
		if preset.Mobile {
			token += "; Mobile"
			gecko = major + ".0"
		}
	}
	return "Mozilla/5.0 (" + token + "; rv:" + major + ".0) Gecko/" + gecko + " Firefox/" + major + ".0"
}

func firefoxAndroidVersion(version, osVersion string) string {
	parts := strings.Split(osVersion, ".")
	androidMajor, _ := strconv.Atoi(parts[0])
	if androidMajor < 10 {
		return "10"
	}
	firefoxMajor, _ := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	if firefoxMajor >= 158 {
		// The fixed 158 source strips the release suffix before applying the
		// Android >=10 floor; older supported versions retain a real suffix.
		return parts[0]
	}
	// EffectiveEnvironment pads OSVersion to three components. Those added
	// zeroes are not a device release suffix; preserve nonzero minor/patch.
	for len(parts) > 1 && parts[len(parts)-1] == "0" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, ".")
}

// FirefoxIdentityPlan keeps Firefox's observed version token (including its
// native version reduction), and never presents Gecko as Safari/WebKit. The
// browser's actual product version is separately checked through BiDi.
func FirefoxIdentityPlan(c Config, nativeUA string) (string, error) {
	if c.EngineID != browserengine.Firefox || !firefoxVersion.MatchString(c.Version) || validateIdentity(c) != nil || !printableUA(nativeUA, 2048) {
		return "", ErrRefused
	}
	versions := observedFirefoxUA.FindAllStringSubmatch(nativeUA, -1)
	if len(versions) != 1 || versions[0][1] != strings.SplitN(c.Version, ".", 2)[0] || !strings.Contains(nativeUA, " Gecko/") {
		return "", ErrRefused
	}
	preset, err := selectedEnvironment(c)
	if err != nil {
		return "", ErrRefused
	}
	if identityMode(c) == "manual" {
		return c.Identity.UserAgent, nil
	}
	if identityMode(c) == "native" || preset == nil {
		return nativeUA, nil
	}
	return generatedFirefoxUA(c.Version, preset), nil
}

// PreviewIdentity is a pure draft for the unified form. The selected official
// catalog version is not a live runtime observation, and Applied stays false.
func PreviewIdentity(c Config) (browserhost.IdentityPreview, error) {
	p := browserhost.IdentityPreview{Mode: identityMode(c), Status: "planned", EngineID: c.EngineID, RuntimeProductVersion: c.Version, VersionSource: "selected-official-version-not-running", RequiresRuntimeReadback: true, NavigatorPlatform: "Win32", ClientHintsSource: "live-native-runtime-required", Warnings: []string{"浏览器环境模拟不会改变真实操作系统、硬件或内核；启动必须完成实际读回。"}}
	if validateIdentity(c) != nil || c.EngineID != browserengine.Chrome && c.EngineID != browserengine.Firefox || c.EngineID == browserengine.Chrome && !chromeVersion.MatchString(c.Version) || c.EngineID == browserengine.Firefox && !firefoxVersion.MatchString(c.Version) {
		return p, ErrRefused
	}
	preset, err := selectedEnvironment(c)
	if err != nil {
		return p, err
	}
	if preset != nil {
		p.NavigatorPlatform = preset.NavigatorPlatform
		if identityMode(c) == "native" {
			p.Warnings = append(p.Warnings, "保留实际原生UA；所选环境仍独立覆盖平台、Client Hints、窗口和触摸，可能与原生UA描述不同。")
		}
		if preset.Config.OS == "ios" {
			p.Warnings = append(p.Warnings, "iOS为合成网页环境；保留实际Chrome或Firefox内核标识，不是Safari或零售iOS浏览器的默认UA。")
		}
	}
	if c.EngineID == browserengine.Firefox {
		p.Warnings = append(p.Warnings, "Firefox使用真实Gecko；UA通过该版本BiDi命令覆盖，平台/触摸仅在本次独立profile使用官方原生偏好并逐frame读回，不提供Chromium Client Hints。")
	}
	if identityMode(c) == "manual" {
		p.UserAgent = c.Identity.UserAgent
		p.Warnings = append(p.Warnings, "手动UA逐字保留；UA文本与真实内核或环境不一致时不会推导或伪造Client Hints。")
	} else if identityMode(c) == "native" {
		p.UserAgentTemplate = "{NativeUserAgent}"
	} else if c.EngineID == browserengine.Chrome {
		p.UserAgent, err = browserhost.GeneratedChromiumUserAgent(preset, c.Version, "")
		if err != nil {
			return p, ErrRefused
		}
	} else {
		p.UserAgent = generatedFirefoxUA(c.Version, preset)
		p.Warnings = append(p.Warnings, "Firefox环境/UA覆盖需要该版本真实BiDi能力；没有对应能力时明确拒绝，不使用脚本冒充。")
	}
	return p, nil
}
