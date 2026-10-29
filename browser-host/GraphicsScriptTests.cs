using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Text;

namespace GodUseVPN.Ultra.BrowserHost
{
    // Pure contract/readback and Node vm tests. No WebView2, GPU or browser is
    // started. Mock contexts are not evidence of actual frame/runtime coverage.
    internal static class GraphicsScriptTests
    {
        private static int checks;
        private static void Check(bool value, string name) { checks++; if (!value) throw new Exception(name); }
        private static void Reject(Action action, string name) { try { action(); } catch { checks++; return; } throw new Exception(name); }
        private static IDictionary<string, object> Profile(string launch) { return LaunchOptions.Object(LaunchOptions.Value(LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(launch)), "profile")); }
        private static string Launch(string original, object graphics, string webgpu)
        {
            var root = LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(original));
            var profile = LaunchOptions.Object(LaunchOptions.Value(root, "profile"));
            profile["graphics"] = graphics; profile["webgpu"] = webgpu;
            return LaunchOptions.Json.Serialize(root);
        }
        internal static int Run(string original)
        {
            checks = 0;
            Check(!LaunchOptions.Parse(original).Graphics.Metadata, "graphics_legacy_native_default");
            Check(!LaunchOptions.Parse(Launch(original, new { mode = "native" }, "runtime-default")).Graphics.Metadata, "graphics_explicit_native");
            Check(LaunchOptions.Parse(Launch(original, new { mode = "native" }, "disabled")).WebGpuDisabled, "independent_native_webgpu_disable");
            foreach (var preset in GraphicsOptions.Presets)
            {
                var parsed = LaunchOptions.Parse(Launch(original, new { mode = "webgl-metadata", presetId = preset[0], vendor = preset[1], renderer = preset[2] }, "disabled"));
                Check(parsed.Graphics.Metadata && parsed.Graphics.Renderer == preset[2], "graphics_named_preset");
            }
            foreach (object graphics in new object[] {
                new { mode="native", vendor="other" },
                new { mode="invent-hardware", presetId="manual",vendor="Vendor",renderer="Renderer" },
                new { mode="webgl-metadata", presetId="unknown",vendor="Vendor",renderer="Renderer" },
                new { mode="webgl-metadata", presetId=GraphicsOptions.Presets[0][0],vendor="wrong",renderer="Renderer" },
                new { mode="webgl-metadata", presetId="manual",vendor="",renderer="Renderer" },
                new { mode="webgl-metadata", presetId="manual",vendor=" Vendor",renderer="Renderer" },
                new { mode="webgl-metadata", presetId="manual",vendor="Vendor\n",renderer="Renderer" },
                new { mode="webgl-metadata", presetId="manual",vendor="厂商",renderer="Renderer" },
                new { mode="webgl-metadata", presetId="manual",vendor=new string('v',129),renderer="Renderer" },
                new { mode="webgl-metadata", presetId="manual",vendor="Vendor",renderer=new string('r',257) },
                new { mode="webgl-metadata", presetId="manual",vendor="Vendor",renderer="Renderer",script="arbitrary" }
            }) Reject(() => LaunchOptions.Parse(Launch(original, graphics, "disabled")), "unsafe_graphics_contract");
            var manual = new { mode = "webgl-metadata", presetId = "manual", vendor = "Vendor \"quoted\" \\ value", renderer = "Renderer');globalThis.injected=true;//</script>" };
            Reject(() => LaunchOptions.Parse(Launch(original, manual, "runtime-default")), "graphics_requires_native_webgpu_disable");
            var options = LaunchOptions.Parse(Launch(original, manual, "disabled"));
            Check(LaunchOptions.BrowserArguments(12345,true).Contains("--disable-features=WebGPUService") && !LaunchOptions.BrowserArguments(12345).Contains("WebGPUService"), "real_webgpu_feature_flag_only_when_requested");
            Check(GraphicsScripts.Install(new GraphicsOptions()).Length == 0, "native_installs_no_graphics_patch");
            var good = Readback(options.Graphics);
            Check(GraphicsScripts.Validate(good, options.Graphics, true).Contains("16"), "all_available_readback_paths");
            var scopes = (object[])good["scopes"];
            var mainPaths = (object[])LaunchOptions.Object(scopes[0])["paths"];
            LaunchOptions.Object(mainPaths[0])["available"] = false;
            Reject(() => GraphicsScripts.Validate(good, options.Graphics, true), "main_webgl_unavailable_is_not_success");
            good = Readback(options.Graphics); scopes = (object[])good["scopes"];
            LaunchOptions.Object(scopes[2])["webgpu"] = "adapter-available";
            Reject(() => GraphicsScripts.Validate(good, options.Graphics, true), "child_webgpu_available_is_not_success");
            good = Readback(options.Graphics); scopes = (object[])good["scopes"];
            var childPaths = (object[])LaunchOptions.Object(scopes[3])["paths"];
            LaunchOptions.Object(childPaths[0])["wrapped"] = false;
            LaunchOptions.Object(childPaths[0])["available"] = false;
            Reject(() => GraphicsScripts.Validate(good, options.Graphics, true), "unavailable_child_cannot_hide_missing_script");
            good = Readback(options.Graphics); scopes = (object[])good["scopes"];
            childPaths = (object[])LaunchOptions.Object(scopes[1])["paths"];
            LaunchOptions.Object(childPaths[2])["renderer"] = "unmasked native renderer";
            Reject(() => GraphicsScripts.Validate(good, options.Graphics, true), "offscreen_mismatch_refuses");
            RunMockJavascript(options.Graphics);
            return checks;
        }
        private static IDictionary<string, object> Readback(GraphicsOptions graphics)
        {
            var scopes = new List<object>();
            foreach (string name in new[] { "main", "gpu-same-origin", "gpu-srcdoc", "gpu-aboutblank" })
            {
                var paths = new List<object>();
                for (int index=0;index<4;index++) paths.Add(new Dictionary<string,object> { {"api",index%2==0?"webgl":"webgl2"},{"offscreen",index>=2},{"available",true},{"supported",true},{"wrapped",true},{"extension",true},{"vendor",graphics.Vendor},{"renderer",graphics.Renderer} });
                scopes.Add(new Dictionary<string,object> { {"name",name},{"webgpu","adapters-blocked"},{"paths",paths.ToArray()} });
            }
            return new Dictionary<string,object> { {"scopes",scopes.ToArray()} };
        }
        private static void RunMockJavascript(GraphicsOptions graphics)
        {
            string path = Path.Combine(Path.GetTempPath(), "ultra-graphics-mock-" + Guid.NewGuid().ToString("N") + ".js");
            try
            {
                string bundle = LaunchOptions.Json.Serialize(new { install = GraphicsScripts.Install(graphics), probe = GraphicsScripts.Probe(true,true), vendor=graphics.Vendor,renderer=graphics.Renderer });
                File.WriteAllText(path, "const bundle=" + bundle + ";\n" + MockHarness, new UTF8Encoding(false));
                using (var process = Process.Start(new ProcessStartInfo("node", "\"" + path + "\"") { UseShellExecute=false, CreateNoWindow=true, RedirectStandardOutput=true, RedirectStandardError=true }))
                {
                    if (!process.WaitForExit(10000)) { process.Kill(); throw new Exception("graphics_mock_timeout"); }
                    string output = process.StandardOutput.ReadToEnd(), error = process.StandardError.ReadToEnd();
                    Check(process.ExitCode == 0 && output.Contains("PASS graphics_js_mock_checks="), "graphics_mock_javascript_failed: " + error);
                    Console.Write(output);
                }
            }
            finally { if (File.Exists(path)) File.Delete(path); }
        }
        private const string MockHarness = @"
