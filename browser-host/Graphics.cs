using System;
using System.Collections.Generic;

namespace GodUseVPN.Ultra.BrowserHost
{
    // Only two WebGL metadata strings are configurable. This never selects a
    // physical adapter, changes acceleration, or changes rendered pixels.
    internal sealed class GraphicsOptions
    {
        internal string Mode = "native", PresetId = "", Vendor = "", Renderer = "";
        internal bool Metadata { get { return Mode == "webgl-metadata"; } }
        internal static readonly string[][] Presets = {
            new[] { "intel-uhd-620-d3d11", "Google Inc. (Intel)", "ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)" },
            new[] { "nvidia-rtx-3060-d3d11", "Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)" },
            new[] { "amd-rx-6600-d3d11", "Google Inc. (AMD)", "ANGLE (AMD, AMD Radeon RX 6600 Direct3D11 vs_5_0 ps_5_0, D3D11)" }
        };
        internal static GraphicsOptions Parse(IDictionary<string, object> profile, bool webGpuDisabled)
        {
            var result = new GraphicsOptions();
            if (!profile.ContainsKey("graphics")) return result;
            var value = LaunchOptions.Object(LaunchOptions.Value(profile, "graphics"));
            LaunchOptions.Keys(value, "mode", "presetId", "vendor", "renderer");
            result.Mode = LaunchOptions.Text(value, "mode");
            result.PresetId = LaunchOptions.OptionalText(value, "presetId");
            result.Vendor = LaunchOptions.OptionalText(value, "vendor");
            result.Renderer = LaunchOptions.OptionalText(value, "renderer");
            if (result.Mode == "native")
            {
                if (result.PresetId.Length != 0 || result.Vendor.Length != 0 || result.Renderer.Length != 0) throw LaunchOptions.Reject("native_graphics_override");
                return result;
            }
            if (!result.Metadata || !webGpuDisabled || !Printable(result.Vendor, 128) || !Printable(result.Renderer, 256)) throw LaunchOptions.Reject("unsupported_graphics_configuration");
            if (result.PresetId == "manual") return result;
            foreach (var preset in Presets)
                if (result.PresetId == preset[0] && result.Vendor == preset[1] && result.Renderer == preset[2]) return result;
            throw LaunchOptions.Reject("graphics_preset_mismatch");
        }
        private static bool Printable(string value, int limit)
        {
            if (String.IsNullOrEmpty(value) || value.Length > limit || value.Trim() != value) return false;
            foreach (char character in value) if (character < 32 || character > 126) return false;
            return true;
        }
    }

