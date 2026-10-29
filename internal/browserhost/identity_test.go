package browserhost

import (
	"strings"
	"testing"
)

func TestManualUserAgentPreservesPrintableTextAndRejectsControls(t *testing.T) {
	config, runtime := environmentFixture(t)
	runtime.Identity = nil
	config.Identity = ProfileIdentity{Mode: "manual"}
	for _, ua := range []string{"QA-Agent/1.0", " Custom UA / exact spaces ", strings.Repeat("x", 1024)} {
		config.Identity.UserAgent = ua
		preview, err := PreviewIdentity(config, runtime)
		if err != nil || preview.UserAgent != ua || preview.Applied || !preview.RequiresRuntimeReadback || preview.ExactChromiumVersionKnown {
			t.Fatal("manual UA was changed or promoted to runtime evidence", err)
		}
	}
	for _, ua := range []string{"", " ", strings.Repeat("x", 1025), "UA\r\nHeader: value", "UA\tvalue", "UA\x00", "UA\x7f", "UA中文"} {
		config.Identity.UserAgent = ua
		if ValidateProfile(config, runtime) == nil {
			t.Fatal("unsafe/manual empty UA accepted")
		}
	}
	for b := 0; b <= 255; b++ {
		config.Identity.UserAgent = "A" + string([]byte{byte(b)}) + "B"
		err := ValidateProfile(config, runtime)
		if (err == nil) != (b >= 32 && b <= 126) {
			t.Fatal("printable ASCII boundary mismatch", b)
		}
	}
	config.Identity = ProfileIdentity{Mode: "manual", UserAgent: "QA-Agent/1.0", ClientHints: &ClientHints{}}
	if ValidateProfile(config, runtime) == nil {
		t.Fatal("manual frontend Client Hints accepted")
	}
}

func TestGeneratedPreviewUsesSelectedEnvironmentAndInstalledMajorOnly(t *testing.T) {
	config, runtime := environmentFixture(t)
	runtime.Identity = nil
	config.Identity = ProfileIdentity{Mode: "generated"}
	for _, preset := range EnvironmentPresets() {
		config.Environment = &preset.Config
		config.Viewport = preset.RecommendedViewport
		p, err := PreviewIdentity(config, runtime)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(p.UserAgent, "Chrome/130.0.0.0") || strings.Contains(p.UserAgent, "2849") || p.ExactChromiumVersionKnown || p.ChromiumVersion != "" || p.Applied || p.VersionSource != "installed-runtime-product-major" || p.NavigatorPlatform != preset.NavigatorPlatform || strings.Contains(p.UserAgent, " Mobile") != preset.Mobile {
			t.Fatal("planned reduced UA not coherent with preset", preset.Config.OS)
		}
	}
	config.Environment = nil
	config.Viewport = ViewportConfig{1280, 720}
	p, err := PreviewIdentity(config, runtime)
	if err != nil || !strings.Contains(p.UserAgent, "Windows NT 10.0; Win64; x64") || config.Environment != nil {
		t.Fatal("effective Windows fallback modified config", err)
	}
	config.Environment = &EnvironmentConfig{Mode: "native"}
	if _, err = PreviewIdentity(config, runtime); err != nil {
		t.Fatal("explicit native environment cannot generate Windows UA", err)
	}
	config.Identity.UserAgent = "supplied by frontend"
	if ValidateProfile(config, runtime) == nil {
		t.Fatal("generated mode accepted frontend UA")
	}
}

func TestIdentityPlanDoesNotInventUnknownChromiumOrDropManualText(t *testing.T) {
	config, runtime := environmentFixture(t)
	runtime.Identity = nil
	runtime.Engine.Version = ""
	config.Identity = ProfileIdentity{Mode: "generated"}
	p, err := PreviewIdentity(config, runtime)
	if err != nil || p.UserAgent != "" || !strings.Contains(p.UserAgentTemplate, "{ChromiumMajor}.0.0.0") || p.ExactChromiumVersionKnown || p.RuntimeProductVersion != "" {
		t.Fatal("unknown engine major became a fake UA", err)
	}
	config.Identity = ProfileIdentity{Mode: "manual", UserAgent: "My exactly edited agent"}
	p, err = PreviewIdentity(config, runtime)
	if err != nil || p.UserAgent != config.Identity.UserAgent {
		t.Fatal("unknown engine dropped explicit manual UA", err)
	}
}

