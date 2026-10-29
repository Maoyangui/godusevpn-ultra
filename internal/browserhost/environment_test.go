package browserhost

import (
	"encoding/json"
	"strings"
	"testing"
)

func environmentFixture(t *testing.T) (ProfileConfig, RuntimeContext) {
	t.Helper()
	runtime := RuntimeContext{Engine: Engine{ID: "webview2", Available: true, Version: "130.0.2849.80"}, Identity: &ObservedIdentity{RuntimeVersion: "130.0.2849.80", UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0", ClientHints: ClientHints{
		Brands: []BrandVersion{{"Not_A Brand", "99"}, {"Chromium", "130"}, {"Microsoft Edge", "130"}}, FullVersionList: []BrandVersion{{"Not_A Brand", "99.0.0.0"}, {"Chromium", "130.0.6723.58"}, {"Microsoft Edge", "130.0.2849.80"}}, Platform: "Windows", PlatformVersion: "15.0.0", Architecture: "x86", Bitness: "64",
	}}}
	config, err := DefaultProfile(runtime, "environment-test", previewBinding())
	if err != nil {
		t.Fatal(err)
	}
	preset := EnvironmentPresets()[2]
	config.Environment = &preset.Config
	config.Viewport = preset.RecommendedViewport
	return config, runtime
}
func TestEnvironmentPresetsArePairedAndIndependent(t *testing.T) {
	config, runtime := environmentFixture(t)
	config.Identity.Mode = "generated"
	for _, preset := range EnvironmentPresets() {
		config.Environment = &preset.Config
		config.Viewport = preset.RecommendedViewport
		if err := ValidateProfile(config, runtime); err != nil {
			t.Fatalf("preset %s: %v", preset.Config.OS, err)
		}
		expected, err := ExpectedEnvironmentIdentity(config, runtime)
		if err != nil {
			t.Fatal(err)
		}
		if expected.NavigatorPlatform != preset.NavigatorPlatform || expected.ClientHints.PlatformVersion != preset.PlatformVersion || expected.ClientHints.Mobile != preset.Mobile || expected.ClientHints.Model != preset.Model || expected.DeviceScaleFactor != preset.DeviceScaleFactor || expected.MaxTouchPoints != preset.MaxTouchPoints {
			t.Fatal("inconsistent device identity")
		}
		if !strings.Contains(expected.UserAgent, "Chrome/130.0.0.0") || strings.Contains(expected.UserAgent, "130.0.6723.58") || strings.Contains(expected.UserAgent, "130.0.2849.80") || strings.Contains(expected.UserAgent, "Version/16") {
			t.Fatal("UA invented engine version or claimed Safari")
		}
		for _, brand := range expected.ClientHints.FullVersionList {
			if brand.Brand == "Microsoft Edge" {
				t.Fatal("Chrome environment retained Edge brand")
			}
		}
	}
	first := EnvironmentPresets()
	first[0].Config.OS = "tampered"
	if EnvironmentPresets()[0].Config.OS != "windows" {
		t.Fatal("preset alias")
	}
}
func TestEnvironmentRejectsMixedOrInjectedConfiguration(t *testing.T) {
	config, runtime := environmentFixture(t)
	for _, bad := range []EnvironmentConfig{
		{Mode: "browser-emulation", OS: "ios", DevicePresetID: "pixel-7"}, {Mode: "browser-emulation", OS: "linux", DevicePresetID: "windows-desktop"}, {Mode: "native", OS: "android"}, {Mode: "native", OSVersion: "13"}, {Mode: "command", DevicePresetID: "../escape"},
	} {
		config.Environment = &bad
		if ValidateProfile(config, runtime) == nil {
			t.Fatalf("accepted inconsistent environment %+v", bad)
		}
	}
	config, runtime = environmentFixture(t)
	data, _ := json.Marshal(config)
	for _, extra := range []string{`"runtimeVersion":"200.0.0.0"`, `"userAgent":"injected"`, `"deviceScaleFactor":2`, `"extraArguments":["--no-sandbox"]`, `"OS":"ios"`} {
		injected := strings.Replace(string(data), `"environment":{`, `"environment":{`+extra+`,`, 1)
		if _, err := DecodeProfile([]byte(injected), runtime); err == nil {
			t.Fatal("environment injection accepted", extra)
		}
	}
	config.Identity.Mode = "observed-exact"
	config.Identity.UserAgent = runtime.Identity.UserAgent
	config.Identity.ClientHints = &runtime.Identity.ClientHints
	if ValidateProfile(config, runtime) == nil {
		t.Fatal("explicit identity mixed with host-derived environment")
	}
}
func TestEnvironmentOSVersionEditingIsBoundedAndConsistent(t *testing.T) {
	config, runtime := environmentFixture(t)
	config.Identity.Mode = "generated"
	for input, want := range map[string]string{"13": "13.0.0", "17.2": "17.2.0", "1.2.3": "1.2.3", "999.999.999": "999.999.999"} {
		config.Environment.OSVersion = input
		preset, err := EffectiveEnvironment(*config.Environment)
		if err != nil || preset.PlatformVersion != want {
			t.Fatal("version derivation")
		}
		identity, err := ExpectedEnvironmentIdentity(config, runtime)
		if err != nil || identity.ClientHints.PlatformVersion != want || !strings.Contains(identity.UserAgent, "Linux; Android 10; K") {
			t.Fatal("OS version did not apply consistently")
		}
	}
	for _, bad := range []string{"0", "01.2", "1.02", "1.2.03", "1.2.3.4", "1000", "1.-1", " 13", "13\n", "13; injected", "13_0"} {
		config.Environment.OSVersion = bad
		if ValidateProfile(config, runtime) == nil {
			t.Fatal("unsafe version accepted")
		}
	}
}
func TestMobileViewportRequiresAnExplicitMobilePreset(t *testing.T) {
	config, runtime := environmentFixture(t)
	config.Viewport = ViewportConfig{320, 568}
	if ValidateProfile(config, runtime) != nil {
		t.Fatal("supported mobile viewport rejected")
	}
	config.Environment = nil
	if ValidateProfile(config, runtime) == nil {
		t.Fatal("native accepted mobile dimensions")
	}
	config.Environment = &EnvironmentConfig{Mode: "native"}
	if ValidateProfile(config, runtime) == nil {
		t.Fatal("explicit native accepted mobile dimensions")
	}
	preset := EnvironmentPresets()[0]
	config.Environment = &preset.Config
	if ValidateProfile(config, runtime) == nil {
		t.Fatal("desktop accepted mobile dimensions")
	}
	config, _ = environmentFixture(t)
	for _, size := range []ViewportConfig{{319, 800}, {390, 567}, {3841, 800}, {390, 2161}} {
		config.Viewport = size
		if ValidateProfile(config, runtime) == nil {
			t.Fatal("mobile bounds exceeded")
		}
	}
}
func TestEnvironmentRequiresLiveChromiumVersionAndMatchingMajors(t *testing.T) {
	for _, mutate := range []func(*RuntimeContext){
		func(r *RuntimeContext) { r.Identity = nil }, func(r *RuntimeContext) { r.Identity.RuntimeVersion = "129.0.1.1" }, func(r *RuntimeContext) { r.Identity.ClientHints.FullVersionList[1].Version = "131.0.6723.58" }, func(r *RuntimeContext) { r.Identity.UserAgent = "Chrome/129.0.0.0" }, func(r *RuntimeContext) { r.Identity.ClientHints.Brands[1].Version = "131" }, func(r *RuntimeContext) {
			r.Identity.ClientHints.FullVersionList = append(r.Identity.ClientHints.FullVersionList, BrandVersion{"Chromium", "130.0.6723.58"})
		},
	} {
		config, runtime := environmentFixture(t)
		mutate(&runtime)
		if _, err := ExpectedEnvironmentIdentity(config, runtime); err == nil {
			t.Fatal("untrusted/unobserved runtime version authorized")
		}
	}
	config, runtime := environmentFixture(t)
	runtime.Identity = nil
	preview, err := PreviewProfile(config, runtime)
	if err != nil || preview.Applied || preview.CanLaunch || preview.EnvironmentRequirements == nil {
		t.Fatal("preview should describe requirement, not fabricate UA or success")
	}
}
func validEnvironmentReadback(t *testing.T, config ProfileConfig, runtime RuntimeContext) EnvironmentReadback {
	t.Helper()
	identity, err := ExpectedEnvironmentIdentity(config, runtime)
	if err != nil {
		t.Fatal(err)
	}
	readback := EnvironmentReadback{ProfileID: config.ProfileID, EngineID: config.EngineID, RuntimeVersion: runtime.Engine.Version, BeforeNavigation: true, WorkersBlockedBeforeRun: true, ServiceWorkerRegistrationBlocked: true}
	for _, context := range EnvironmentRequirements(config).Contexts {
		readback.Contexts = append(readback.Contexts, EnvironmentContextReadback{context, identity, config.Viewport})
	}
	return readback
}
func TestEnvironmentReadbackFailsClosedAcrossFramesAndWorkers(t *testing.T) {
	config, runtime := environmentFixture(t)
	readback := validEnvironmentReadback(t, config, runtime)
	if err := ValidateEnvironmentReadback(config, runtime, readback); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*EnvironmentReadback){func(r *EnvironmentReadback) { r.ProfileID = "other" }, func(r *EnvironmentReadback) { r.BeforeNavigation = false }, func(r *EnvironmentReadback) { r.WorkersBlockedBeforeRun = false }, func(r *EnvironmentReadback) { r.ServiceWorkerRegistrationBlocked = false }, func(r *EnvironmentReadback) { r.Contexts = r.Contexts[:2] }, func(r *EnvironmentReadback) { r.Contexts[2].Context = r.Contexts[0].Context }, func(r *EnvironmentReadback) { r.Contexts[1].Identity.NavigatorPlatform = "Win32" }, func(r *EnvironmentReadback) { r.Contexts[2].Identity.MaxTouchPoints = 0 }, func(r *EnvironmentReadback) { r.Contexts[0].Viewport.Width++ }, func(r *EnvironmentReadback) { r.Contexts[0].Identity.ClientHints.Model = "" }} {
		value := validEnvironmentReadback(t, config, runtime)
		mutate(&value)
		if ValidateEnvironmentReadback(config, runtime, value) == nil {
			t.Fatal("incomplete/mismatching readback accepted")
		}
	}
}
func TestMobileSeedPreviewPreservesEnvironmentAndNeverCreatesUA(t *testing.T) {
	config, runtime := environmentFixture(t)
	runtime.Identity = nil
	a, err := RandomizedProfilePreview(config, runtime, "seed1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := RandomizedProfilePreview(config, runtime, "seed1")
	if err != nil || !sameJSON(a.Config, b.Config) || !sameJSON(a.Config.Environment, config.Environment) || a.Config.Identity.UserAgent != "" || a.Config.Identity.ClientHints != nil || a.Config.Viewport.Width < 320 || a.Config.Viewport.Width > 414 {
		t.Fatal("seed broke mobile group or fabricated UA")
	}
}

func TestNonWindowsSeedRetainsManualGPUAndRefusesD3DNamedPreset(t *testing.T) {
	for _, index := range []int{1, 2, 3} {
		config, runtime := environmentFixture(t)
		preset := EnvironmentPresets()[index]
		config.Environment = &preset.Config
		config.Viewport = preset.RecommendedViewport
		config.WebGPU = "disabled"
		config.Graphics = &GraphicsConfig{Mode: "webgl-metadata", PresetID: "manual", Vendor: "Synthetic vendor", Renderer: "Synthetic user confirmed renderer"}
		preview, err := RandomizedProfilePreview(config, runtime, "seed1")
		if err != nil || !sameJSON(preview.Config.Graphics, config.Graphics) {
			t.Fatal("non-Windows seed inserted Windows GPU preset")
		}
		warning := false
		for _, value := range preview.Warnings {
			if strings.Contains(value, "Windows D3D preset randomization is disabled") {
				warning = true
			}
		}
		if !warning {
			t.Fatal("manual metadata scope warning missing")
		}
		graphics := GraphicsPresets()[0]
		config.Graphics = &graphics
		if ValidateProfile(config, runtime) == nil {
			t.Fatal("non-Windows environment accepted named Windows D3D GPU preset")
		}
	}
}
