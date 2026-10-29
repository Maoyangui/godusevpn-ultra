package browserhost

import (
	"reflect"
	"strings"
	"testing"
)

func graphicsRuntime() RuntimeContext {
	return RuntimeContext{Engine: Engine{ID: "webview2", Available: true, Version: "149.0.1234.56"}}
}

func graphicsFixture(t *testing.T) ProfileConfig {
	t.Helper()
	config, err := DefaultProfile(graphicsRuntime(), "graphics-fixture", previewBinding())
	if err != nil {
		t.Fatal(err)
	}
	preset := GraphicsPresets()[0]
	config.Graphics = &preset
	config.WebGPU = "disabled"
	return config
}

func TestGraphicsNativeRemainsBackwardCompatibleAndNeverChangesHardware(t *testing.T) {
	config, err := DefaultProfile(graphicsRuntime(), "native-fixture", previewBinding())
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncodeProfile(config, graphicsRuntime())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"graphics"`) || config.WebGPU != "runtime-default" || GraphicsRequirements(config) != nil {
		t.Fatal("native default gained an override")
	}
	decoded, err := DecodeProfile(data, graphicsRuntime())
	if err != nil || decoded.Graphics != nil {
		t.Fatal("old profile JSON no longer compatible", err)
	}
	config.Graphics = &GraphicsConfig{Mode: "native"}
	if err := ValidateProfile(config, graphicsRuntime()); err != nil {
		t.Fatal(err)
	}
	config.WebGPU = "disabled"
	required := GraphicsRequirements(config)
	if err := ValidateProfile(config, graphicsRuntime()); err != nil || required == nil || required.WebGLMetadata || !required.WebGPUDisabled || required.ChangesNativeHardware || required.ChangesWebGPUAdapterIdentity {
		t.Fatal("independent WebGPU disable mixed with hardware/metadata", err)
	}
}

func TestGraphicsPresetAndManualJSONHaveStrictBoundaries(t *testing.T) {
	config := graphicsFixture(t)
	for _, preset := range GraphicsPresets() {
		config.Graphics = &preset
		data, err := EncodeProfile(config, graphicsRuntime())
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeProfile(data, graphicsRuntime())
		if err != nil || !reflect.DeepEqual(config, decoded) {
			t.Fatal("preset round trip failed", err)
		}
	}
	config.Graphics = &GraphicsConfig{Mode: "webgl-metadata", PresetID: "manual", Vendor: `Display "vendor" <test>`, Renderer: `Renderer "; throw new Error('data only'); //`}
	data, err := EncodeProfile(config, graphicsRuntime())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "<test>") || strings.Contains(string(data), `"vendor":"Display "vendor"`) {
		t.Fatal("manual strings were emitted as executable/raw markup")
	}
	decoded, err := DecodeProfile(data, graphicsRuntime())
	if err != nil || !reflect.DeepEqual(config.Graphics, decoded.Graphics) {
		t.Fatal("manual metadata was not plain JSON data", err)
	}
	for _, field := range []string{`"script":"alert(1)"`, `"arguments":["--gpu-testing-gl-vendor=x"]`, `"nativeDeviceName":"other"`, `"adapter":{"device":"fake"}`, `"vendor":"duplicate"`} {
		injected := strings.Replace(string(data), `"graphics":{`, `"graphics":{`+field+",", 1)
		if _, err := DecodeProfile([]byte(injected), graphicsRuntime()); err == nil {
			t.Fatal("unknown/duplicate GPU field accepted", field)
		}
	}
}

func TestGraphicsRejectsUnsupportedNamesInconsistentPresetsAndUncoveredWebGPU(t *testing.T) {
	for _, mutate := range []func(*ProfileConfig){
		func(c *ProfileConfig) { c.Graphics.Mode = "system-device-name" }, func(c *ProfileConfig) { c.Graphics.Mode = "native" },
		func(c *ProfileConfig) { c.WebGPU = "runtime-default" }, func(c *ProfileConfig) { c.WebGL = "disabled" },
		func(c *ProfileConfig) { c.Graphics.PresetID = "unknown" }, func(c *ProfileConfig) { c.Graphics.Vendor = "other vendor" }, func(c *ProfileConfig) { c.Graphics.Renderer = "other renderer" },
		func(c *ProfileConfig) { c.Graphics.PresetID = "manual"; c.Graphics.Vendor = "" }, func(c *ProfileConfig) { c.Graphics.PresetID = "manual"; c.Graphics.Vendor = " vendor" },
		func(c *ProfileConfig) { c.Graphics.PresetID = "manual"; c.Graphics.Vendor = "vendor\r\nInjected" }, func(c *ProfileConfig) { c.Graphics.PresetID = "manual"; c.Graphics.Renderer = "渲染器" },
		func(c *ProfileConfig) { c.Graphics.PresetID = "manual"; c.Graphics.Vendor = strings.Repeat("x", 129) }, func(c *ProfileConfig) { c.Graphics.PresetID = "manual"; c.Graphics.Renderer = strings.Repeat("x", 257) },
	} {
		config := graphicsFixture(t)
		mutate(&config)
		if err := ValidateProfile(config, graphicsRuntime()); err == nil {
			t.Fatal("unsafe or inconsistent GPU request accepted")
		}
	}
	config := graphicsFixture(t)
	runtime := graphicsRuntime()
	runtime.Engine.ID = "chrome"
	config.EngineID = "chrome"
	if err := ValidateProfile(config, runtime); err == nil {
		t.Fatal("metadata exposed on an engine with no matching host integration")
	}
}