    internal static class GraphicsScripts
    {
        // The data is serialized as JSON, never interpolated into executable
        // JavaScript. The native method still performs its receiver check first.
        // Its GLenum conversion is observed exactly once, including exotic
        // valueOf/Symbol.toPrimitive inputs; unknown parameters retain semantics.
        internal static string Install(GraphicsOptions options)
        {
            if (!options.Metadata) return "";
            return InstallBody + "(" + LaunchOptions.Json.Serialize(new { vendor = options.Vendor, renderer = options.Renderer }) + ");";
        }
        private const string InstallBody = @"(function(config) {
'use strict';
const apply = Reflect.apply, getDescriptor = Object.getOwnPropertyDescriptor, define = Object.defineProperty;
const primitiveSymbol = Symbol.toPrimitive, marker = Symbol.for('GodUseVPN.Ultra.WebGLMetadata.v1');
const TypeErrorNative = TypeError, slice = Array.prototype.slice;
const primitive = value => value === null || (typeof value !== 'object' && typeof value !== 'function');
function numberPrimitive(value) {
  if (primitive(value)) return value;
  const exotic = value[primitiveSymbol];
  if (exotic !== null && exotic !== undefined) {
    if (typeof exotic !== 'function') throw new TypeErrorNative('Invalid numeric conversion');
    const result = apply(exotic, value, ['number']);
    if (!primitive(result)) throw new TypeErrorNative('Invalid numeric conversion');
    return result;
  }
  for (const name of ['valueOf', 'toString']) {
    const method = value[name];
    if (typeof method === 'function') { const result = apply(method, value, []); if (primitive(result)) return result; }
  }
  throw new TypeErrorNative('Invalid numeric conversion');
}
for (const name of ['WebGLRenderingContext', 'WebGL2RenderingContext']) {
  const constructor = globalThis[name];
  if (typeof constructor !== 'function') continue;
  const prototype = constructor.prototype, descriptor = getDescriptor(prototype, 'getParameter');
  if (!descriptor || typeof descriptor.value !== 'function') throw new TypeErrorNative('Unsupported WebGL method');
  const original = descriptor.value;
  const replacement = { getParameter(pname) {
    if (arguments.length === 0) return apply(original, this, arguments);
    let converted;
    const forwarded = apply(slice, arguments, []);
    forwarded[0] = { [primitiveSymbol]() {
      const value = numberPrimitive(pname);
      converted = (+value) >>> 0;
      return value;
    } };
    const result = apply(original, this, forwarded);
    // A missing/disabled WEBGL_debug_renderer_info extension must still return
    // the original null/error rather than fabricating extension availability.
    if (typeof result === 'string' && converted === 0x9245) return config.vendor;
    if (typeof result === 'string' && converted === 0x9246) return config.renderer;
    return result;
  } }.getParameter;
  define(replacement, marker, { value: true });
  define(prototype, 'getParameter', { value: replacement, writable: descriptor.writable, enumerable: descriptor.enumerable, configurable: descriptor.configurable });
}
})";

        // Runs only against the app-owned bootstrap document and its own fixed
        // same-origin/srcdoc/about:blank frames. No website supplies this code.
        internal static string Probe(bool metadata, bool webGpuDisabled)
        {
            return ProbeBody + "(" + LaunchOptions.Json.Serialize(new { metadata = metadata, webGpuDisabled = webGpuDisabled }) + ")";
        }
        private const string ProbeBody = @"(async function(config) {
