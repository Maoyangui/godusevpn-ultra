package browserhost

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func previewRuntime() RuntimeContext {
	return RuntimeContext{Engine: Engine{ID: "edge", Available: true, Version: "149.0.1234.56"}}
}

func previewBinding() NetworkBinding {
	return NetworkBinding{Mode: "tunnel", ReferenceID: "test-tunnel"}
}

func validProfile(t *testing.T) ProfileConfig {
	t.Helper()
	c, err := DefaultProfile(previewRuntime(), "private-test", previewBinding())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestProfileJSONRoundTripAndPreviewDoNotClaimEnforcement(t *testing.T) {
	runtime := previewRuntime()
	config := validProfile(t)
	config.Region = RegionPresets()[5]
	config.Permissions.Camera = "block"
	config.Cookies = "block-third-party"
	config.WebRTC = "disable-non-proxied-udp"
	config.WebGL = "disabled"
	config.ExitCleanup = "delete-owned-profile"
	data, err := EncodeProfile(config, runtime)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeProfile(data, runtime)
	if err != nil || !reflect.DeepEqual(config, decoded) {
		t.Fatalf("round trip failed: %v", err)
	}
	preview, err := PreviewProfile(decoded, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || preview.CanLaunch || !preview.NeedsRuntimeReadback || preview.Capabilities.HostImplemented || preview.Capabilities.CanLaunch || preview.Capabilities.RuntimeVerified {
		t.Fatal("pure preview claimed a host, launch, setting or runtime evidence")
	}
	for _, field := range preview.Capabilities.Parameters {
		if field.Enforced || field.Detail == "" {
			t.Fatalf("misleading or unexplained capability: %s", field.ID)
		}
	}
	decoded.Region.AcceptLanguages[0] = "corrupted"
	if preview.Config.Region.AcceptLanguages[0] != "zh-CN" {
		t.Fatal("preview aliases mutable caller configuration")
	}
}

func TestManualProfileRejectsUnsafeAndUnsupportedValues(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		mutate      func(*ProfileConfig)
	}{
		{"future schema", "schemaVersion", func(c *ProfileConfig) { c.SchemaVersion++ }},
		{"unknown engine", "engineId", func(c *ProfileConfig) { c.EngineID = "firefox" }},
		{"unprovided CEF", "engineId", func(c *ProfileConfig) { c.EngineID = "cef" }},
		{"engine alias must serialize canonically", "engineId", func(c *ProfileConfig) { c.EngineID = "webview" }},
		{"profile traversal", "profileId", func(c *ProfileConfig) { c.ProfileID = "../default" }},
		{"drive path", "profileId", func(c *ProfileConfig) { c.ProfileID = `C:\\Users\\Default` }},
		{"UNC path", "profileId", func(c *ProfileConfig) { c.ProfileID = `\\server\\share` }},
		{"device name", "profileId", func(c *ProfileConfig) { c.ProfileID = "con" }},
		{"device port", "profileId", func(c *ProfileConfig) { c.ProfileID = "lpt9" }},
		{"alternate stream", "profileId", func(c *ProfileConfig) { c.ProfileID = "profile:stream" }},
		{"argument injection", "profileId", func(c *ProfileConfig) { c.ProfileID = "--remote-debugging-port=1" }},
		{"trailing dot", "profileId", func(c *ProfileConfig) { c.ProfileID = "profile." }},
		{"header under native mode", "identity", func(c *ProfileConfig) { c.Identity.UserAgent = "injected" }},
		{"arbitrary identity", "identity.mode", func(c *ProfileConfig) { c.Identity.Mode = "random" }},
		{"locale mismatch", "region", func(c *ProfileConfig) { c.Region.Locale = "ja-JP" }},
		{"timezone mismatch", "region", func(c *ProfileConfig) { c.Region.Timezone = "Asia/Tokyo" }},
		{"language injection", "region", func(c *ProfileConfig) { c.Region.AcceptLanguages = []string{"en-US\r\nX-Test: injected"} }},
		{"unsupported region", "region", func(c *ProfileConfig) { c.Region.PresetID = "arbitrary" }},
		{"negative viewport", "viewport", func(c *ProfileConfig) { c.Viewport.Width = -1 }},
		{"oversized viewport", "viewport", func(c *ProfileConfig) { c.Viewport.Height = 1 << 30 }},
		{"fake WebRTC disable", "webrtc", func(c *ProfileConfig) { c.WebRTC = "disabled" }},
		{"fake WebGL GPU", "webgl", func(c *ProfileConfig) { c.WebGL = "random-gpu" }},
		{"unsupported WebGPU adapter override", "webgpu", func(c *ProfileConfig) { c.WebGPU = "custom-adapter" }},
		{"global permission grant", "permissions.camera", func(c *ProfileConfig) { c.Permissions.Camera = "allow" }},
		{"global clipboard grant", "permissions.clipboardRead", func(c *ProfileConfig) { c.Permissions.ClipboardRead = "allow" }},
		{"unsupported cookies", "cookies", func(c *ProfileConfig) { c.Cookies = "block-all" }},
		{"direct network", "binding.mode", func(c *ProfileConfig) { c.Binding.Mode = "direct" }},
		{"fallback", "binding.allowDirectFallback", func(c *ProfileConfig) { c.Binding.AllowDirectFallback = true }},
		{"binding path", "binding.referenceId", func(c *ProfileConfig) { c.Binding.ReferenceID = "../../tunnel" }},
		{"unexpected proxy", "binding.proxyUrl", func(c *ProfileConfig) { c.Binding.ProxyURL = "http://127.0.0.1:8080" }},
		{"arbitrary cleanup", "exitCleanup", func(c *ProfileConfig) { c.ExitCleanup = "delete-all-browser-data" }},
		{"seed injection", "randomizationSeed", func(c *ProfileConfig) { c.RandomizationSeed = "seed\x00injection" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := validProfile(t)
			tc.mutate(&config)
			err := ValidateProfile(config, previewRuntime())
			var detail *ConfigError
			if !errors.As(err, &detail) || detail.Field != tc.field {
				t.Fatalf("expected error for %s, got %v", tc.field, err)
			}
			if _, err := PreviewProfile(config, previewRuntime()); err == nil {
				t.Fatal("invalid proposal still produced a preview")
			}
		})
	}
}

func TestLoopbackProxyRejectsCredentialsPathsRemoteHostsAndFallback(t *testing.T) {
	for _, endpoint := range []string{"http://127.0.0.1:8080", "socks5://[::1]:1080"} {
		binding := NetworkBinding{Mode: "loopback-proxy", ReferenceID: "local-proxy", ProxyURL: endpoint}
		if _, err := DefaultProfile(previewRuntime(), "private-test", binding); err != nil {
			t.Fatalf("valid literal loopback endpoint rejected: %v", err)
		}
	}
	for _, endpoint := range []string{
		"http://example.com:8080", "http://localhost:8080", "http://0.0.0.0:8080", "http://192.0.2.1:8080",
		"http://127.0.0.1.evil.invalid:8080", "http://2130706433:8080", "http://[::ffff:127.0.0.1]:8080",
		"http://user:password@127.0.0.1:8080", "http://127.0.0.1:8080/path", "http://127.0.0.1:8080/",
		"http://127.0.0.1:8080?token=test", "http://127.0.0.1:8080?", "http://127.0.0.1:8080#x", "http://127.0.0.1:8080#",
		"http://127.0.0.1:0", "http://127.0.0.1:65536", "http://127.0.0.1:08080", "http://127.0.0.1", "http://127.0.0.1:+80",
		"https://127.0.0.1:8080", "file:///C:/browser.exe", "http://127.0.0.1:8080\r\nX-Test: injected",
	} {
		binding := NetworkBinding{Mode: "loopback-proxy", ReferenceID: "local-proxy", ProxyURL: endpoint}
		if err := validateBinding(binding); err == nil {
			t.Fatalf("unsafe endpoint accepted: %q", endpoint)
		}
	}
}

func TestProfileJSONRejectsUnknownNestedFieldsDuplicatesAndTrailingData(t *testing.T) {
	data, err := EncodeProfile(validProfile(t), previewRuntime())
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(string) string{
		"executable path": func(s string) string { return strings.Replace(s, "{", `{"executablePath":"C:/browser.exe",`, 1) },
		"profile path":    func(s string) string { return strings.Replace(s, "{", `{"profileDirectory":"../existing-profile",`, 1) },
		"debug port":      func(s string) string { return strings.Replace(s, "{", `{"remoteDebuggingPort":9222,`, 1) },
		"extra args":      func(s string) string { return strings.Replace(s, "{", `{"extraArguments":["--no-sandbox"],`, 1) },
		"nested args": func(s string) string {
			return strings.Replace(s, `"identity":{`, `"identity":{"arguments":["--no-sandbox"],`, 1)
		},
		"screen spoof": func(s string) string {
			return strings.Replace(s, `"viewport":{`, `"viewport":{"deviceScaleFactor":4,`, 1)
		},
		"duplicate ID":       func(s string) string { return strings.Replace(s, "{", `{"profileId":"other",`, 1) },
		"casefold duplicate": func(s string) string { return strings.Replace(s, "{", `{"PROFILEID":"other",`, 1) },
		"duplicate nested":   func(s string) string { return strings.Replace(s, `"binding":{`, `"binding":{"mode":"direct",`, 1) },
		"trailing object":    func(s string) string { return s + `{}` },
		"trailing scalar":    func(s string) string { return s + ` true` },
		"trailing broken":    func(s string) string { return s + ` *` },
		"null object":        func(string) string { return `null` },
		"missing fields":     func(string) string { return `{"schemaVersion":1}` },
		"size limit":         func(string) string { return strings.Repeat(" ", maxConfigBytes+1) },
		"deep nesting":       func(string) string { return strings.Repeat("[", 18) + "0" + strings.Repeat("]", 18) },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeProfile([]byte(mutate(string(data))), previewRuntime()); err == nil {
				t.Fatal("unsafe/incomplete JSON accepted")
			}
		})
	}
}