func TestGraphicsSeedUsesPairsOnlyAfterExplicitMetadataSelection(t *testing.T) {
	config := graphicsFixture(t)
	config.Permissions.Camera = "block"
	first, err := RandomizedProfilePreview(config, graphicsRuntime(), "gpu-seed_42")
	if err != nil {
		t.Fatal(err)
	}
	second, err := RandomizedProfilePreview(config, graphicsRuntime(), "gpu-seed_42")
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("graphics seed was not reproducible", err)
	}
	if first.Config.WebGPU != "disabled" || first.Config.Permissions.Camera != "block" || first.Config.Graphics == config.Graphics {
		t.Fatal("random preview lost explicit settings or aliased caller graphics")
	}
	matched := false
	for _, preset := range GraphicsPresets() {
		if *first.Config.Graphics == preset {
			matched = true
		}
	}
	if !matched || first.Applied || first.CanLaunch || first.GraphicsRequirements == nil {
		t.Fatal("random metadata escaped presets or claimed application")
	}
	native, err := SeededPreview(graphicsRuntime(), "native-fixture", "same-seed", previewBinding())
	if err != nil || native.Config.Graphics != nil || native.Config.WebGPU != "runtime-default" {
		t.Fatal("seed silently added GPU override to native profile", err)
	}
	presets := GraphicsPresets()
	presets[0].Vendor = "corrupted"
	if GraphicsPresets()[0].Vendor == "corrupted" {
		t.Fatal("caller mutated canonical graphics presets")
	}
}

func graphicsReadbackFixture(config ProfileConfig) GraphicsReadback {
	r := GraphicsReadback{ProfileID: config.ProfileID, EngineID: config.EngineID, RuntimeVersion: graphicsRuntime().Engine.Version, BeforeNavigation: true, WorkerConstructorsBlocked: true, WorkerTargetsBlockedBeforeRun: true, ServiceWorkerRegistrationBlocked: true}
	for _, name := range []string{"main-frame", "same-origin-frame", "cross-origin-frame"} {
		r.Contexts = append(r.Contexts, GraphicsContextReadback{Context: name, WebGL1: &WebGLMetadataReadback{Vendor: config.Graphics.Vendor, Renderer: config.Graphics.Renderer}, WebGL2: &WebGLMetadataReadback{Vendor: config.Graphics.Vendor, Renderer: config.Graphics.Renderer}, WebGPUChecked: true})
	}
	return r
}

func TestGraphicsReadbackRequiresFreshIdentityFrameAPIsAndBlockedWorkers(t *testing.T) {
	config := graphicsFixture(t)
	if err := ValidateGraphicsReadback(config, graphicsRuntime(), graphicsReadbackFixture(config)); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*GraphicsReadback){
		func(r *GraphicsReadback) { r.ProfileID = "other-profile" }, func(r *GraphicsReadback) { r.RuntimeVersion = "148.0.0.0" }, func(r *GraphicsReadback) { r.EngineID = "chrome" }, func(r *GraphicsReadback) { r.BeforeNavigation = false },
		func(r *GraphicsReadback) { r.WorkerConstructorsBlocked = false }, func(r *GraphicsReadback) { r.WorkerTargetsBlockedBeforeRun = false }, func(r *GraphicsReadback) { r.ServiceWorkerRegistrationBlocked = false }, func(r *GraphicsReadback) { r.Contexts = r.Contexts[:2] },
		func(r *GraphicsReadback) { r.Contexts[2].Context = "main-frame" }, func(r *GraphicsReadback) { r.Contexts[2].Context = "unobserved" }, func(r *GraphicsReadback) { r.Contexts[1].WebGL1 = nil }, func(r *GraphicsReadback) { r.Contexts[2].WebGL2.Renderer = "native GPU leaked" },
		func(r *GraphicsReadback) { r.Contexts[0].WebGPUChecked = false }, func(r *GraphicsReadback) { r.Contexts[1].WebGPUAvailable = true },
	} {
		readback := graphicsReadbackFixture(config)
		mutate(&readback)
		if err := ValidateGraphicsReadback(config, graphicsRuntime(), readback); err == nil {
			t.Fatal("incomplete/mismatched graphics evidence accepted")
		}
	}
	runtime := graphicsRuntime()
	runtime.Engine.Version = ""
	if err := ValidateGraphicsReadback(config, runtime, graphicsReadbackFixture(config)); err == nil {
		t.Fatal("unknown runtime version accepted as graphics evidence")
	}
}
