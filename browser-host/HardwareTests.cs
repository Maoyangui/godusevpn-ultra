using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Text;
using System.Threading.Tasks;

namespace GodUseVPN.Ultra.BrowserHost
{
    // Fake CDP replies and Node vm contexts only. This executable never links
    // WebView2/WinForms, inspects a CPU, or invokes native Windows APIs.
    internal static class HardwareTests
    {
        private static int count;
        private static void Check(bool value,string name) { count++; if(!value)throw new Exception(name); }
        private static void Refuse(Action action,string name) { try { action(); } catch {count++;return;}throw new Exception(name); }
        private static string WithHardware(string original,string hardware,string key="hardware")
        { return original.Replace("\"identity\":", "\""+key+"\":"+hardware+",\"identity\":"); }
        private static IDictionary<string,object> Object(string json)
        { return LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(json)); }
        internal static int Run(string original)
        {
            count=0;Check(!LaunchOptions.Parse(original).Hardware.Custom,"hardware_omission_preserves_native");
            for(int value=1;value<=64;value++)
            {
                var options=LaunchOptions.Parse(WithHardware(original,"{\"mode\":\"logical-processors\",\"logicalProcessors\":"+value+"}"));
                Check(options.Hardware.Custom&&options.Hardware.LogicalProcessors==value,"hardware_integer_matrix");
            }
            foreach(string hardware in new[]{"null","{}","[]","false","\"8\"","{\"mode\":\"native\"}","{\"mode\":\"logical-processors\"}","{\"logicalProcessors\":8}","{\"mode\":\"logical-processors\",\"logicalProcessors\":null}","{\"mode\":\"logical-processors\",\"logicalProcessors\":\"8\"}","{\"mode\":\"logical-processors\",\"logicalProcessors\":8.0}","{\"mode\":\"logical-processors\",\"logicalProcessors\":8e0}","{\"mode\":\"logical-processors\",\"logicalProcessors\":0}","{\"mode\":\"logical-processors\",\"logicalProcessors\":-1}","{\"mode\":\"logical-processors\",\"logicalProcessors\":65}","{\"mode\":\"logical-processors\",\"logicalProcessors\":2147483648}","{\"Mode\":\"logical-processors\",\"logicalProcessors\":8}","{\"mode\":\"logical-processors\",\"LogicalProcessors\":8}","{\"mode\":\"logical-processors\",\"logicalProcessors\":8,\"ram\":16}","{\"mode\":\"logical-processors\",\"logicalProcessors\":8,\"logicalProcessors\":8}","{\"mode\":\"logical-processors\",\"logicalProcessors\":8,\"LOGICALPROCESSORS\":8}","{\"mode\":\"logical-processors\",\"logicalProcessors\":8,\"logical\\u0050rocessors\":8}"})
                Refuse(()=>LaunchOptions.Parse(WithHardware(original,hardware)),"hardware_bad_json_matrix");
            foreach(string key in new[]{"Hardware","HARDWARE","hArDwArE"})Refuse(()=>LaunchOptions.Parse(WithHardware(original,"{\"mode\":\"logical-processors\",\"logicalProcessors\":8}",key)),"hardware_case_alias_refused");
            var custom=LaunchOptions.Parse(WithHardware(original,"{\"mode\":\"logical-processors\",\"logicalProcessors\":8}")).Hardware;
            var calls=new List<string>();
            custom.ConfigureTarget((method,parameters)=>{
                calls.Add(method);
                var payload=Object(LaunchOptions.Json.Serialize(parameters));
                if(method==HardwareOptions.Method){Check(payload.Count==1&&LaunchOptions.Number(payload,"hardwareConcurrency")==8,"official_cdp_payload_only");return Task.FromResult(Object("{}"));}
                Check(method=="Runtime.evaluate"&&LaunchOptions.Text(payload,"expression")=="({hardwareConcurrency:navigator.hardwareConcurrency})"&&LaunchOptions.Boolean(payload,"returnByValue"),"native_property_read_only_probe");
                return Task.FromResult(Object("{\"result\":{\"value\":{\"hardwareConcurrency\":8}}}"));
            }).GetAwaiter().GetResult();
            Check(calls.Count==2&&calls[0]==HardwareOptions.Method&&calls[1]=="Runtime.evaluate","command_then_independent_readback");
            int nativeCalls=0;LaunchOptions.Parse(original).Hardware.ConfigureTarget((method,parameters)=>{nativeCalls++;throw new Exception("native_must_not_call_protocol");}).GetAwaiter().GetResult();Check(nativeCalls==0,"legacy_native_no_command_or_probe");
            foreach(string response in new[]{"{\"result\":{\"value\":{\"hardwareConcurrency\":7}}}","{\"result\":{\"value\":{\"hardwareConcurrency\":\"8\"}}}","{\"result\":{\"value\":{\"hardwareConcurrency\":8.1}}}","{\"result\":{\"value\":{}}}","{\"exceptionDetails\":{},\"result\":{}}","{\"error\":{\"code\":-32601}}","{}"})
            {
                var gate=new NavigationGate();
                Refuse(()=>{custom.ConfigureTarget((method,parameters)=>Task.FromResult(Object(method==HardwareOptions.Method?"{}":response))).GetAwaiter().GetResult();gate.Configured();},"bad_cpu_probe_never_opens_navigation");
                Check(!gate.Allowed,"cpu_probe_failure_gate_stays_closed");
            }
            calls.Clear();Refuse(()=>custom.ConfigureTarget((method,parameters)=>{calls.Add(method);return Task.FromResult(Object("{\"error\":{\"code\":-32601}}"));}).GetAwaiter().GetResult(),"unsupported_experimental_command_refused");Check(calls.Count==1,"no_readback_after_command_failure");
            var good=FrameReadback(8);custom.ValidateFrames(good);Check(true,"all_owned_frame_counts_match");
            var scopes=(object[])good["scopes"];((IDictionary<string,object>)scopes[2])["logicalProcessors"]=9;Refuse(()=>custom.ValidateFrames(good),"one_frame_mismatch_refused");
            good=FrameReadback(8);good["scopes"]=new object[]{((object[])good["scopes"])[0]};Refuse(()=>custom.ValidateFrames(good),"missing_frames_refused");
            good=FrameReadback(8);((IDictionary<string,object>)((object[])good["scopes"])[1])["name"]="main";Refuse(()=>custom.ValidateFrames(good),"duplicate_frame_scope_refused");
            RunMockJavascript();return count;
        }
        private static IDictionary<string,object> FrameReadback(int value)
        {
            var scopes=new List<object>();foreach(string name in new[]{"main","cpu-same-origin","cpu-srcdoc","cpu-aboutblank"})scopes.Add(new Dictionary<string,object>{{"name",name},{"logicalProcessors",value}});
            return new Dictionary<string,object>{{"scopes",scopes.ToArray()}};
        }
        private static void RunMockJavascript()
        {
            string path=Path.Combine(Path.GetTempPath(),"ultra-hardware-mock-"+Guid.NewGuid().ToString("N")+".js");
            try
            {
                File.WriteAllText(path,"const probe="+LaunchOptions.Json.Serialize(HardwareOptions.ProbeFrames)+";\n"+MockHarness,new UTF8Encoding(false));
                using(var process=Process.Start(new ProcessStartInfo("node","\""+path+"\""){UseShellExecute=false,CreateNoWindow=true,RedirectStandardOutput=true,RedirectStandardError=true}))
                {
                    if(!process.WaitForExit(10000)){process.Kill();throw new Exception("hardware_mock_timeout");}
                    string output=process.StandardOutput.ReadToEnd(),error=process.StandardError.ReadToEnd();Check(process.ExitCode==0&&output.Contains("PASS hardware_js_mock_checks="),"hardware_mock_javascript_failed: "+error);Console.Write(output);
                }
            }
            finally{if(File.Exists(path))File.Delete(path);}
        }
        private const string MockHarness=@"
'use strict';const vm=require('node:vm');let checks=0;
function check(value,name){checks++;if(!value)throw Error(name)}
function frame(value=8){return {contentWindow:{navigator:Object.freeze({hardwareConcurrency:value})},contentDocument:{readyState:'complete'}}}
const frames={'cpu-same-origin':frame(),'cpu-srcdoc':frame(),'cpu-aboutblank':frame()};
const native=Object.freeze({hardwareConcurrency:8});
const scope=vm.createContext({navigator:native,document:{getElementById:name=>frames[name]}});
const result=vm.runInContext(probe,scope);
check(result.scopes.length===4&&result.scopes.every(item=>item.logicalProcessors===8),'all_owned_native_values_observed');
check(native.hardwareConcurrency===8&&Object.isFrozen(native),'main_native_property_not_patched');
for(const child of Object.values(frames))check(child.contentWindow.navigator.hardwareConcurrency===8&&Object.isFrozen(child.contentWindow.navigator),'frame_native_property_not_patched');
frames['cpu-srcdoc']=frame(3);const mismatch=vm.runInContext(probe,scope);check(mismatch.scopes[2].logicalProcessors===3,'mismatch_not_hidden_or_fabricated');
delete frames['cpu-aboutblank'];let missing=false;try{vm.runInContext(probe,scope)}catch(_){missing=true}check(missing,'missing_frame_throws');
frames['cpu-aboutblank']=frame();frames['cpu-aboutblank'].contentDocument.readyState='loading';let loading=false;try{vm.runInContext(probe,scope)}catch(_){loading=true}check(loading,'unready_frame_throws');
frames['cpu-aboutblank']=frame();Object.defineProperty(frames['cpu-aboutblank'],'contentWindow',{get(){throw Error('cross_origin_denied')}});let inaccessible=false;try{vm.runInContext(probe,scope)}catch(_){inaccessible=true}check(inaccessible,'inaccessible_context_not_silently_omitted');
console.log('PASS hardware_js_mock_checks='+checks);
";
    }
}