'use strict';
const marker = Symbol.for('GodUseVPN.Ultra.WebGLMetadata.v1');
function read(scope, kind, offscreen) {
  const constructor = scope[kind === 'webgl2' ? 'WebGL2RenderingContext' : 'WebGLRenderingContext'];
  const wrapped = !!(constructor && constructor.prototype.getParameter[marker]);
  if (offscreen && typeof scope.OffscreenCanvas !== 'function') return { api: kind, offscreen, available: false, supported: !!constructor, wrapped, reason: 'api-unavailable' };
  const canvas = offscreen ? new scope.OffscreenCanvas(16, 16) : scope.document.createElement('canvas');
  let gl;
  try { gl = canvas.getContext(kind); } catch (_) { return { api: kind, offscreen, available: false, supported: !!constructor, wrapped, reason: 'context-error' }; }
  if (!gl) return { api: kind, offscreen, available: false, supported: !!constructor, wrapped, reason: 'context-unavailable' };
  try {
    const extension = gl.getExtension('WEBGL_debug_renderer_info');
    if (!extension) return { api: kind, offscreen, available: true, supported: !!constructor, wrapped, extension: false };
    return { api: kind, offscreen, available: true, supported: !!constructor, wrapped, extension: true, vendor: gl.getParameter(extension.UNMASKED_VENDOR_WEBGL), renderer: gl.getParameter(extension.UNMASKED_RENDERER_WEBGL) };
  } finally { const release = gl.getExtension('WEBGL_lose_context'); if (release) release.loseContext(); }
}
async function inspect(scope, name) {
  let webgpu = 'runtime-default';
  if (config.webGpuDisabled) {
    if (!scope.isSecureContext) throw new Error('Insecure graphics probe');
    const gpu = scope.navigator.gpu;
    if (gpu === undefined) webgpu = 'api-absent';
    else {
      const results = await Promise.all([gpu.requestAdapter(), gpu.requestAdapter({forceFallbackAdapter:true}), gpu.requestAdapter({powerPreference:'high-performance'}), gpu.requestAdapter({powerPreference:'low-power'})]);
      if (results.some(value => value !== null)) throw new Error('Native WebGPU adapter remains available');
      webgpu = 'adapters-blocked';
    }
  }
  const paths = config.metadata ? [read(scope,'webgl',false),read(scope,'webgl2',false),read(scope,'webgl',true),read(scope,'webgl2',true)] : [];
  return { name, webgpu, paths };
}
const scopes = [await inspect(window, 'main')];
if (config.metadata || config.webGpuDisabled) {
  for (const id of ['gpu-same-origin','gpu-srcdoc','gpu-aboutblank']) {
    const frame = document.getElementById(id);
    if (!frame || !frame.contentWindow || frame.contentDocument.readyState !== 'complete') throw new Error('Graphics probe frame incomplete');
    scopes.push(await inspect(frame.contentWindow, id));
  }
}
return { scopes };
})";

        internal static string Validate(object probe, GraphicsOptions options, bool webGpuDisabled)
        {
            var root = LaunchOptions.Object(probe);
            var scopes = LaunchOptions.Value(root, "scopes") as object[];
            if (scopes == null || scopes.Length != (options.Metadata || webGpuDisabled ? 4 : 1)) throw LaunchOptions.Reject("graphics_scope_readback_failed");
            string[] names = { "main", "gpu-same-origin", "gpu-srcdoc", "gpu-aboutblank" };
            int checkedPaths = 0, unavailable = 0;
            for (int index = 0; index < scopes.Length; index++)
            {
                var scope = LaunchOptions.Object(scopes[index]);
                if (LaunchOptions.Text(scope, "name") != names[index]) throw LaunchOptions.Reject("graphics_scope_readback_failed");
                string gpu = LaunchOptions.Text(scope, "webgpu");
                if (webGpuDisabled && gpu != "api-absent" && gpu != "adapters-blocked") throw LaunchOptions.Reject("native_webgpu_disable_failed");
                if (!options.Metadata) continue;
                var paths = LaunchOptions.Value(scope, "paths") as object[];
                if (paths == null || paths.Length != 4) throw LaunchOptions.Reject("webgl_path_readback_failed");
                for (int number = 0; number < paths.Length; number++)
                {
                    var path = LaunchOptions.Object(paths[number]);
                    bool available = LaunchOptions.Boolean(path, "available");
                    if (LaunchOptions.Text(path, "api") != (number % 2 == 0 ? "webgl" : "webgl2") || LaunchOptions.Boolean(path, "offscreen") != (number >= 2)) throw LaunchOptions.Reject("webgl_path_readback_failed");
                    if (LaunchOptions.Boolean(path, "supported") && !LaunchOptions.Boolean(path, "wrapped")) throw LaunchOptions.Reject("frame_graphics_script_missing");
                    if (!available)
                    {
                        if (index == 0 && number < 2) throw LaunchOptions.Reject("main_webgl_context_required");
                        unavailable++; continue;
                    }
                    if (!LaunchOptions.Boolean(path, "wrapped") || !LaunchOptions.Boolean(path, "extension") || LaunchOptions.Text(path, "vendor") != options.Vendor || LaunchOptions.Text(path, "renderer") != options.Renderer) throw LaunchOptions.Reject("webgl_metadata_readback_failed");
                    checkedPaths++;
                }
            }
            if (options.Metadata) return "WebGL 名称已读回：" + checkedPaths + " 条可用路径；" + unavailable + " 条上下文不可用，未计为匹配。WebGPU 原生适配器已阻断。";
            return webGpuDisabled ? "GPU 名称保持原生；WebGPU 原生适配器已阻断（接口可能仍可见）。" : "GPU / WebGPU 保持原生。";
        }
    }
}