func observedRuntime() RuntimeContext {
	runtime := previewRuntime()
	runtime.Identity = &ObservedIdentity{
		RuntimeVersion: runtime.Engine.Version,
		UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/149.0.0.0 Safari/537.36 Edg/149.0.1234.56",
		ClientHints: ClientHints{
			Brands:          []BrandVersion{{"Microsoft Edge", "149"}, {"Chromium", "149"}},
			FullVersionList: []BrandVersion{{"Microsoft Edge", runtime.Engine.Version}, {"Chromium", "149.0.1234.56"}},
			Platform:        "Windows", PlatformVersion: "19.0.0", Architecture: "x86", Bitness: "64",
		},
	}
	return runtime
}

func observedProfile(t *testing.T, runtime RuntimeContext) ProfileConfig {
	t.Helper()
	config := validProfile(t)
	config.Identity = ProfileIdentity{Mode: "observed-exact", UserAgent: runtime.Identity.UserAgent, ClientHints: &runtime.Identity.ClientHints}
	return config
}

func TestExplicitIdentityRequiresIndependentMatchingRuntimeVersionAndHints(t *testing.T) {
	runtime := observedRuntime()
	config := observedProfile(t, runtime)
	if err := ValidateProfile(config, runtime); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ProfileConfig, *RuntimeContext){
		func(_ *ProfileConfig, r *RuntimeContext) { r.Identity = nil },
		func(_ *ProfileConfig, r *RuntimeContext) { r.Engine.Version = "" },
		func(_ *ProfileConfig, r *RuntimeContext) { r.Engine.Version = "150.0.1234.56" },
		func(c *ProfileConfig, _ *RuntimeContext) { c.Identity.UserAgent += " FakeBrowser/1.0" },
		func(c *ProfileConfig, _ *RuntimeContext) { c.Identity.ClientHints = nil },
		func(c *ProfileConfig, r *RuntimeContext) {
			r.Identity.UserAgent = strings.ReplaceAll(r.Identity.UserAgent, "Edg/149.0.1234.56", "Edg/148.0.0.0")
			c.Identity.UserAgent = r.Identity.UserAgent
		},
		func(c *ProfileConfig, r *RuntimeContext) {
			r.Identity.UserAgent = strings.ReplaceAll(r.Identity.UserAgent, "Edg/149.0.1234.56", "Edg/149.9.9.9")
			c.Identity.UserAgent = r.Identity.UserAgent
		},
		func(_ *ProfileConfig, r *RuntimeContext) {
			r.Identity.ClientHints.FullVersionList[0].Version = "148.0.0.0"
		},
		func(_ *ProfileConfig, r *RuntimeContext) { r.Identity.ClientHints.Brands[0].Version = "150" },
		func(_ *ProfileConfig, r *RuntimeContext) { r.Identity.ClientHints.Platform = "Android" },
		func(_ *ProfileConfig, r *RuntimeContext) { r.Identity.ClientHints.Mobile = true },
		func(_ *ProfileConfig, r *RuntimeContext) { r.Identity.ClientHints.Bitness = "128" },
		func(_ *ProfileConfig, r *RuntimeContext) { r.Identity.ClientHints.Architecture = "invented-cpu" },
		func(_ *ProfileConfig, r *RuntimeContext) {
			r.Identity.ClientHints.Brands[0].Brand = "Microsoft Edge\r\nInjected"
		},
		func(c *ProfileConfig, r *RuntimeContext) {
			r.Identity.UserAgent += "\r\nInjected: value"
			c.Identity.UserAgent = r.Identity.UserAgent
		},
	} {
		runtime := observedRuntime()
		config := observedProfile(t, runtime)
		mutate(&config, &runtime)
		if err := ValidateProfile(config, runtime); err == nil {
			t.Fatal("unobserved, mismatched or injectable identity accepted")
		}
	}
}