func TestLiveIdentityPlanUsesChromiumFullAndManualReadbackStillUsesSelectedCH(t *testing.T) {
	for _, preset := range EnvironmentPresets() {
		config, runtime := environmentFixture(t)
		config.Environment = &preset.Config
		config.Viewport = preset.RecommendedViewport
		config.Identity = ProfileIdentity{Mode: "generated"}
		p, err := PreviewIdentity(config, runtime)
		if err != nil || !p.ExactChromiumVersionKnown || p.ChromiumVersion != "130.0.6723.58" || !strings.Contains(p.UserAgent, "Chrome/130.0.0.0") || strings.Contains(p.UserAgent, "Chrome/130.0.6723.58") || strings.Contains(p.UserAgent, "Chrome/130.0.2849.80") || p.VersionSource != "live-native-client-hints" || p.Applied {
			t.Fatal("Edge full version substituted for observed Chromium", err)
		}
		config.Identity = ProfileIdentity{Mode: "manual", UserAgent: "Custom user edited UA/7.0"}
		expected, err := ExpectedEnvironmentIdentity(config, runtime)
		if err != nil || expected.UserAgent != config.Identity.UserAgent || expected.ClientHints.Platform != preset.Platform || expected.ClientHints.PlatformVersion != preset.PlatformVersion || expected.ClientHints.Model != preset.Model {
			t.Fatal("manual override silently reverted or changed selected CH", err)
		}
		readback := validEnvironmentReadback(t, config, runtime)
		if ValidateEnvironmentReadback(config, runtime, readback) != nil {
			t.Fatal("manual readback refused")
		}
		readback.Contexts[0].Identity.UserAgent = "native UA restored"
		if ValidateEnvironmentReadback(config, runtime, readback) == nil {
			t.Fatal("manual UA readback mismatch allowed")
		}
	}
}

func TestGeneratedNativePlanAndLegacySnapshotsRemainDistinct(t *testing.T) {
	config, runtime := environmentFixture(t)
	config.Environment = nil
	config.Viewport = ViewportConfig{1280, 720}
	config.Identity = ProfileIdentity{Mode: "generated"}
	p, err := PreviewIdentity(config, runtime)
	if err != nil || !strings.Contains(p.UserAgent, "Chrome/130.0.0.0") || strings.Contains(p.UserAgent, "Chrome/130.0.6723.58") || strings.Contains(p.UserAgent, "Edg/") {
		t.Fatal("generated native Windows UA failed", err)
	}
	config.Identity = ProfileIdentity{Mode: "native"}
	p, err = PreviewIdentity(config, runtime)
	if err != nil || p.UserAgent != runtime.Identity.UserAgent {
		t.Fatal("legacy native snapshot changed", err)
	}
	config.Identity = ProfileIdentity{Mode: "observed-exact", UserAgent: runtime.Identity.UserAgent, ClientHints: &runtime.Identity.ClientHints}
	p, err = PreviewIdentity(config, runtime)
	if err != nil || p.UserAgent != config.Identity.UserAgent {
		t.Fatal("legacy observed-exact snapshot changed", err)
	}
	runtime.Identity.ClientHints.FullVersionList[1].Version = "131.0.6723.58"
	if _, err = PreviewIdentity(config, runtime); err == nil {
		t.Fatal("different Chromium engine major supplied runtime evidence")
	}
}

func TestIdentityPreviewIsDetachedAndRandomizationPreservesManualText(t *testing.T) {
	config, runtime := environmentFixture(t)
	runtime.Identity = nil
	config.Identity = ProfileIdentity{Mode: "manual", UserAgent: "Exact manual UA/1.0"}
	preview, err := RandomizedProfilePreview(config, runtime, "identity-seed")
	if err != nil || preview.Config.Identity.UserAgent != config.Identity.UserAgent || preview.IdentityPreview.UserAgent != config.Identity.UserAgent || preview.IdentityPreview.Applied {
		t.Fatal("seed or preview changed manual identity", err)
	}
	preview.IdentityPreview.Warnings[0] = "changed"
	again, _ := PreviewIdentity(config, runtime)
	if again.Warnings[0] == "changed" {
		t.Fatal("preview warnings alias")
	}
	runtime.Engine.ID = "edge"
	config.EngineID = "edge"
	config.Environment = nil
	config.Viewport = ViewportConfig{1280, 720}
	if ValidateProfile(config, runtime) == nil {
		t.Fatal("manual UA promoted to unsupported native-engine host")
	}
}

func TestManualMismatchesHaveSpecificFeedbackWithoutRewritingText(t *testing.T) {
	config, runtime := environmentFixture(t)
	runtime.Identity = nil
	config.Identity = ProfileIdentity{Mode: "manual", UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/999.0.0.0"}
	p, err := PreviewIdentity(config, runtime)
	if err != nil || p.UserAgent != config.Identity.UserAgent {
		t.Fatal("manual choice rewritten", err)
	}
	joined := strings.Join(p.Warnings, "\n")
	if !strings.Contains(joined, "Chrome major differs") || !strings.Contains(joined, "OS token differs") {
		t.Fatal("specific manual mismatch feedback missing")
	}
}

func TestLegacyObservedSnapshotWithoutChromiumFullStillPreviews(t *testing.T) {
	runtime := observedRuntime()
	runtime.Identity.ClientHints.Brands = runtime.Identity.ClientHints.Brands[:1]
	runtime.Identity.ClientHints.FullVersionList = runtime.Identity.ClientHints.FullVersionList[:1]
	config := observedProfile(t, runtime)
	for _, mode := range []string{"native", "observed-exact"} {
		if mode == "native" {
			config.Identity = ProfileIdentity{Mode: "native"}
		} else {
			config.Identity = ProfileIdentity{Mode: mode, UserAgent: runtime.Identity.UserAgent, ClientHints: &runtime.Identity.ClientHints}
		}
		p, err := PreviewProfile(config, runtime)
		if err != nil || p.IdentityPreview.UserAgent != runtime.Identity.UserAgent || p.IdentityPreview.ExactChromiumVersionKnown {
			t.Fatal("legacy native/observed-exact lost compatibility", err)
		}
	}
}
