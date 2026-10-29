package browserenginehost

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

func TestChromeGeneratedUAUsesSharedBuilderWithoutChangingRealCHVersions(t *testing.T) {
	for _, original := range browserhost.EnvironmentPresets() {
		c := identityConfig()
		environment := original.Config
		if environment.OS == "windows" {
			environment.OSVersion = "13.0.0" // UA-CH contract, never NT13.
		}
		c.Environment = &environment
		preset, err := selectedEnvironment(c)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := browserhost.GeneratedChromiumUserAgent(preset, c.Version, fixtureNativeUA)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := ChromeIdentityPlan(c, fixtureNativeUA, json.RawMessage(fixtureNativeCH))
		if err != nil || plan.UserAgent != expected || !strings.Contains(plan.UserAgent, "Chrome/156.0.0.0") || strings.Contains(plan.UserAgent, c.Version) || plan.ClientHints.FullVersionList[1].Version != c.Version || plan.ClientHints.FullVersionList[2].Version != c.Version || plan.ClientHints.PlatformVersion != preset.PlatformVersion {
			t.Fatal("native path drifted from shared UA builder or invented a CH version", environment.OS, err)
		}
		preview, err := PreviewIdentity(c)
		if err != nil || preview.UserAgent != expected || preview.RuntimeProductVersion != c.Version || preview.Applied || preview.ExactChromiumVersionKnown {
			t.Fatal("preview/runtime identity policy differs", environment.OS, err)
		}
		if environment.OS == "windows" && (!strings.Contains(plan.UserAgent, "Windows NT 10.0;") || strings.Contains(plan.UserAgent, "Windows NT 13")) {
			t.Fatal("CH platform contract became an NT marketing version")
		}
	}
}

func TestNativeUARemainsExactWithFourIndependentEnvironmentOverrides(t *testing.T) {
	for _, engine := range []string{browserengine.Chrome, browserengine.Firefox} {
		for _, preset := range browserhost.EnvironmentPresets() {
			t.Run(engine+"/"+preset.Config.OS, func(t *testing.T) {
				c := identityConfig()
				c.EngineID = engine
				c.Identity.Mode = "native"
				environment := preset.Config
				c.Environment = &environment
				if engine == browserengine.Chrome {
					plan, err := ChromeIdentityPlan(c, fixtureNativeUA, json.RawMessage(fixtureNativeCH))
					if err != nil || plan.UserAgent != fixtureNativeUA || !plan.Override || plan.ClientHints.Platform != preset.Platform || plan.NavigatorPlatform != preset.NavigatorPlatform {
						t.Fatal("native UA or independent environment changed", err)
					}
				} else {
					c.Version = "158.0.1"
					const native = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:158.0) Gecko/20100101 Firefox/158.0"
					ua, err := FirefoxIdentityPlan(c, native)
					if err != nil || ua != native {
						t.Fatal("native Firefox UA changed", err)
					}
				}
				preview, err := PreviewIdentity(c)
				if err != nil || preview.UserAgent != "" || preview.UserAgentTemplate != "{NativeUserAgent}" || !strings.Contains(strings.Join(preview.Warnings, " "), "保留实际原生UA") {
					t.Fatal("native preview claims a generated UA or hides independent environment", err)
				}
			})
		}
	}
}

func TestFirefoxGeneratedUAHasVersionCorrectPlatformAndGeckoTokens(t *testing.T) {
	for _, test := range []struct {
		name, version, os, preset, osVersion, token, gecko string
	}{
		{"windows-contract", "158.0.1", "windows", "windows-desktop", "13.0.0", "Windows NT 10.0; Win64; x64", "20100101"},
		{"macos-capped", "158.0.1", "macos", "macos-desktop", "23.4.5", "Macintosh; Intel Mac OS X 10.15", "20100101"},
		{"android-old-padded", "147.0.1", "android", "pixel-7", "13.0.0", "Android 13; Mobile", "147.0"},
		{"android-old-minor", "147.0.1", "android", "pixel-7", "13.2.0", "Android 13.2; Mobile", "147.0"},
		{"android-old-patch", "157.0.1", "android", "pixel-7", "13.0.1", "Android 13.0.1; Mobile", "157.0"},
		{"android-modern-strips-minor", "158.0.1", "android", "pixel-7", "13.2.4", "Android 13; Mobile", "158.0"},
		{"android-modern-floor", "158.0.1", "android", "pixel-7", "9.3.2", "Android 10; Mobile", "158.0"},
		{"android-old-floor", "147.0.1", "android", "pixel-7", "9.3.2", "Android 10; Mobile", "147.0"},
		{"ios-explicit-synthetic", "158.0.1", "ios", "iphone-14", "16.0.0", "iPhone; CPU iPhone OS 16_0_0 like Mac OS X; Mobile", "158.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := identityConfig()
			c.EngineID = browserengine.Firefox
			c.Version = test.version
			c.Environment = &browserhost.EnvironmentConfig{Mode: "browser-emulation", OS: test.os, DevicePresetID: test.preset, OSVersion: test.osVersion}
			major := strings.SplitN(c.Version, ".", 2)[0]
			native := fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:%s.0) Gecko/20100101 Firefox/%s.0", major, major)
			expected := fmt.Sprintf("Mozilla/5.0 (%s; rv:%s.0) Gecko/%s Firefox/%s.0", test.token, major, test.gecko, major)
			ua, err := FirefoxIdentityPlan(c, native)
			if err != nil || ua != expected || strings.Contains(ua, "Safari/") || strings.Contains(ua, "FxiOS/") || strings.Contains(ua, "Chrome/") {
				t.Fatal("wrong Firefox platform, mobile or engine version", err, ua)
			}
			preview, err := PreviewIdentity(c)
			if err != nil || preview.UserAgent != expected || preview.RuntimeProductVersion != c.Version || preview.Applied {
				t.Fatal("Firefox preview/runtime policy differs", err)
			}
			if test.os == "ios" && !strings.Contains(strings.Join(preview.Warnings, " "), "合成网页环境") {
				t.Fatal("synthetic iOS UA presented as a retail native browser")
			}
		})
	}
}

func TestFirefoxNativePreferencesDoNotTurnCHContractsIntoNTOrModernMacVersion(t *testing.T) {
	for _, test := range []struct{ os, preset, version, expected string }{
		{"windows", "windows-desktop", "13.0.0", "Windows NT 10.0; Win64; x64"},
		{"macos", "macos-desktop", "23.4.5", "Intel Mac OS X 10.15"},
	} {
		c := identityConfig()
		c.EngineID, c.Version = browserengine.Firefox, "158.0.1"
		c.Environment = &browserhost.EnvironmentConfig{Mode: "browser-emulation", OS: test.os, DevicePresetID: test.preset, OSVersion: test.version}
		preset, err := selectedEnvironment(c)
		if err != nil || firefoxOSCPU(preset) != test.expected {
			t.Fatal("wrong native OSCPU preference", test.os, err)
		}
		encoded, _ := json.Marshal(test.expected)
		if !strings.Contains(string(firefoxPreferences(c, 43123)), `user_pref("general.oscpu.override", `+string(encoded)+");") {
			t.Fatal("native preference does not use the normalized OSCPU", test.os)
		}
	}
}