func TestObservedIdentityAllowsOnlyActualEngineBrandAndObservedReducedVersion(t *testing.T) {
	for _, engine := range []struct{ id, brand, token string }{
		{"edge", "Microsoft Edge", "Edg/"},
		{"webview", "Microsoft Edge WebView2", "Edg/"},
		{"chrome", "Google Chrome", "Chrome/"},
	} {
		runtime := observedRuntime()
		runtime.Engine.ID = engine.id
		runtime.Identity.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " + engine.token + "149.0.0.0"
		runtime.Identity.ClientHints.Brands[0].Brand = engine.brand
		runtime.Identity.ClientHints.FullVersionList[0].Brand = engine.brand
		config, err := DefaultProfile(runtime, "private-test", previewBinding())
		if err != nil {
			t.Fatal(err)
		}
		config.Identity = ProfileIdentity{Mode: "observed-exact", UserAgent: runtime.Identity.UserAgent, ClientHints: &runtime.Identity.ClientHints}
		if err := ValidateProfile(config, runtime); err != nil {
			t.Fatalf("exact observed identity rejected for %s: %v", engine.id, err)
		}
		runtime.Identity.ClientHints.FullVersionList[0].Brand = "Unrelated Browser"
		if err := ValidateProfile(config, runtime); err == nil {
			t.Fatalf("runtime brand absent from full-version hints accepted for %s", engine.id)
		}
	}
}

