package browserenginehost

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

func graphicsConfig() *browserhost.GraphicsConfig {
	return &browserhost.GraphicsConfig{Mode: "webgl-metadata", PresetID: "manual", Vendor: `Selected "vendor"`, Renderer: `Selected renderer\with-literal-data`}
}
func fixtureGraphicsReadback(c Config) string {
	scopes := []map[string]any{}
	for i, name := range []string{"main", "same-origin", "opaque-origin", "cross-origin"} {
		paths := []map[string]any{}
		if c.Graphics != nil && c.Graphics.Mode == "webgl-metadata" {
			for n := 0; n < 4; n++ {
				api := "webgl"
				if n%2 != 0 {
					api = "webgl2"
				}
				paths = append(paths, map[string]any{"api": api, "offscreen": n >= 2, "available": true, "status": "matched-native-query", "vendor": c.Graphics.Vendor, "renderer": c.Graphics.Renderer})
			}
		}
		scopes = append(scopes, map[string]any{"name": name, "secure": i != 2, "webgpuPresent": false, "workerBlocked": true, "paths": paths})
	}
	raw, _ := json.Marshal(map[string]any{"scopes": scopes})
	return string(raw)
}
func TestNativeGraphicsStrictCapabilityAndNames(t *testing.T) {
	f := newDriverFixture(t, "firefox")
	c := f.config
	if ValidateGraphicsConfig(c) != nil {
		t.Fatal("old native profile refused")
	}
	c.Graphics = graphicsConfig()
	c.WebGPU = "disabled"
	if ValidateGraphicsConfig(c) != nil {
		t.Fatal("valid native Gecko metadata refused")
	}
	for _, mutation := range []func(*Config){func(c *Config) { c.EngineID = "chrome-for-testing" }, func(c *Config) { c.WebGPU = "runtime-default" }, func(c *Config) { c.Graphics.Vendor = "" }, func(c *Config) { c.Graphics.Vendor = "x\n" }, func(c *Config) { c.Graphics.Vendor = "Vendor中文" }, func(c *Config) { c.Graphics.Vendor = strings.Repeat("a", 129) }, func(c *Config) { c.Graphics.Renderer = strings.Repeat("r", 257) }, func(c *Config) { c.Graphics.PresetID = "unknown" }} {
		bad := copyHardwareConfig(c)
		mutation(&bad)
		if ValidateGraphicsConfig(bad) == nil {
			t.Fatal("invalid/unsupported metadata accepted", bad.EngineID)
		}
	}
	c.Graphics = nil
	c.EngineID = "chrome-for-testing"
	if ValidateGraphicsConfig(c) == nil {
		t.Fatal("unimplemented Chrome WebGPU disabled accepted")
	}
}
func TestFirefoxNativeGraphicsPrefsAreScopedAndDoNotDisableFingerprintingProtection(t *testing.T) {
	c := newDriverFixture(t, "firefox").config
	if s := string(firefoxPreferences(c, 43123)); strings.Contains(s, "webgl.") || strings.Contains(s, "dom.webgpu") {
		t.Fatal("omitted native graphics acquired preferences")
	}
	c.WebGPU = "disabled"
	s := string(firefoxPreferences(c, 43123))
	if !strings.Contains(s, `user_pref("dom.webgpu.enabled", false);`) || strings.Contains(s, "webgl.") {
		t.Fatal("WebGPU-only request changed names", s)
	}
	c.Graphics = graphicsConfig()
	s = string(firefoxPreferences(c, 43123))
	for _, name := range []string{"webgl.override-unmasked-vendor", "webgl.override-unmasked-renderer", "webgl.sanitize-unmasked-renderer"} {
		if !strings.Contains(s, name) {
			t.Fatal("missing native preference", name)
		}
	}
	for _, forbidden := range []string{"privacy.resistFingerprinting", "privacy.fingerprintingProtection", "webgl.enable-privileged-extensions"} {
		if strings.Contains(s, forbidden) {
			t.Fatal("silently weakened unrelated protection", forbidden)
		}
	}
	if !strings.Contains(s, `user_pref("webgl.sanitize-unmasked-renderer", false);`) || !strings.Contains(s, `Selected \"vendor\"`) {
		t.Fatal("exact metadata serialization was not JSON safe")
	}
}
func TestGraphicsReadbackRejectsMissingInsecureAndUnavailableEvidence(t *testing.T) {
	c := newDriverFixture(t, "firefox").config
	c.Graphics = graphicsConfig()
	c.WebGPU = "disabled"
	good := fixtureGraphicsReadback(c)
	e, err := verifyGraphicsReadback(good, c)
	if err != nil || e.WindowScopesVerified != 4 || e.WebGLContextsVerified != 16 || !e.WorkerExecutionBlocked || e.WorkerAPIRuntimeReadback {
		t.Fatal("wrong evidence", e, err)
	}
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["scopes"] = v["scopes"].([]any)[:3] },
		func(v map[string]any) { v["scopes"].([]any)[0].(map[string]any)["secure"] = false },
		func(v map[string]any) { v["scopes"].([]any)[2].(map[string]any)["secure"] = true },
		func(v map[string]any) { v["scopes"].([]any)[3].(map[string]any)["webgpuPresent"] = true },
		func(v map[string]any) { delete(v["scopes"].([]any)[1].(map[string]any), "workerBlocked") },
		func(v map[string]any) {
			v["scopes"].([]any)[1].(map[string]any)["paths"].([]any)[0].(map[string]any)["vendor"] = "Mozilla"
		},
		func(v map[string]any) {
			p := v["scopes"].([]any)[2].(map[string]any)["paths"].([]any)[3].(map[string]any)
			p["available"] = false
			p["status"] = "context-unavailable"
		},
		func(v map[string]any) {
			p := v["scopes"].([]any)[0].(map[string]any)["paths"].([]any)[0].(map[string]any)
			p["available"] = false
			p["status"] = "extension-unavailable"
		},
	} {
		var v map[string]any
		json.Unmarshal([]byte(good), &v)
		change(v)
		bad, _ := json.Marshal(v)
		if _, err := verifyGraphicsReadback(string(bad), c); err == nil {
			t.Fatal("unproven metadata counted as matched", string(bad))
		}
	}
}
func TestFirefoxGraphicsProtocolRequiresOwnedCrossOriginAndExistingHTTPUAProbes(t *testing.T) {
	for _, metadata := range []bool{false, true} {
		t.Run(map[bool]string{false: "webgpu-only", true: "metadata"}[metadata], func(t *testing.T) {
			f := newDriverFixture(t, "firefox")
			f.config.WebGPU = "disabled"
			if metadata {
				f.config.Graphics = graphicsConfig()
			}
			f.start()
			d := newFirefoxDriver(f.m, f.config, f.launch, func() {}).(*firefoxDriver)
			defer d.Close()
			if err := d.Configure(context.Background()); err != nil {
				t.Fatal("native graphics protocol failed", err)
			}
			e := d.GraphicsEvidence()
			if e == nil || e.WindowScopesVerified != 4 || e.WorkerAPIRuntimeReadback || f.count("browsingContext.navigate") != 2 {
				t.Fatal("graphics/HTTP evidence missing", e)
			}
			if metadata && e.WebGLContextsVerified != 16 {
				t.Fatal("Offscreen not read")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			matched := 0
			for _, command := range f.commands {
				if command.Method == "network.provideResponse" && strings.HasPrefix(command.Params["request"].(string), "https://godusevpn-graphics.invalid/owned/") {
					matched++
					body := command.Params["body"].(map[string]any)
					data, err := base64.StdEncoding.DecodeString(body["value"].(string))
					if err != nil || !strings.Contains(string(data), "getParameter") || strings.Contains(string(data), "Object.defineProperty") {
						t.Fatal("not fixed read-only probe")
					}
				}
			}
			if matched != 1 {
				t.Fatal("cross-origin probe was not independently fulfilled", matched)
			}
		})
	}
}
func TestFirefoxGraphicsNativeMismatchNeverReady(t *testing.T) {
	for _, scope := range []int{0, 1, 2, 3} {
		t.Run([]string{"main", "same", "opaque", "cross"}[scope], func(t *testing.T) {
			f := newDriverFixture(t, "firefox")
			f.config.Graphics = graphicsConfig()
			f.config.WebGPU = "disabled"
			f.mutate = func(command driverCommand, result map[string]any) {
				if command.Method != "script.evaluate" || !strings.HasPrefix(command.Params["expression"].(string), "(async function ultraGraphicsProbe(){") {
					return
				}
				var v map[string]any
				json.Unmarshal([]byte(result["result"].(map[string]any)["value"].(string)), &v)
				v["scopes"].([]any)[scope].(map[string]any)["paths"].([]any)[0].(map[string]any)["renderer"] = "sanitized/mismatching"
				raw, _ := json.Marshal(v)
				result["result"].(map[string]any)["value"] = string(raw)
			}
			f.start()
			d := newFirefoxDriver(f.m, f.config, f.launch, func() {}).(*firefoxDriver)
			defer d.Close()
			if d.Configure(context.Background()) == nil || d.GraphicsEvidence() != nil {
				t.Fatal("mismatch reached graphics readiness")
			}
		})
	}
}

func TestGraphicsProductionProbeReadsNativeMocksWithoutReplacingMethods(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is unavailable for the pure vm fixture")
	}
	for _, metadata := range []bool{false, true} {
		c := newDriverFixture(t, "firefox").config
		c.WebGPU = "disabled"
		if metadata {
			c.Graphics = graphicsConfig()
		}
		input := map[string]any{"expression": graphicsProbeExpression(c, "https://godusevpn-graphics.invalid/owned/"+strings.Repeat("a", 48)), "frameHTML": graphicsFrameHTML(c), "vendor": "selected-vendor", "renderer": "selected-renderer", "metadata": metadata}
		raw, _ := json.Marshal(input)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, node, "../../docs/ultra/qa/native-graphics-probe-pure.cjs")
		cmd.Stdin = strings.NewReader(string(raw))
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("pure production probe failed (metadata=%v): %v %s", metadata, err, out)
		}
	}
}