const vm=require('node:vm'); let checks=0;
function check(value,name){checks++;if(!value)throw Error(name);}
function context(){
 const scope=vm.createContext({});
 vm.runInContext(`
  class GL {
   constructor(){this.valid=true;this.debug=true;this.error=0;}
   getParameter(pname){
    if(!this||this.valid!==true)throw new TypeError('invalid native receiver');
    if(arguments.length===0)throw new TypeError('missing native argument');
    const value=(+pname)>>>0;
    if(value===0x9245||value===0x9246){if(!this.debug){this.error=1280;return null;}return value===0x9245?'native-vendor':'native-renderer';}
    return value===42?99:'native-other';
   }
   getExtension(name){if(name==='WEBGL_debug_renderer_info')return this.debug?{UNMASKED_VENDOR_WEBGL:0x9245,UNMASKED_RENDERER_WEBGL:0x9246}:null;if(name==='WEBGL_lose_context')return{loseContext(){}};return null;}
  }
  class GL2 extends GL{};
  Object.defineProperty(GL2.prototype,'getParameter',Object.getOwnPropertyDescriptor(GL.prototype,'getParameter'));
  globalThis.WebGLRenderingContext=GL;globalThis.WebGL2RenderingContext=GL2;
  globalThis.window=globalThis;globalThis.isSecureContext=true;
  globalThis.navigator={gpu:{requestAdapter:async()=>null}};
  globalThis.OffscreenCanvas=class {getContext(kind){return kind==='webgl2'?new GL2():new GL();}};
  globalThis.document={readyState:'complete',createElement(){return{getContext(kind){return kind==='webgl2'?new GL2():new GL();}};},getElementById(id){return globalThis.framesById[id];}};
 `,scope);
 return scope;
}
(async()=>{
 const main=context(), frames=[context(),context(),context()];
 main.framesById={};['gpu-same-origin','gpu-srcdoc','gpu-aboutblank'].forEach((id,i)=>main.framesById[id]={contentWindow:frames[i],contentDocument:{readyState:'complete'}});
 for(const scope of [main,...frames])vm.runInContext(bundle.install,scope);
 main.expectedVendor=bundle.vendor;main.expectedRenderer=bundle.renderer;
 const semantics=vm.runInContext(`(()=>{
  const gl=new WebGLRenderingContext(), gl2=new WebGL2RenderingContext();let converts=0, receiverConverts=0;
  const object={valueOf(){converts++;return 0x9245;}};
  const values=[gl.getParameter(0x9245),gl.getParameter('37446'),gl.getParameter(object),gl2.getParameter(0x9246),gl.getParameter(42)];
  let invalid=false;try{WebGLRenderingContext.prototype.getParameter.call({}, {valueOf(){receiverConverts++;return 0x9245;}});}catch(e){invalid=e instanceof TypeError;}
  let missing=false;try{gl.getParameter();}catch(e){missing=e instanceof TypeError;}
  let bigint=false;try{gl.getParameter(1n);}catch(e){bigint=e instanceof TypeError;}
  gl.debug=false;const unavailable=gl.getParameter(0x9245),error=gl.error;
  return{values,converts,receiverConverts,invalid,missing,bigint,unavailable,error,descriptor:Object.getOwnPropertyDescriptor(WebGLRenderingContext.prototype,'getParameter'),gpuUnmodified:typeof navigator.gpu.requestAdapter==='function',injected:globalThis.injected};
 })()`,main);
 check(semantics.values[0]===bundle.vendor&&semantics.values[1]===bundle.renderer&&semantics.values[2]===bundle.vendor&&semantics.values[3]===bundle.renderer,'webgl1_2_metadata_and_safe_data');
 check(semantics.converts===1&&semantics.receiverConverts===0&&semantics.invalid&&semantics.missing&&semantics.bigint,'native_conversion_receiver_order');
 check(semantics.values[4]===99&&semantics.unavailable===null&&semantics.error===1280,'unknown_and_unavailable_native_semantics');
 check(semantics.descriptor.writable&&semantics.descriptor.configurable&&!semantics.descriptor.enumerable,'descriptor_preserved');
 check(semantics.gpuUnmodified&&semantics.injected===undefined,'no_webgpu_api_fabrication_or_code_injection');
 const probe=await vm.runInContext(bundle.probe,main);
 check(probe.scopes.length===4,'all_owned_mock_frame_scopes');
 for(const scope of probe.scopes){check(scope.webgpu==='adapters-blocked','native_adapter_readback');for(const path of scope.paths)check(path.available&&path.wrapped&&path.vendor===bundle.vendor&&path.renderer===bundle.renderer,'offscreen_and_frame_mock_values');}
 vm.runInContext('navigator.gpu.requestAdapter=async()=>({nativeAdapter:true})',frames[1]);
 let rejected=false;try{await vm.runInContext(bundle.probe,main);}catch(_){rejected=true;}check(rejected,'native_adapter_available_refuses');
 console.log('PASS graphics_js_mock_checks='+checks);
})().catch(error=>{console.error(error.message);process.exitCode=1;});
";
    }
}