func TestSeededPreviewIsRepeatableAndPreservesUnknownNativeIdentity(t *testing.T) {
	runtime := previewRuntime()
	runtime.Engine.Version = ""
	first, err := SeededPreview(runtime, "private-test", "chosen_seed_42", previewBinding())
	if err != nil {
		t.Fatal(err)
	}
	second, err := SeededPreview(runtime, "private-test", "chosen_seed_42", previewBinding())
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("fixed seed was not deterministic: %v", err)
	}
	for _, seed := range []string{"a", "b", "another_seed", "new-identity-1", "new-identity-2"} {
		preview, err := SeededPreview(runtime, "private-test", seed, previewBinding())
		if err != nil {
			t.Fatal(err)
		}
		if preview.Config.Identity.Mode != "native" || preview.Config.Identity.UserAgent != "" || preview.Config.Identity.ClientHints != nil {
			t.Fatal("randomization synthesized a runtime identity")
		}
		if err := ValidateProfile(preview.Config, runtime); err != nil {
			t.Fatalf("seed generated an inconsistent region: %v", err)
		}
	}
	for _, seed := range []string{"", "seed with spaces", "../seed", strings.Repeat("a", 129)} {
		if _, err := SeededPreview(runtime, "private-test", seed, previewBinding()); err == nil {
			t.Fatal("unsupported seed accepted")
		}
	}
	regions := RegionPresets()
	regions[0].AcceptLanguages[0] = "corrupted"
	if RegionPresets()[0].AcceptLanguages[0] != "en-US" {
		t.Fatal("caller could mutate supported region groups")
	}
}

func TestCapabilitiesNeverInferHostSupportFromRuntimeInventory(t *testing.T) {
	for _, engine := range []Engine{
		{ID: "edge", Available: true}, {ID: "chrome", Available: true}, {ID: "webview", Available: true},
		{ID: "webview2", Available: true}, {ID: "cef", Available: true}, {ID: "firefox", Available: true}, {ID: "edge", Available: false},
	} {
		runtime := RuntimeContext{Engine: engine}
		matrix := CapabilitiesFor(runtime)
		if matrix.CanLaunch || matrix.HostImplemented || matrix.RuntimeVerified {
			t.Fatal("inventory was upgraded to host/runtime proof")
		}
		_, err := DefaultProfile(runtime, "private-test", previewBinding())
		if matrix.PreviewSupported != (err == nil) {
			t.Fatalf("capability and validation disagree for %s", engine.ID)
		}
		for _, field := range matrix.Parameters {
			want := []string{"native"}
			if canonicalEngineID(engine.ID) == "webview2" {
				want = append(want, "generated", "manual")
			}
			if field.ID == "identity" && !reflect.DeepEqual(field.Values, want) {
				t.Fatal("identity capabilities confused planned override with observed snapshot")
			}
		}
	}
	config, err := DefaultProfile(RuntimeContext{Engine: Engine{ID: "webview", Available: true}}, "private-test", previewBinding())
	if err != nil || config.EngineID != "webview2" {
		t.Fatal("Windows inventory alias was not handled at the configuration boundary")
	}
	matrix := CapabilitiesFor(observedRuntime())
	encoded, _ := json.Marshal(matrix)
	if strings.Contains(string(encoded), "Mozilla/") || strings.Contains(string(encoded), "19.0.0") {
		t.Fatal("capability matrix leaked observed identity values")
	}
}
