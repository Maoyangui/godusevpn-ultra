package browserenginehost

import (
	"encoding/json"
	"errors"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

var ErrGraphicsReadback = errors.New("browser_engine_native_graphics_readback_refused")

// GraphicsEvidence contains scope/count evidence, never native GPU identifiers.
// Worker execution remains blocked; no Worker API runtime readback is claimed.
type GraphicsEvidence struct {
	Implementation           string `json:"implementation"`
	WindowScopesVerified     int    `json:"windowScopesVerified"`
	WebGLContextsVerified    int    `json:"webglContextsVerified"`
	OffscreenAPIAbsentScopes int    `json:"offscreenApiAbsentScopes"`
	WebGPU                   string `json:"webgpu"`
	WorkerExecutionBlocked   bool   `json:"workerExecutionBlocked"`
	WorkerAPIRuntimeReadback bool   `json:"workerApiRuntimeReadback"`
}

// ValidateGraphicsConfig checks capability as well as the shared strict names.
// Stock Chromium has no implemented native metadata backend in this host.
func ValidateGraphicsConfig(c Config) error {
	if c.WebGPU != "" && c.WebGPU != "runtime-default" && c.WebGPU != "disabled" {
		return ErrRefused
	}
	if browserhost.ValidateGraphicsOptions(c.Graphics, "runtime-default", c.WebGPU, c.Environment) != nil {
		return ErrRefused
	}
	if graphicsRequested(c) && c.EngineID != browserengine.Firefox {
		return ErrRefused
	}
	return nil
}

func graphicsRequested(c Config) bool {
	return c.WebGPU == "disabled" || c.Graphics != nil && c.Graphics.Mode == "webgl-metadata"
}

func addFirefoxGraphicsPreferences(prefs map[string]any, c Config) {
	if c.WebGPU == "disabled" {
		prefs["dom.webgpu.enabled"] = false
	}
	if c.Graphics != nil && c.Graphics.Mode == "webgl-metadata" {
		prefs["webgl.override-unmasked-vendor"] = c.Graphics.Vendor
		prefs["webgl.override-unmasked-renderer"] = c.Graphics.Renderer
		// Gecko sanitizes the renderer after applying its native override.
		// This is scoped only to the user's explicit metadata request in a new
		// private profile. RFP/FPP, extensions and actual hardware stay intact.
		prefs["webgl.sanitize-unmasked-renderer"] = false
	}
}

type graphicsPath struct {
	API       string `json:"api"`
	Offscreen bool   `json:"offscreen"`
	Available *bool  `json:"available"`
	Status    string `json:"status"`
	Vendor    string `json:"vendor"`
	Renderer  string `json:"renderer"`
}
type graphicsScope struct {
	Name          string         `json:"name"`
	Secure        *bool          `json:"secure"`
	WebGPUPresent *bool          `json:"webgpuPresent"`
	WorkerBlocked *bool          `json:"workerBlocked"`
	Paths         []graphicsPath `json:"paths"`
}

func verifyGraphicsReadback(raw string, c Config) (*GraphicsEvidence, error) {
	var value struct {
		Scopes []graphicsScope `json:"scopes"`
	}
	if ValidateGraphicsConfig(c) != nil || c.EngineID != browserengine.Firefox || c.WebGPU != "disabled" || !graphicsRequested(c) || len(raw) == 0 || len(raw) > 32768 || uniqueProtocolJSON([]byte(raw)) != nil || json.Unmarshal([]byte(raw), &value) != nil || len(value.Scopes) != 4 {
		return nil, ErrGraphicsReadback
	}
	evidence := &GraphicsEvidence{Implementation: "gecko-native-preferences", WebGPU: "native-pref-disabled", WorkerExecutionBlocked: true}
	names := []string{"main", "same-origin", "opaque-origin", "cross-origin"}
	metadata := c.Graphics != nil && c.Graphics.Mode == "webgl-metadata"
	for i, scope := range value.Scopes {
		if scope.Name != names[i] || scope.Secure == nil || *scope.Secure != (i != 2) || scope.WebGPUPresent == nil || *scope.WebGPUPresent || scope.WorkerBlocked == nil || !*scope.WorkerBlocked {
			return nil, ErrGraphicsReadback
		}
		if metadata {
			if len(scope.Paths) != 4 {
				return nil, ErrGraphicsReadback
			}
			if (scope.Paths[2].Status == "api-absent") != (scope.Paths[3].Status == "api-absent") {
				return nil, ErrGraphicsReadback
			}
			for index, path := range scope.Paths {
				api := "webgl"
				if index%2 != 0 {
					api = "webgl2"
				}
				if path.API != api || path.Offscreen != (index >= 2) || path.Available == nil {
					return nil, ErrGraphicsReadback
				}
				if !*path.Available {
					// API absence is distinct from a failed/null existing context.
					// Neither is counted as a successful metadata match.
					if index < 2 || path.Status != "api-absent" {
						return nil, ErrGraphicsReadback
					}
					if index == 2 {
						evidence.OffscreenAPIAbsentScopes++
					}
					continue
				}
				if path.Status != "matched-native-query" || path.Vendor != c.Graphics.Vendor || path.Renderer != c.Graphics.Renderer {
					return nil, ErrGraphicsReadback
				}
				evidence.WebGLContextsVerified++
			}
		} else if len(scope.Paths) != 0 {
			return nil, ErrGraphicsReadback
		}
		evidence.WindowScopesVerified++
	}
	return evidence, nil
}

// This only reads native APIs. No navigator/WebGL getter is replaced. Worker
// constructors are deliberately still the already-installed execution denial.
const graphicsCollect = `async function collect(){
const read=(api,offscreen)=>{const base={api,offscreen};if(offscreen&&typeof OffscreenCanvas!=='function')return {...base,available:false,status:'api-absent'};
let gl;try{const c=offscreen?new OffscreenCanvas(16,16):document.createElement('canvas');gl=c.getContext(api);if(!gl)return {...base,available:false,status:'context-unavailable'};const e=gl.getExtension('WEBGL_debug_renderer_info');if(!e)return {...base,available:false,status:'extension-unavailable'};return {...base,available:true,status:'matched-native-query',vendor:gl.getParameter(e.UNMASKED_VENDOR_WEBGL),renderer:gl.getParameter(e.UNMASKED_RENDERER_WEBGL)};}catch(_){return {...base,available:false,status:'context-error'}}finally{if(gl){const lose=gl.getExtension('WEBGL_lose_context');if(lose)lose.loseContext()}}};
return {secure:isSecureContext,webgpuPresent:('gpu' in navigator)||(typeof globalThis.GPU!=='undefined'),workerBlocked:(()=>{try{new Worker('data:text/javascript,');return false}catch(e){return e.name==='NotAllowedError'}})(),paths:METADATA?[read('webgl',false),read('webgl2',false),read('webgl',true),read('webgl2',true)]:[]};}`

func graphicsCollector(c Config) string {
	metadata := "false"
	if c.Graphics != nil && c.Graphics.Mode == "webgl-metadata" {
		metadata = "true"
	}
	return "const METADATA=" + metadata + ";const collect=" + graphicsCollect + ";"
}

func graphicsFrameHTML(c Config) string {
	return "<!doctype html><meta charset='utf-8'><script>" + graphicsCollector(c) + `collect().then(value=>parent.postMessage({ultraGraphics:true,value},'*'));</script>`
}

func graphicsProbeExpression(c Config, crossURL string) string {
	u, _ := json.Marshal(crossURL)
	html, _ := json.Marshal(graphicsFrameHTML(c))
	return `(async function ultraGraphicsProbe(){` + graphicsCollector(c) + `const top=await collect();const source=` + string(html) + `;const cross=` + string(u) + `;
const frames=await Promise.all(['same-origin','opaque-origin','cross-origin'].map(name=>new Promise((resolve,reject)=>{const f=document.createElement('iframe');let done=false;const finish=(err,value)=>{if(done)return;done=true;clearTimeout(timer);removeEventListener('message',on);f.remove();err?reject(err):resolve({name,...value})};const on=e=>{const origin=name==='cross-origin'?new URL(cross).origin:name==='opaque-origin'?'null':location.origin;if(e.source===f.contentWindow&&e.origin===origin&&e.data&&e.data.ultraGraphics===true)finish(null,e.data.value)};const timer=setTimeout(()=>finish(Error('graphics probe timeout')),5000);addEventListener('message',on);if(name==='same-origin')f.srcdoc=source;else if(name==='opaque-origin')f.src='data:text/html,'+encodeURIComponent(source);else f.src=cross;(document.body||document.documentElement).appendChild(f)})));
return JSON.stringify({scopes:[{name:'main',...top},...frames]});})()`
}