func TestFirefoxGraphicsCrossOriginNetworkEvidenceCannotBeReplacedByFrameBooleans(t *testing.T) {
	for _, fault := range []string{"graphics-cross-request-missing", "graphics-cross-http-ua", "network.provideResponse"} {
		t.Run(fault, func(t *testing.T) {
			f := newDriverFixture(t, "firefox")
			f.config.Graphics = graphicsConfig()
			f.config.WebGPU = "disabled"
			f.fault = fault
			if fault == "network.provideResponse" {
				f.faultRequest = "https://godusevpn-graphics.invalid/owned/"
			}
			f.start()
			d := newFirefoxDriver(f.m, f.config, f.launch, func() {}).(*firefoxDriver)
			defer d.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			if d.Configure(ctx) == nil || d.GraphicsEvidence() != nil {
				t.Fatal("missing/wrong/failed owned cross-origin response reached readiness")
			}
		})
	}
}

func TestGraphicsOffscreenAbsenceIsExplicitAndNotMatched(t *testing.T) {
	c := newDriverFixture(t, "firefox").config
	c.Graphics = graphicsConfig()
	c.WebGPU = "disabled"
	var v map[string]any
	json.Unmarshal([]byte(fixtureGraphicsReadback(c)), &v)
	paths := v["scopes"].([]any)[0].(map[string]any)["paths"].([]any)
	for _, index := range []int{2, 3} {
		p := paths[index].(map[string]any)
		p["available"] = false
		p["status"] = "api-absent"
		delete(p, "vendor")
		delete(p, "renderer")
	}
	raw, _ := json.Marshal(v)
	e, err := verifyGraphicsReadback(string(raw), c)
	if err != nil || e.OffscreenAPIAbsentScopes != 1 || e.WebGLContextsVerified != 14 {
		t.Fatal("absent API counted as matched", e, err)
	}
	paths[3].(map[string]any)["status"] = "context-unavailable"
	raw, _ = json.Marshal(v)
	if _, err := verifyGraphicsReadback(string(raw), c); err == nil {
		t.Fatal("inconsistent Offscreen evidence accepted")
	}
}

func TestGraphicsConfigurationAndEvidenceDoNotAliasCallerMemory(t *testing.T) {
	f := newDriverFixture(t, "firefox")
	f.config.Graphics = graphicsConfig()
	f.config.WebGPU = "disabled"
	d := newFirefoxDriver(f.m, f.config, f.launch, func() {}).(*firefoxDriver)
	defer d.Close()
	f.config.Graphics.Vendor = "changed externally"
	if d.config.Graphics.Vendor == f.config.Graphics.Vendor {
		t.Fatal("configuration shares a mutable caller pointer")
	}
	d.graphicsEvidence = &GraphicsEvidence{WindowScopesVerified: 4, WorkerAPIRuntimeReadback: false}
	r := d.GraphicsEvidence()
	r.WindowScopesVerified = 0
	if d.GraphicsEvidence().WindowScopesVerified != 4 {
		t.Fatal("returned evidence aliases driver state")
	}
	s := &Session{status: Status{Graphics: d.GraphicsEvidence()}}
	r = s.Status().Graphics
	r.WindowScopesVerified = 0
	if s.Status().Graphics.WindowScopesVerified != 4 {
		t.Fatal("status evidence aliases session")
	}
}
