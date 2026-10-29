package browserenginehost

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

const fixtureNativeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"
const fixtureNativeCH = `{"brands":[{"brand":"Not=A?Brand","version":"99"},{"brand":"Chromium","version":"156"},{"brand":"Google Chrome","version":"156"}],"fullVersionList":[{"brand":"Not=A?Brand","version":"99.0.0.0"},{"brand":"Chromium","version":"156.0.8078.47"},{"brand":"Google Chrome","version":"156.0.8078.47"}],"platform":"Windows","platformVersion":"15.0.0","architecture":"x86","bitness":"64","mobile":false,"model":""}`

func identityConfig() Config {
	return Config{EngineID: browserengine.Chrome, Version: "156.0.8078.47", Identity: browserhost.ProfileIdentity{Mode: "generated"}}
}
func TestChromeIdentityUsesObservedEngineBrandsAndSelectedEnvironment(t *testing.T) {
	for _, preset := range browserhost.EnvironmentPresets() {
		c := identityConfig()
		environment := preset.Config
		c.Environment = &environment
		plan, err := ChromeIdentityPlan(c, fixtureNativeUA, json.RawMessage(fixtureNativeCH))
		if err != nil || !plan.Override || plan.NavigatorPlatform != preset.NavigatorPlatform || plan.ClientHints.Platform != preset.Platform || plan.ClientHints.PlatformVersion != preset.PlatformVersion || plan.Mobile != preset.Mobile || plan.DeviceScaleFactor != preset.DeviceScaleFactor || plan.MaxTouchPoints != preset.MaxTouchPoints || !strings.Contains(plan.UserAgent, "Chrome/156.0.0.0") || strings.Contains(plan.UserAgent, " Mobile") != preset.Mobile {
			t.Fatal("environment was not derived from actual engine and fixed preset", preset.Config.OS, err)
		}
		if plan.ClientHints.Brands[0].Brand != "Not=A?Brand" || plan.ClientHints.Brands[1].Version != "156" || plan.ClientHints.FullVersionList[2].Brand != "Google Chrome" || plan.ClientHints.FullVersionList[1].Version != c.Version || plan.ClientHints.FullVersionList[2].Version != c.Version {
			t.Fatal("native brands were synthesized or lost")
		}
		c.Identity = browserhost.ProfileIdentity{Mode: "manual", UserAgent: " user text Chrome/1.0.0.0 "}
		manual, err := ChromeIdentityPlan(c, fixtureNativeUA, json.RawMessage(fixtureNativeCH))
		if err != nil || manual.UserAgent != c.Identity.UserAgent || manual.ClientHints.FullVersionList[1].Version != c.Version {
			t.Fatal("manual UA changed or fabricated live Client Hints")
		}
	}
}
func TestChromeNativeModeAndIdentityInputValidation(t *testing.T) {
	c := identityConfig()
	c.Identity = browserhost.ProfileIdentity{}
	plan, err := ChromeIdentityPlan(c, fixtureNativeUA, json.RawMessage(fixtureNativeCH))
	if err != nil || plan.Override || plan.UserAgent != fixtureNativeUA || plan.DeviceScaleFactor != 0 || plan.MaxTouchPoints != -1 || plan.ClientHints.PlatformVersion != "15.0.0" {
		t.Fatal("native identity was overwritten")
	}
	for _, raw := range []string{strings.Replace(fixtureNativeCH, "156.0.8078.47", "156.0.8078.48", 1), strings.Replace(fixtureNativeCH, `"platform":"Windows"`, `"platform":"Android"`, 1), strings.Replace(fixtureNativeCH, `"platform":"Windows"`, `"Platform":"Windows"`, 1), strings.Replace(fixtureNativeCH, `"model":""`, `"model":"","wow64":false`, 1), strings.Replace(fixtureNativeCH, `"brand":"Google Chrome"`, `"brand":"Unexpected"`, 1), strings.Replace(fixtureNativeCH, `"mobile":false`, `"mobile":true`, 1)} {
		if _, err := ChromeIdentityPlan(c, fixtureNativeUA, json.RawMessage(raw)); err == nil {
			t.Fatal("untrusted/mismatched native hints accepted")
		}
	}
	for _, ua := range []string{"", " ", "A\r\nB", "A\tB", "名称", strings.Repeat("a", 1025)} {
		c.Identity = browserhost.ProfileIdentity{Mode: "manual", UserAgent: ua}
		if validateIdentity(c) == nil {
			t.Fatal("unsafe manual UA accepted")
		}
	}
	c.Identity = browserhost.ProfileIdentity{Mode: "manual", UserAgent: "safe", ClientHints: &browserhost.ClientHints{}}
	if validateIdentity(c) == nil {
		t.Fatal("frontend Client Hints injection accepted")
	}
	c.Identity = browserhost.ProfileIdentity{Mode: "native", UserAgent: "injected"}
	if validateIdentity(c) == nil {
		t.Fatal("native accepted override text")
	}
}
func TestNativeIdentityPreviewIsOnlyAPlan(t *testing.T) {
	c := identityConfig()
	preset := browserhost.EnvironmentPresets()[2].Config
	c.Environment = &preset
	p, err := PreviewIdentity(c)
	if err != nil || p.Applied || p.ExactChromiumVersionKnown || !p.RequiresRuntimeReadback || p.VersionSource != "selected-official-version-not-running" || !strings.Contains(p.UserAgent, "Android 10; K") || p.NavigatorPlatform != "Linux armv8l" {
		t.Fatal("draft preview claimed actual runtime state")
	}
	c.Identity = browserhost.ProfileIdentity{Mode: "manual", UserAgent: "Exact Edited UA/7"}
	p, err = PreviewIdentity(c)
	if err != nil || p.UserAgent != c.Identity.UserAgent || p.Applied {
		t.Fatal("preview rewrote manual text")
	}
	c.Environment = &browserhost.EnvironmentConfig{Mode: "browser-emulation", OS: "ios", DevicePresetID: "pixel-7"}
	if _, err = PreviewIdentity(c); err == nil {
		t.Fatal("mismatched platform/device accepted")
	}
}

