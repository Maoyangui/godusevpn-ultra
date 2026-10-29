package browserhost

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGeneratedChromiumSharedGoldenFormat(t *testing.T) {
	var fixture struct {
		FormatVersion int                                                                        `json:"formatVersion"`
		Cases         []struct{ Name, OS, DevicePresetID, OSVersion, Version, UserAgent string } `json:"cases"`
	}
	data, err := os.ReadFile("testdata/chromium-ua-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil || fixture.FormatVersion != 1 || len(fixture.Cases) != 9 {
		t.Fatal("invalid shared golden fixture", err)
	}
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			var preset *EnvironmentPreset
			if test.OS != "" {
				value, err := EffectiveEnvironment(EnvironmentConfig{Mode: "browser-emulation", OS: test.OS, DevicePresetID: test.DevicePresetID, OSVersion: test.OSVersion})
				if err != nil {
					t.Fatal(err)
				}
				preset = &value
			}
			actual, err := GeneratedChromiumUserAgent(preset, test.Version, "")
			if err != nil || actual != test.UserAgent {
				t.Fatalf("golden mismatch: %s %v", actual, err)
			}
			if strings.Contains(actual, test.Version) || strings.Contains(actual, "CriOS/") || strings.Contains(actual, "FxiOS/") || strings.Contains(actual, "Version/") {
				t.Fatal("generated UA leaked full version or claimed another actual engine")
			}
		})
	}
	for _, version := range []string{"", "149", "0.0.0.0", "149.0.1.2\r\nX: y", "{ChromiumFullVersion}", "149.0.0.0 extra"} {
		if _, err = GeneratedChromiumUserAgent(nil, version, ""); err == nil {
			t.Fatal("unsafe version accepted")
		}
	}
	if ua, err := GeneratedChromiumUserAgent(nil, "{ChromiumMajor}", ""); err != nil || !strings.Contains(ua, "Chrome/{ChromiumMajor}.0.0.0") {
		t.Fatal("unknown major fabricated")
	}
	legacyWindows := "Mozilla/5.0 (Windows NT 6.3; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
	if ua, err := GeneratedChromiumUserAgent(nil, "140.0.7339.90", legacyWindows); err != nil || !strings.Contains(ua, "Windows NT 10.0;") {
		t.Fatal("generated Windows token did not stay reduced")
	}
}

func TestNativeUAIsVerbatimWithEveryIndependentEnvironment(t *testing.T) {
	for _, preset := range EnvironmentPresets() {
		config, runtime := environmentFixture(t)
		config.Environment = &preset.Config
		config.Viewport = preset.RecommendedViewport
		config.Identity = ProfileIdentity{Mode: "native"}
		preview, err := PreviewIdentity(config, runtime)
		if err != nil || preview.UserAgent != runtime.Identity.UserAgent || !strings.Contains(strings.Join(preview.Warnings, " "), "Native UA text is retained") {
			t.Fatal("native UA rewritten or environment difference hidden", err)
		}
		expected, err := ExpectedEnvironmentIdentity(config, runtime)
		if err != nil || expected.UserAgent != runtime.Identity.UserAgent || expected.NavigatorPlatform != preset.NavigatorPlatform || expected.ClientHints.PlatformVersion != preset.PlatformVersion || expected.ClientHints.Model != preset.Model {
			t.Fatal("native UA preservation changed independent environment", err)
		}
		full := ""
		for _, brand := range expected.ClientHints.FullVersionList {
			if brand.Brand == "Chromium" {
				full = brand.Version
			}
		}
		if full != "130.0.6723.58" {
			t.Fatal("actual CH full engine version reduced")
		}
		runtime.Identity = nil
		planned, err := PreviewIdentity(config, runtime)
		if err != nil || planned.UserAgent != "" || planned.UserAgentTemplate != "{NativeUserAgent}" {
			t.Fatal("native inventory preview invented observed text", err)
		}
	}
}

func TestWindowsPlatformVersionRemainsExplicitApiContract(t *testing.T) {
	config, runtime := environmentFixture(t)
	config.Identity.Mode = "generated"
	config.Environment = &EnvironmentConfig{Mode: "browser-emulation", OS: "windows", DevicePresetID: "windows-desktop", OSVersion: "11.0"}
	config.Viewport = ViewportConfig{1280, 720}
	expected, err := ExpectedEnvironmentIdentity(config, runtime)
	if err != nil || expected.ClientHints.PlatformVersion != "11.0.0" || !strings.Contains(expected.UserAgent, "Windows NT 10.0;") || config.Environment.OSVersion != "11.0" {
		t.Fatal("saved explicit platform contract silently reinterpreted", err)
	}
}