func TestFirefoxIdentityUsesNativeGeckoVersionAndRealProfilePreferences(t *testing.T) {
	c := identityConfig()
	c.EngineID = browserengine.Firefox
	c.Version = "158.0.1"
	const native = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:158.0) Gecko/20100101 Firefox/158.0"
	ua, err := FirefoxIdentityPlan(c, native)
	if err != nil || ua != native {
		t.Fatal("generated native Firefox version reduction was changed")
	}
	for _, preset := range browserhost.EnvironmentPresets() {
		environment := preset.Config
		c.Environment = &environment
		ua, err := FirefoxIdentityPlan(c, native)
		gecko := "20100101"
		if preset.Mobile {
			gecko = "158.0"
		}
		if err != nil || !strings.Contains(ua, "Gecko/"+gecko+" Firefox/158.0") || strings.Contains(ua, "Safari/") || strings.Contains(ua, " Mobile") != preset.Mobile {
			t.Fatal("Firefox generator invented a different engine")
		}
		prefs := string(firefoxPreferences(c, 49152))
		for _, key := range []string{"general.platform.override", "general.oscpu.override", "dom.maxtouchpoints.testing.value", "dom.w3c_touch_events.enabled", "dom.meta-viewport.enabled"} {
			if !strings.Contains(prefs, "user_pref(\""+key+"\",") {
				t.Fatal("requested native preference missing", key)
			}
		}
		c.Identity = browserhost.ProfileIdentity{Mode: "manual", UserAgent: " Exactly user edited Gecko/9 "}
		ua, err = FirefoxIdentityPlan(c, native)
		if err != nil || ua != c.Identity.UserAgent {
			t.Fatal("manual Firefox UA was rewritten")
		}
		c.Identity.Mode = "generated"
		c.Identity.UserAgent = ""
	}
	if _, err = FirefoxIdentityPlan(c, strings.Replace(native, "Firefox/158.0", "Firefox/157.0", 1)); err == nil {
		t.Fatal("wrong engine major accepted")
	}
}
